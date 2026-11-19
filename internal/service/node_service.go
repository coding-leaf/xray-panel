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
	"sort"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
)

const (
	defaultNodeSyncIntervalSec = 60
	defaultRemoteLogCacheTTL   = 30 * time.Second
	defaultRemoteLogWaitTimout = 25 * time.Second
)

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

type cachedRemoteLog struct {
	Service         string
	Lines           []string
	RequestedLines  int
	Error           string
	CollectedAtUnix int64
	CachedAt        time.Time
}

// RemoteNodeLogsResponse represents the HTTP/service response payload for remote on-demand logs.
type RemoteNodeLogsResponse struct {
	NodeID      uint     `json:"nodeId"`
	Service     string   `json:"service"`
	Lines       []string `json:"lines"`
	Cached      bool     `json:"cached"`
	Pending     bool     `json:"pending"`
	CollectedAt int64    `json:"collectedAt"`
	Error       string   `json:"error"`
}

// NodeService manages remote direct-connect nodes, tokens, sync snapshots, and traffic ingestion.
type NodeService struct {
	nodeRepo               domain.NodeRepository
	batchRepo              domain.NodeTrafficBatchRepository
	quotaRepo              domain.NodeQuotaRepository
	inboundRepo            domain.InboundRepository
	userRepo               domain.UserRepository
	compiler               RemoteNodeCompiler
	stateNotifier          NodeStateChangeCallback
	localOutboundsProvider func(context.Context) ([]domain.Outbound, error)
	syncIntervalSec        int
	logWaitTimeout         time.Duration

	mu             sync.RWMutex
	snapshots      map[uint]*cachedNodeSnapshot
	globalEpoch    uint64
	nodeEpochs     map[uint]uint64
	pendingLogReqs map[uint]*domain.AgentLogRequest
	logCache       map[string]*cachedRemoteLog
	logWaiters     map[string][]chan *cachedRemoteLog
	logReqLines    map[string]int
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
		logWaitTimeout:  defaultRemoteLogWaitTimout,
		snapshots:       make(map[uint]*cachedNodeSnapshot),
		nodeEpochs:      make(map[uint]uint64),
		pendingLogReqs:  make(map[uint]*domain.AgentLogRequest),
		logCache:        make(map[string]*cachedRemoteLog),
		logWaiters:      make(map[string][]chan *cachedRemoteLog),
		logReqLines:     make(map[string]int),
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

// WithLocalOutboundsProvider injects a provider returning Master local outbounds for static relay credential injection.
func (s *NodeService) WithLocalOutboundsProvider(fn func(context.Context) ([]domain.Outbound, error)) *NodeService {
	s.localOutboundsProvider = fn
	return s
}

// WithLogWaitTimeout overrides the wait timeout for on-demand remote log collection (primarily for tests).
func (s *NodeService) WithLogWaitTimeout(d time.Duration) *NodeService {
	if d > 0 {
		s.logWaitTimeout = d
	}
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
	prevHost := node.Host
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
	if !strings.EqualFold(strings.TrimSpace(prevHost), strings.TrimSpace(node.Host)) {
		s.InvalidateNode(id)
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
	if req.LogResult != nil {
		s.handleIncomingLogResult(nodeID, req.LogResult, now)
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
	resp.LogRequest = s.popPendingLogRequest(nodeID, req.LogResult)

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

	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("load node %d failed: %w", nodeID, err)
	}
	outbounds, err := parseNodeOutbounds(node.OutboundsJSON, false)
	if err != nil {
		return nil, fmt.Errorf("parse node %d outbounds failed: %w", nodeID, err)
	}
	routing, err := parseNodeRouting(node.RoutingJSON)
	if err != nil {
		return nil, fmt.Errorf("parse node %d routing failed: %w", nodeID, err)
	}
	dns, err := parseNodeDNS(node.DNSJSON)
	if err != nil {
		return nil, fmt.Errorf("parse node %d dns failed: %w", nodeID, err)
	}

	var staticClients []domain.StaticInboundClient
	if s.localOutboundsProvider != nil {
		localOutbounds, err := s.localOutboundsProvider(ctx)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("load local outbounds for node %d relay injection failed: %w", nodeID, err)
		}
		if err == nil {
			staticClients = extractStaticRelayClients(node, nodeInbounds, localOutbounds)
		}
	}

	// ConfigHash covers structural node/inbound settings, custom node config, and static relay clients (excluding dynamic users)
	// so user mutations trigger gRPC AddUser/RemoveUser without restarting Xray.
	var structuralBytes, fullConfigBytes []byte
	if fullCompiler, ok := s.compiler.(RemoteNodeFullCompiler); ok {
		structuralBytes, err = fullCompiler.CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, nil, staticClients)
		if err != nil {
			return nil, fmt.Errorf("compile structural config for node %d failed: %w", nodeID, err)
		}
		fullConfigBytes, err = fullCompiler.CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, allUsers, staticClients)
		if err != nil {
			return nil, fmt.Errorf("compile full config for node %d failed: %w", nodeID, err)
		}
	} else {
		structuralBytes, err = s.compiler.CompileRemoteNodeToJSON(nodeInbounds, nil)
		if err != nil {
			return nil, fmt.Errorf("compile structural config for node %d failed: %w", nodeID, err)
		}
		fullConfigBytes, err = s.compiler.CompileRemoteNodeToJSON(nodeInbounds, allUsers)
		if err != nil {
			return nil, fmt.Errorf("compile full config for node %d failed: %w", nodeID, err)
		}
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

func isValidRemoteLogService(service string) bool {
	return service == "xray" || service == "xray-agent"
}

func remoteLogCacheKey(nodeID uint, service string) string {
	return fmt.Sprintf("%d:%s", nodeID, service)
}

func nonNilLines(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}

func tailLines(lines []string, maxLines int) []string {
	if len(lines) == 0 {
		return []string{}
	}
	if maxLines <= 0 || len(lines) <= maxLines {
		out := make([]string, len(lines))
		copy(out, lines)
		return out
	}
	out := make([]string, maxLines)
	copy(out, lines[len(lines)-maxLines:])
	return out
}

func generateLogRequestID() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("log-%d-%s", time.Now().UnixNano(), hex.EncodeToString(buf))
}

// RequestNodeLogs fetches on-demand service logs from a remote node using 30s cache and waiter wakeup.
func (s *NodeService) RequestNodeLogs(
	ctx context.Context,
	nodeID uint,
	svcName string,
	lines int,
	forceRefresh bool,
) (*RemoteNodeLogsResponse, error) {
	if nodeID == 0 {
		return nil, fmt.Errorf("%w: nodeId must be > 0", domain.ErrInvalidInput)
	}
	svc := strings.TrimSpace(svcName)
	if svc == "" {
		svc = "xray"
	}
	if !isValidRemoteLogService(svc) {
		return nil, fmt.Errorf("%w: unsupported log service %q (allowed: xray, xray-agent)", domain.ErrInvalidInput, svc)
	}
	if lines <= 0 {
		lines = 100
	}
	if lines > 1000 {
		lines = 1000
	}

	if _, err := s.nodeRepo.GetByID(ctx, nodeID); err != nil {
		return nil, err
	}

	cacheKey := remoteLogCacheKey(nodeID, svc)
	now := time.Now()

	s.mu.Lock()
	if !forceRefresh {
		if cached, ok := s.logCache[cacheKey]; ok && now.Sub(cached.CachedAt) <= defaultRemoteLogCacheTTL && cached.RequestedLines >= lines {
			resp := &RemoteNodeLogsResponse{
				NodeID:      nodeID,
				Service:     svc,
				Lines:       tailLines(cached.Lines, lines),
				Cached:      true,
				Pending:     false,
				CollectedAt: cached.CollectedAtUnix,
				Error:       cached.Error,
			}
			s.mu.Unlock()
			return resp, nil
		}
	}

	reqID := generateLogRequestID()
	waitCh := make(chan *cachedRemoteLog, 1)
	s.pendingLogReqs[nodeID] = &domain.AgentLogRequest{
		RequestID: reqID,
		Service:   svc,
		Lines:     lines,
	}
	s.logReqLines[reqID] = lines
	s.logWaiters[reqID] = append(s.logWaiters[reqID], waitCh)
	waitTimeout := s.logWaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = defaultRemoteLogWaitTimout
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.removeLogWaiterLocked(reqID, waitCh)
		if len(s.logWaiters[reqID]) == 0 {
			delete(s.logReqLines, reqID)
		}
		if cur, ok := s.pendingLogReqs[nodeID]; ok && cur.RequestID == reqID {
			delete(s.pendingLogReqs, nodeID)
		}
		s.mu.Unlock()
	}()

	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()

	select {
	case res := <-waitCh:
		return &RemoteNodeLogsResponse{
			NodeID:      nodeID,
			Service:     svc,
			Lines:       tailLines(res.Lines, lines),
			Cached:      false,
			Pending:     false,
			CollectedAt: res.CollectedAtUnix,
			Error:       res.Error,
		}, nil
	case <-timer.C:
		return &RemoteNodeLogsResponse{
			NodeID:  nodeID,
			Service: svc,
			Lines:   []string{},
			Cached:  false,
			Pending: true,
			Error:   "timed out waiting for agent log response",
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *NodeService) removeLogWaiterLocked(reqID string, target chan *cachedRemoteLog) {
	list := s.logWaiters[reqID]
	if len(list) == 0 {
		return
	}
	filtered := list[:0]
	for _, ch := range list {
		if ch != target {
			filtered = append(filtered, ch)
		}
	}
	if len(filtered) == 0 {
		delete(s.logWaiters, reqID)
	} else {
		s.logWaiters[reqID] = filtered
	}
}

func (s *NodeService) handleIncomingLogResult(nodeID uint, res *domain.AgentLogResult, now time.Time) {
	if res == nil {
		return
	}
	svc := strings.TrimSpace(res.Service)
	if !isValidRemoteLogService(svc) {
		return
	}
	linesCopy := make([]string, len(res.Lines))
	copy(linesCopy, res.Lines)

	s.mu.Lock()
	reqLines := s.logReqLines[res.RequestID]
	if reqLines <= 0 {
		reqLines = len(linesCopy)
	}
	delete(s.logReqLines, res.RequestID)

	entry := &cachedRemoteLog{
		Service:         svc,
		Lines:           linesCopy,
		RequestedLines:  reqLines,
		Error:           res.Error,
		CollectedAtUnix: res.CollectedAtUnix,
		CachedAt:        now,
	}
	s.logCache[remoteLogCacheKey(nodeID, svc)] = entry

	waiters := s.logWaiters[res.RequestID]
	delete(s.logWaiters, res.RequestID)
	for _, ch := range waiters {
		select {
		case ch <- entry:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *NodeService) popPendingLogRequest(nodeID uint, ack *domain.AgentLogResult) *domain.AgentLogRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pendingLogReqs[nodeID]
	if !ok || pending == nil {
		return nil
	}
	if ack != nil && ack.RequestID == pending.RequestID {
		delete(s.pendingLogReqs, nodeID)
		return nil
	}
	delete(s.pendingLogReqs, nodeID)
	return pending
}

func parseNodeOutbounds(raw string, withDefaults bool) ([]domain.Outbound, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if !withDefaults {
			return nil, nil
		}
		return []domain.Outbound{
			{Tag: "direct", Protocol: "freedom"},
			{Tag: "block", Protocol: "blackhole"},
		}, nil
	}
	var outbounds []domain.Outbound
	if err := json.Unmarshal([]byte(trimmed), &outbounds); err != nil {
		return nil, err
	}
	return outbounds, nil
}

func parseNodeRouting(raw string) (*domain.RoutingConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var cfg domain.RoutingConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func parseNodeDNS(raw string) (*domain.DNSConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var cfg domain.DNSConfig
	if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func normalizeHostForMatch(host string) string {
	h := strings.TrimSpace(host)
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	return strings.TrimSpace(h)
}

// extractStaticRelayClients matches local VLESS/VMess/Trojan outbounds pointing to node.Host and a remote inbound's Port/ExternalPort,
// preserving each outbound's exact flow value without synthesizing vision flow.
func extractStaticRelayClients(node *domain.Node, nodeInbounds []domain.Inbound, localOutbounds []domain.Outbound) []domain.StaticInboundClient {
	if node == nil || len(nodeInbounds) == 0 || len(localOutbounds) == 0 {
		return nil
	}
	nodeHost := normalizeHostForMatch(node.Host)
	if nodeHost == "" {
		return nil
	}

	var result []domain.StaticInboundClient
	for _, ob := range localOutbounds {
		proto := strings.ToLower(strings.TrimSpace(ob.Protocol))
		if proto != "vless" && proto != "vmess" && proto != "trojan" {
			continue
		}
		settingsStr := strings.TrimSpace(ob.SettingsJSON)
		if settingsStr == "" || settingsStr == "{}" || settingsStr == "null" {
			continue
		}

		var settings struct {
			Vnext []struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Users   []struct {
					ID   string `json:"id"`
					Flow string `json:"flow"`
				} `json:"users"`
			} `json:"vnext"`
			Servers []struct {
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Password string `json:"password"`
			} `json:"servers"`
		}
		if err := json.Unmarshal([]byte(settingsStr), &settings); err != nil {
			continue
		}

		var addr string
		var port int
		var credID string
		var flow string

		switch proto {
		case "vless", "vmess":
			if len(settings.Vnext) == 0 || len(settings.Vnext[0].Users) == 0 {
				continue
			}
			addr = normalizeHostForMatch(settings.Vnext[0].Address)
			port = settings.Vnext[0].Port
			credID = strings.TrimSpace(settings.Vnext[0].Users[0].ID)
			if proto == "vless" {
				flow = strings.TrimSpace(settings.Vnext[0].Users[0].Flow)
				if strings.EqualFold(flow, "none") {
					flow = ""
				}
			}
		case "trojan":
			if len(settings.Servers) == 0 {
				continue
			}
			addr = normalizeHostForMatch(settings.Servers[0].Address)
			port = settings.Servers[0].Port
			credID = strings.TrimSpace(settings.Servers[0].Password)
		}

		if addr == "" || port <= 0 || credID == "" {
			continue
		}
		if !strings.EqualFold(addr, nodeHost) {
			continue
		}

		for _, inb := range nodeInbounds {
			if !inb.Enabled || inb.Tag == "api" {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(inb.Protocol), proto) {
				continue
			}
			if port != inb.Port && !(inb.ExternalPort > 0 && port == inb.ExternalPort) {
				continue
			}
			result = append(result, domain.StaticInboundClient{
				InboundTag: inb.Tag,
				Protocol:   proto,
				ID:         credID,
				Flow:       flow,
				Email:      "relay-auto:" + ob.Tag + "@internal",
			})
		}
	}

	if len(result) > 1 {
		sort.Slice(result, func(i, j int) bool {
			if result[i].InboundTag != result[j].InboundTag {
				return result[i].InboundTag < result[j].InboundTag
			}
			if result[i].Email != result[j].Email {
				return result[i].Email < result[j].Email
			}
			return result[i].ID < result[j].ID
		})
	}
	return result
}
