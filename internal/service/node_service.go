package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
)

const defaultNodeSyncIntervalSec = 60

// NodeStateChangeCallback is invoked when a node automatically changes or recovers its desired state.
type NodeStateChangeCallback func(ctx context.Context, node *domain.Node, oldState, newState, reason string)

// NodeCacheInvalidator invalidates cached compiled node configuration and user snapshots.
type NodeCacheInvalidator interface {
	InvalidateNode(nodeID uint)
	InvalidateAll()
}

// RemoteNodeCompiler compiles remote node Xray configurations and user sync payloads.
type RemoteNodeCompiler interface {
	CompileRemoteNodeToJSON(inbounds []domain.Inbound, users []domain.User) ([]byte, error)
	BuildRemoteNodeUsers(inbounds []domain.Inbound, users []domain.User) ([]domain.AgentSyncUser, error)
}

type nodeInboundFinder interface {
	FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error)
}

type cachedNodeSnapshot struct {
	ConfigHash string
	UsersHash  string
	ConfigJSON string
	Users      []domain.AgentSyncUser
}

// NodeService manages remote direct-connect nodes, tokens, sync snapshots, and traffic ingestion.
type NodeService struct {
	nodeRepo        domain.NodeRepository
	batchRepo       domain.NodeTrafficBatchRepository
	quotaRepo       domain.NodeQuotaRepository
	inboundRepo     domain.InboundRepository
	userRepo        domain.UserRepository
	compiler        RemoteNodeCompiler
	stateNotifier   NodeStateChangeCallback
	syncIntervalSec int

	mu          sync.RWMutex
	snapshots   map[uint]*cachedNodeSnapshot
	globalEpoch uint64
	nodeEpochs  map[uint]uint64
}

func NewNodeService(
	nodeRepo domain.NodeRepository,
	batchRepo domain.NodeTrafficBatchRepository,
	inboundRepo domain.InboundRepository,
	userRepo domain.UserRepository,
	compiler RemoteNodeCompiler,
) *NodeService {
	svc := &NodeService{
		nodeRepo:        nodeRepo,
		batchRepo:       batchRepo,
		inboundRepo:     inboundRepo,
		userRepo:        userRepo,
		compiler:        compiler,
		syncIntervalSec: defaultNodeSyncIntervalSec,
		snapshots:       make(map[uint]*cachedNodeSnapshot),
		nodeEpochs:      make(map[uint]uint64),
	}
	if qr, ok := nodeRepo.(domain.NodeQuotaRepository); ok {
		svc.quotaRepo = qr
	}
	return svc
}

// WithQuotaRepo injects the node quota repository (auto-detected when nodeRepo implements it).
func (s *NodeService) WithQuotaRepo(repo domain.NodeQuotaRepository) *NodeService {
	s.quotaRepo = repo
	return s
}

// WithStateChangeNotifier injects a callback invoked on automatic node quota state transitions.
func (s *NodeService) WithStateChangeNotifier(fn NodeStateChangeCallback) *NodeService {
	s.stateNotifier = fn
	return s
}

// HashNodeToken returns the lowercase SHA-256 hex digest of a plaintext node token.
func HashNodeToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func generateNodeToken() (plain, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate node token failed: %w", err)
	}
	plain = hex.EncodeToString(buf)
	hash = HashNodeToken(plain)
	prefix = plain[:8]
	return plain, hash, prefix, nil
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *NodeService) InvalidateNode(nodeID uint) {
	if s == nil || nodeID == 0 {
		return
	}
	s.mu.Lock()
	s.nodeEpochs[nodeID]++
	delete(s.snapshots, nodeID)
	s.mu.Unlock()
}

func (s *NodeService) InvalidateAll() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.globalEpoch++
	clear(s.snapshots)
	s.mu.Unlock()
}

func (s *NodeService) ListNodes(ctx context.Context) ([]domain.Node, error) {
	nodes, err := s.nodeRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range nodes {
		nodes[i].RefreshOnline(now)
	}
	return nodes, nil
}

func (s *NodeService) GetNode(ctx context.Context, id uint) (*domain.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	node.RefreshOnline(time.Now())
	return node, nil
}

func validateNodeQuotaFields(node *domain.Node) error {
	if node.TrafficLimitBytes < 0 {
		return fmt.Errorf("%w: trafficLimitBytes must be >= 0", domain.ErrInvalidInput)
	}
	if node.BillingResetDay < 0 || node.BillingResetDay > 31 {
		return fmt.Errorf("%w: billingResetDay must be between 0 and 31", domain.ErrInvalidInput)
	}
	mode := strings.TrimSpace(node.TrafficCountMode)
	if mode == "" {
		mode = domain.TrafficCountModeBoth
	}
	if !domain.IsValidTrafficCountMode(mode) {
		return fmt.Errorf("%w: unsupported trafficCountMode %q", domain.ErrInvalidInput, node.TrafficCountMode)
	}
	node.TrafficCountMode = mode

	if math.IsNaN(node.TrafficMultiplier) || math.IsInf(node.TrafficMultiplier, 0) || node.TrafficMultiplier <= 0 {
		return fmt.Errorf("%w: trafficMultiplier must be > 0", domain.ErrInvalidInput)
	}
	if node.WarnThresholdPct <= 0 || node.DrainThresholdPct <= 0 || node.BreakThresholdPct <= 0 ||
		node.WarnThresholdPct > node.DrainThresholdPct ||
		node.DrainThresholdPct > node.BreakThresholdPct ||
		node.BreakThresholdPct > 100 {
		return fmt.Errorf("%w: thresholds must satisfy 0 < warn <= drain <= break <= 100", domain.ErrInvalidInput)
	}
	return nil
}

func (s *NodeService) CreateNode(ctx context.Context, dto domain.CreateNodeDTO) (*domain.CreateNodeResponse, error) {
	name := strings.TrimSpace(dto.Name)
	host := strings.TrimSpace(dto.Host)
	if name == "" || host == "" {
		return nil, fmt.Errorf("%w: name and host are required", domain.ErrInvalidInput)
	}

	desiredState := strings.TrimSpace(dto.DesiredState)
	if desiredState == "" {
		desiredState = domain.NodeStateActive
	}
	if !domain.IsValidNodeState(desiredState) {
		return nil, fmt.Errorf("%w: unsupported desiredState %q", domain.ErrInvalidInput, desiredState)
	}

	enabled := true
	if dto.Enabled != nil {
		enabled = *dto.Enabled
	}

	var limitBytes int64
	if dto.TrafficLimitBytes != nil {
		limitBytes = *dto.TrafficLimitBytes
	}
	var resetDay int
	if dto.BillingResetDay != nil {
		resetDay = *dto.BillingResetDay
	}
	countMode := strings.TrimSpace(dto.TrafficCountMode)
	if countMode == "" {
		countMode = domain.TrafficCountModeBoth
	}
	multiplier := 1.0
	if dto.TrafficMultiplier != nil {
		multiplier = *dto.TrafficMultiplier
	}
	warnPct := domain.DefaultWarnThresholdPct
	if dto.WarnThresholdPct != nil {
		warnPct = *dto.WarnThresholdPct
	}
	drainPct := domain.DefaultDrainThresholdPct
	if dto.DrainThresholdPct != nil {
		drainPct = *dto.DrainThresholdPct
	}
	breakPct := domain.DefaultBreakThresholdPct
	if dto.BreakThresholdPct != nil {
		breakPct = *dto.BreakThresholdPct
	}

	plain, hash, prefix, err := generateNodeToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	node := &domain.Node{
		Name:              name,
		Host:              host,
		Region:            strings.TrimSpace(dto.Region),
		TokenHash:         hash,
		TokenPrefix:       prefix,
		Enabled:           enabled,
		DesiredState:      desiredState,
		Remark:            strings.TrimSpace(dto.Remark),
		TrafficLimitBytes: limitBytes,
		BillingResetDay:   resetDay,
		TrafficCountMode:  countMode,
		TrafficMultiplier: multiplier,
		WarnThresholdPct:  warnPct,
		DrainThresholdPct: drainPct,
		BreakThresholdPct: breakPct,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := validateNodeQuotaFields(node); err != nil {
		return nil, err
	}

	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, fmt.Errorf("create node failed: %w", err)
	}
	node.RefreshComputed(now)

	return &domain.CreateNodeResponse{
		Node:  *node,
		Token: plain,
	}, nil
}

func (s *NodeService) UpdateNode(ctx context.Context, id uint, dto domain.UpdateNodeDTO) (*domain.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Normalize legacy zero values before applying partial quota updates.
	node.TrafficCountMode = node.EffectiveTrafficCountMode()
	node.TrafficMultiplier = node.EffectiveMultiplier()
	node.WarnThresholdPct, node.DrainThresholdPct, node.BreakThresholdPct = node.EffectiveThresholds()

	if dto.Name != nil {
		name := strings.TrimSpace(*dto.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", domain.ErrInvalidInput)
		}
		node.Name = name
	}
	if dto.Host != nil {
		host := strings.TrimSpace(*dto.Host)
		if host == "" {
			return nil, fmt.Errorf("%w: host cannot be empty", domain.ErrInvalidInput)
		}
		node.Host = host
	}
	if dto.Region != nil {
		node.Region = strings.TrimSpace(*dto.Region)
	}
	if dto.Enabled != nil {
		node.Enabled = *dto.Enabled
	}
	if dto.DesiredState != nil {
		state := strings.TrimSpace(*dto.DesiredState)
		if !domain.IsValidNodeState(state) {
			return nil, fmt.Errorf("%w: unsupported desiredState %q", domain.ErrInvalidInput, state)
		}
		if state != node.DesiredState {
			node.DesiredState = state
			if state == domain.NodeStateActive {
				node.AutoStateReason = ""
			} else {
				node.AutoStateReason = domain.AutoReasonManual
			}
		}
	}
	if dto.Remark != nil {
		node.Remark = strings.TrimSpace(*dto.Remark)
	}
	if dto.TrafficLimitBytes != nil {
		node.TrafficLimitBytes = *dto.TrafficLimitBytes
	}
	if dto.BillingResetDay != nil {
		node.BillingResetDay = *dto.BillingResetDay
	}
	if dto.TrafficCountMode != nil {
		mode := strings.TrimSpace(*dto.TrafficCountMode)
		if mode == "" {
			return nil, fmt.Errorf("%w: trafficCountMode cannot be empty", domain.ErrInvalidInput)
		}
		node.TrafficCountMode = mode
	}
	if dto.TrafficMultiplier != nil {
		node.TrafficMultiplier = *dto.TrafficMultiplier
	}
	if dto.WarnThresholdPct != nil {
		node.WarnThresholdPct = *dto.WarnThresholdPct
	}
	if dto.DrainThresholdPct != nil {
		node.DrainThresholdPct = *dto.DrainThresholdPct
	}
	if dto.BreakThresholdPct != nil {
		node.BreakThresholdPct = *dto.BreakThresholdPct
	}

	if err := validateNodeQuotaFields(node); err != nil {
		return nil, err
	}
	node.UpdatedAt = time.Now()

	if err := s.nodeRepo.Update(ctx, node); err != nil {
		return nil, fmt.Errorf("update node failed: %w", err)
	}

	node.RefreshOnline(time.Now())
	return node, nil
}

// SetNodeDesiredState manually updates a node's desired state and records reason as "manual".
func (s *NodeService) SetNodeDesiredState(ctx context.Context, id uint, desiredState string) (*domain.Node, error) {
	state := strings.TrimSpace(desiredState)
	if !domain.IsValidNodeState(state) {
		return nil, fmt.Errorf("%w: unsupported desiredState %q", domain.ErrInvalidInput, state)
	}

	if s.quotaRepo != nil {
		if err := s.quotaRepo.UpdateAutoState(ctx, id, state, domain.AutoReasonManual); err != nil {
			return nil, err
		}
	} else {
		node, err := s.nodeRepo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		node.DesiredState = state
		node.AutoStateReason = domain.AutoReasonManual
		node.UpdatedAt = time.Now()
		if err := s.nodeRepo.Update(ctx, node); err != nil {
			return nil, err
		}
	}

	updated, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	updated.RefreshComputed(time.Now())
	return updated, nil
}

// ResetNodeCycleTraffic zeros a node's cycle counters and restores active state if auto-degraded.
func (s *NodeService) ResetNodeCycleTraffic(ctx context.Context, id uint) (*domain.Node, error) {
	if s.quotaRepo == nil {
		return nil, fmt.Errorf("node quota repository is not configured")
	}
	if err := s.quotaRepo.ResetCycleTraffic(ctx, id, 0, true); err != nil {
		return nil, err
	}
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	node.RefreshComputed(time.Now())
	return node, nil
}

// CheckAndResetMonthlyTraffic checks all nodes against their billing reset day and resets cycle counters when due.
func (s *NodeService) CheckAndResetMonthlyTraffic(ctx context.Context) error {
	return s.CheckAndResetMonthlyTrafficAt(ctx, time.Now())
}

// CheckAndResetMonthlyTrafficAt executes monthly billing cycle resets relative to the provided timestamp.
func (s *NodeService) CheckAndResetMonthlyTrafficAt(ctx context.Context, now time.Time) error {
	if s.quotaRepo == nil {
		return nil
	}

	today := now.Day()
	currentYearMonth := now.Year()*100 + int(now.Month())
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()

	nodes, err := s.nodeRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, n := range nodes {
		if n.BillingResetDay <= 0 {
			continue
		}
		effectiveResetDay := n.BillingResetDay
		if effectiveResetDay > lastDayOfMonth {
			effectiveResetDay = lastDayOfMonth
		}
		if today >= effectiveResetDay && n.LastResetMonth != currentYearMonth {
			oldState := n.DesiredState
			wasAutoDegraded := domain.IsAutoQuotaStateReason(n.AutoStateReason) && oldState != domain.NodeStateActive
			if err := s.quotaRepo.ResetCycleTraffic(ctx, n.ID, currentYearMonth, true); err != nil {
				errs = append(errs, fmt.Errorf("reset node %d cycle traffic failed: %w", n.ID, err))
				continue
			}
			if wasAutoDegraded {
				updated, getErr := s.nodeRepo.GetByID(ctx, n.ID)
				if getErr == nil && updated != nil {
					updated.RefreshComputed(now)
					s.notifyStateChange(ctx, updated, oldState, updated.DesiredState, "monthly_reset_recovered")
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (s *NodeService) notifyStateChange(ctx context.Context, node *domain.Node, oldState, newState, reason string) {
	if s.stateNotifier == nil || node == nil {
		return
	}
	s.stateNotifier(ctx, node, oldState, newState, reason)
}

func (s *NodeService) DeleteNode(ctx context.Context, id uint) error {
	if _, err := s.nodeRepo.GetByID(ctx, id); err != nil {
		return err
	}

	count, err := s.nodeRepo.CountInbounds(ctx, id)
	if err != nil {
		return fmt.Errorf("count bound inbounds failed: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: node %d still has %d bound inbound(s)", domain.ErrNodeHasInbounds, id, count)
	}

	if err := s.nodeRepo.Delete(ctx, id); err != nil {
		return err
	}
	s.InvalidateNode(id)
	return nil
}

func (s *NodeService) RotateToken(ctx context.Context, id uint) (*domain.RotateTokenResponse, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	plain, hash, prefix, err := generateNodeToken()
	if err != nil {
		return nil, err
	}

	node.TokenHash = hash
	node.TokenPrefix = prefix
	node.UpdatedAt = time.Now()
	if err := s.nodeRepo.Update(ctx, node); err != nil {
		return nil, fmt.Errorf("rotate node token failed: %w", err)
	}

	return &domain.RotateTokenResponse{
		NodeID:      node.ID,
		TokenPrefix: prefix,
		Token:       plain,
	}, nil
}

func (s *NodeService) AuthenticateAgent(ctx context.Context, plainToken string) (*domain.Node, error) {
	token := strings.TrimSpace(plainToken)
	if token == "" {
		return nil, domain.ErrUnauthorized
	}

	hash := HashNodeToken(token)
	node, err := s.nodeRepo.GetByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if !node.Enabled {
		return nil, domain.ErrForbidden
	}
	return node, nil
}

func sanitizeMasterURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "https://YOUR_PANEL_DOMAIN", nil
	}
	if strings.ContainsAny(trimmed, "\r\n'\"\\`$;|&<>") {
		return "", fmt.Errorf("%w: invalid masterUrl characters", domain.ErrInvalidInput)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("%w: masterUrl must be a valid http/https URL", domain.ErrInvalidInput)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// RenderInstallScript renders a systemd deployment template without embedding plaintext tokens or hardcoded hosts.
func (s *NodeService) RenderInstallScript(ctx context.Context, id uint, masterURL string) (string, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	safeURL, err := sanitizeMasterURL(masterURL)
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

# Node: %s (ID: %d, TokenPrefix: %s)
MASTER_URL="%s"
NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"

if [ "$NODE_TOKEN" = "<YOUR_NODE_TOKEN>" ]; then
  echo "Error: set NODE_TOKEN to the one-time token issued for this node." >&2
  exit 1
fi

install -d -m 0750 /var/lib/xray-agent
cat >/etc/systemd/system/xray-agent.service <<EOF
[Unit]
Description=Xray Panel Thin Agent
After=network-online.target xray.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/xray-agent --master-url ${MASTER_URL} --token ${NODE_TOKEN} --spool-dir /var/lib/xray-agent --interval 20s
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

chmod 0600 /etc/systemd/system/xray-agent.service
systemctl daemon-reload
systemctl enable --now xray-agent
`, strings.ReplaceAll(node.Name, "\n", " "), node.ID, node.TokenPrefix, safeURL)

	return script, nil
}

// HandleAgentSync processes agent heartbeat metrics, idempotent traffic batches, and hash-short-circuited sync payloads.
func (s *NodeService) HandleAgentSync(ctx context.Context, nodeID uint, req *domain.AgentSyncRequest) (*domain.AgentSyncResponse, error) {
	if req == nil {
		req = &domain.AgentSyncRequest{}
	}

	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if !node.Enabled {
		return nil, domain.ErrForbidden
	}

	if req.TrafficBatch != nil && strings.TrimSpace(req.TrafficBatch.BatchID) != "" && s.batchRepo != nil {
		date := time.Now().Format("2006-01-02")
		applied, affectedUsers, err := s.batchRepo.ApplyNodeTrafficBatch(ctx, nodeID, req.TrafficBatch, date)
		if err != nil {
			return nil, fmt.Errorf("apply node traffic batch failed: %w", err)
		}
		if applied {
			for _, u := range affectedUsers {
				if !u.IsActive() {
					// A user crossed quota on this batch; invalidate immediately so this response drops them.
					s.InvalidateAll()
					break
				}
			}
		}
	}

	now := time.Now()
	if err := s.nodeRepo.UpdateHeartbeat(ctx, nodeID, req, now); err != nil {
		return nil, fmt.Errorf("update node heartbeat failed: %w", err)
	}
	if strings.TrimSpace(req.LastError) != "" {
		slog.Warn("Remote node agent reported sync error",
			slog.Uint64("node_id", uint64(nodeID)),
			slog.String("last_error", req.LastError),
		)
	}

	// Re-read latest node after batch & heartbeat increments so quota evaluation sees updated cycle counters.
	latestNode, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	latestNode.RefreshComputed(now)

	oldState := latestNode.DesiredState
	if oldState == "" {
		oldState = domain.NodeStateActive
	}
	desiredState := oldState
	usedBytes := latestNode.CycleUsedBytes
	_, drainPct, breakPct := latestNode.EffectiveThresholds()

	if latestNode.TrafficLimitBytes > 0 && latestNode.AutoStateReason != domain.AutoReasonManual && s.quotaRepo != nil {
		switch {
		case domain.ReachedQuotaThreshold(usedBytes, latestNode.TrafficLimitBytes, breakPct):
			if oldState != domain.NodeStateCircuitBroken {
				if err := s.quotaRepo.UpdateAutoState(ctx, nodeID, domain.NodeStateCircuitBroken, domain.AutoReasonBreakQuota); err != nil {
					return nil, fmt.Errorf("update node auto break state failed: %w", err)
				}
				desiredState = domain.NodeStateCircuitBroken
				latestNode.DesiredState = desiredState
				latestNode.AutoStateReason = domain.AutoReasonBreakQuota
				s.notifyStateChange(ctx, latestNode, oldState, desiredState, domain.AutoReasonBreakQuota)
			}
		case domain.ReachedQuotaThreshold(usedBytes, latestNode.TrafficLimitBytes, drainPct):
			if oldState == domain.NodeStateActive {
				if err := s.quotaRepo.UpdateAutoState(ctx, nodeID, domain.NodeStateDraining, domain.AutoReasonDrainQuota); err != nil {
					return nil, fmt.Errorf("update node auto drain state failed: %w", err)
				}
				desiredState = domain.NodeStateDraining
				latestNode.DesiredState = desiredState
				latestNode.AutoStateReason = domain.AutoReasonDrainQuota
				s.notifyStateChange(ctx, latestNode, oldState, desiredState, domain.AutoReasonDrainQuota)
			}
		}
	}

	snap, err := s.getOrBuildSnapshot(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	configUnchanged := req.ConfigHash != "" && req.ConfigHash == snap.ConfigHash
	usersUnchanged := req.UsersHash != "" && req.UsersHash == snap.UsersHash

	cycleMonth := latestNode.LastResetMonth
	if cycleMonth <= 0 {
		cycleMonth = now.Year()*100 + int(now.Month())
	}

	resp := &domain.AgentSyncResponse{
		OK:              true,
		DesiredState:    desiredState,
		SyncIntervalSec: s.syncIntervalSec,
		ConfigHash:      snap.ConfigHash,
		UsersHash:       snap.UsersHash,
		ConfigUnchanged: configUnchanged,
		UsersUnchanged:  usersUnchanged,
		QuotaPolicy: &domain.AgentQuotaPolicy{
			TrafficLimitBytes: latestNode.TrafficLimitBytes,
			BreakThresholdPct: breakPct,
			CountMode:         latestNode.EffectiveTrafficCountMode(),
			Multiplier:        latestNode.EffectiveMultiplier(),
			CycleUsedBytes:    usedBytes,
			CycleMonth:        cycleMonth,
		},
	}
	if !configUnchanged {
		resp.ConfigJSON = snap.ConfigJSON
	}
	if !usersUnchanged {
		resp.Users = snap.Users
	}

	return resp, nil
}

func (s *NodeService) findNodeInbounds(ctx context.Context, nodeID uint) ([]domain.Inbound, error) {
	if s.inboundRepo == nil {
		return nil, nil
	}
	if finder, ok := s.inboundRepo.(nodeInboundFinder); ok {
		return finder.FindByNodeID(ctx, nodeID)
	}
	all, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]domain.Inbound, 0)
	for _, ib := range all {
		if ib.NodeID == nodeID {
			matched = append(matched, ib)
		}
	}
	return matched, nil
}

func (s *NodeService) getOrBuildSnapshot(ctx context.Context, nodeID uint) (*cachedNodeSnapshot, error) {
	s.mu.RLock()
	if snap, ok := s.snapshots[nodeID]; ok {
		s.mu.RUnlock()
		return snap, nil
	}
	startGlobalEpoch := s.globalEpoch
	startNodeEpoch := s.nodeEpochs[nodeID]
	s.mu.RUnlock()

	nodeInbounds, err := s.findNodeInbounds(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("load node %d inbounds failed: %w", nodeID, err)
	}

	var allUsers []domain.User
	if s.userRepo != nil {
		allUsers, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("load users for node %d snapshot failed: %w", nodeID, err)
		}
	}

	if s.compiler == nil {
		return nil, fmt.Errorf("remote node compiler is not configured")
	}

	// ConfigHash covers structural node/inbound settings (excluding dynamic users)
	// so user mutations trigger gRPC AddUser/RemoveUser without restarting Xray.
	structuralBytes, err := s.compiler.CompileRemoteNodeToJSON(nodeInbounds, nil)
	if err != nil {
		return nil, fmt.Errorf("compile structural config for node %d failed: %w", nodeID, err)
	}
	fullConfigBytes, err := s.compiler.CompileRemoteNodeToJSON(nodeInbounds, allUsers)
	if err != nil {
		return nil, fmt.Errorf("compile full config for node %d failed: %w", nodeID, err)
	}
	syncUsers, err := s.compiler.BuildRemoteNodeUsers(nodeInbounds, allUsers)
	if err != nil {
		return nil, fmt.Errorf("build sync users for node %d failed: %w", nodeID, err)
	}
	usersBytes, err := json.Marshal(syncUsers)
	if err != nil {
		return nil, fmt.Errorf("marshal sync users for node %d failed: %w", nodeID, err)
	}

	snap := &cachedNodeSnapshot{
		ConfigHash: sha256HexBytes(structuralBytes),
		UsersHash:  sha256HexBytes(usersBytes),
		ConfigJSON: string(fullConfigBytes),
		Users:      syncUsers,
	}

	s.mu.Lock()
	if s.globalEpoch == startGlobalEpoch && s.nodeEpochs[nodeID] == startNodeEpoch {
		s.snapshots[nodeID] = snap
	}
	s.mu.Unlock()

	return snap, nil
}
