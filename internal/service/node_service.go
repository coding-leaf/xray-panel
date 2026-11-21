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
	defaultNodeSyncIntervalSec  = 60
	defaultRemoteLogCacheTTL    = 30 * time.Second
	defaultNodeCommandTTL       = 180 * time.Second
	defaultNodeCommandResultTTL = 180 * time.Second
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
	CompileRemoteNodeFullToJSON(
		inbounds []domain.Inbound,
		outbounds []domain.Outbound,
		routing *domain.RoutingConfig,
		dns *domain.DNSConfig,
		users []domain.User,
		staticClients []domain.StaticInboundClient,
	) ([]byte, error)
	BuildRemoteNodeUsers(inbounds []domain.Inbound, users []domain.User) ([]domain.AgentSyncUser, error)
}

type cachedNodeSnapshot struct {
	ConfigHash     string
	FullConfigHash string
	UsersHash      string
	ConfigJSON     string
	Users          []domain.AgentSyncUser
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
	CommandID   string   `json:"commandId,omitempty"`
	Status      string   `json:"status,omitempty"`
	CollectedAt string   `json:"collectedAt,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// RemoteGeoUpdateResponse represents the HTTP/service response payload for remote on-demand GeoData updates.
type RemoteGeoUpdateResponse struct {
	NodeID      uint   `json:"nodeId"`
	Pending     bool   `json:"pending"`
	CommandID   string `json:"commandId,omitempty"`
	Status      string `json:"status,omitempty"`
	GeoIPSize   int64  `json:"geoipSize,omitempty"`
	GeoSiteSize int64  `json:"geositeSize,omitempty"`
	DurationMs  int64  `json:"durationMs,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Error       string `json:"error,omitempty"`
}

// NodeLogCommandResult holds the completed log payload of an async log_tail command.
type NodeLogCommandResult struct {
	Service     string   `json:"service"`
	Lines       []string `json:"lines"`
	CollectedAt string   `json:"collectedAt,omitempty"`
}

// NodeGeoCommandResult holds the completed update metadata of an async geodata_update command.
type NodeGeoCommandResult struct {
	GeoIPSize   int64  `json:"geoipSize"`
	GeoSiteSize int64  `json:"geositeSize"`
	DurationMs  int64  `json:"durationMs"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// NodeCommandSnapshot is the read-only HTTP/service view of an asynchronous node command.
type NodeCommandSnapshot struct {
	ID        string                `json:"id"`
	NodeID    uint                  `json:"nodeId"`
	Type      string                `json:"type"`
	Status    string                `json:"status"`
	CreatedAt time.Time             `json:"createdAt"`
	UpdatedAt time.Time             `json:"updatedAt"`
	Error     string                `json:"error,omitempty"`
	LogResult *NodeLogCommandResult `json:"logResult,omitempty"`
	GeoResult *NodeGeoCommandResult `json:"geoResult,omitempty"`
}

type nodeCommandRecord struct {
	Seq       uint64
	ID        string
	NodeID    uint
	Command   domain.AgentCommand
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	Error     string
	LogResult *NodeLogCommandResult
	GeoResult *NodeGeoCommandResult
}

// NodeService manages remote direct-connect nodes, tokens, sync snapshots, and traffic ingestion.
type NodeService struct {
	nodeRepo               domain.NodeRepository
	inboundRepo            domain.InboundRepository
	userRepo               domain.UserRepository
	compiler               RemoteNodeCompiler
	stateNotifier          NodeStateChangeCallback
	localOutboundsProvider func(context.Context) ([]domain.Outbound, error)
	syncIntervalSec        int
	commandTTL             time.Duration
	commandResultTTL       time.Duration
	nowFn                  func() time.Time

	mu             sync.RWMutex
	snapshots      map[uint]*cachedNodeSnapshot
	globalEpoch    uint64
	nodeEpochs     map[uint]uint64
	cmdSeq         uint64
	commands       map[string]*nodeCommandRecord
	activeCmdByKey map[string]string
	logCache       map[string]*cachedRemoteLog
}

func NewNodeService(
	nodeRepo domain.NodeRepository,
	inboundRepo domain.InboundRepository,
	userRepo domain.UserRepository,
	compiler RemoteNodeCompiler,
) *NodeService {
	return &NodeService{
		nodeRepo:         nodeRepo,
		inboundRepo:      inboundRepo,
		userRepo:         userRepo,
		compiler:         compiler,
		syncIntervalSec:  defaultNodeSyncIntervalSec,
		commandTTL:       defaultNodeCommandTTL,
		commandResultTTL: defaultNodeCommandResultTTL,
		nowFn:            time.Now,
		snapshots:        make(map[uint]*cachedNodeSnapshot),
		nodeEpochs:       make(map[uint]uint64),
		commands:         make(map[string]*nodeCommandRecord),
		activeCmdByKey:   make(map[string]string),
		logCache:         make(map[string]*cachedRemoteLog),
	}
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

// WithNowFunc overrides the clock source used by command TTL and log cache logic (primarily for deterministic tests).
func (s *NodeService) WithNowFunc(fn func() time.Time) *NodeService {
	if fn != nil {
		s.mu.Lock()
		s.nowFn = fn
		s.mu.Unlock()
	}
	return s
}

// WithCommandTTL overrides command execution and terminal result retention TTLs (primarily for tests).
func (s *NodeService) WithCommandTTL(commandTTL, resultTTL time.Duration) *NodeService {
	s.mu.Lock()
	if commandTTL > 0 {
		s.commandTTL = commandTTL
	}
	if resultTTL > 0 {
		s.commandResultTTL = resultTTL
	}
	s.mu.Unlock()
	return s
}

func (s *NodeService) nowLocked() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
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

func (s *NodeService) resolveExpectedConfigHash(ctx context.Context, nodeID uint) string {
	if s.compiler == nil {
		return ""
	}
	snap, err := s.getOrBuildSnapshot(ctx, nodeID)
	if err != nil {
		slog.Warn("Failed to resolve expected config hash for node",
			slog.Uint64("node_id", uint64(nodeID)),
			slog.String("error", err.Error()),
		)
		return ""
	}
	if snap == nil {
		return ""
	}
	return snap.ConfigHash
}

func (s *NodeService) ListNodes(ctx context.Context) ([]domain.Node, error) {
	nodes, err := s.nodeRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range nodes {
		nodes[i].RefreshOnline(now)
		nodes[i].RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, nodes[i].ID))
	}
	return nodes, nil
}

func (s *NodeService) GetNode(ctx context.Context, id uint) (*domain.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	node.RefreshOnline(time.Now())
	node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))
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
	node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))

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

	node.RefreshComputed(time.Now())
	node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))
	return node, nil
}

// SetNodeDesiredState manually updates a node's desired state and records reason as "manual".
func (s *NodeService) SetNodeDesiredState(ctx context.Context, id uint, desiredState string) (*domain.Node, error) {
	state := strings.TrimSpace(desiredState)
	if !domain.IsValidNodeState(state) {
		return nil, fmt.Errorf("%w: unsupported desiredState %q", domain.ErrInvalidInput, state)
	}

	if err := s.nodeRepo.UpdateAutoState(ctx, id, state, domain.AutoReasonManual); err != nil {
		return nil, err
	}

	updated, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	updated.RefreshComputed(time.Now())
	updated.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, updated.ID))
	return updated, nil
}

// ResetNodeCycleTraffic zeros a node's cycle counters and restores active state if auto-degraded.
func (s *NodeService) ResetNodeCycleTraffic(ctx context.Context, id uint) (*domain.Node, error) {
	if s.nodeRepo == nil {
		return nil, fmt.Errorf("node repository is not configured")
	}
	if err := s.nodeRepo.ResetCycleTraffic(ctx, id, 0, true); err != nil {
		return nil, err
	}
	node, err := s.nodeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	node.RefreshComputed(time.Now())
	node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))
	return node, nil
}

// CheckAndResetMonthlyTraffic checks all nodes against their billing reset day and resets cycle counters when due.
func (s *NodeService) CheckAndResetMonthlyTraffic(ctx context.Context) error {
	return s.CheckAndResetMonthlyTrafficAt(ctx, time.Now())
}

// CheckAndResetMonthlyTrafficAt executes monthly billing cycle resets relative to the provided timestamp.
func (s *NodeService) CheckAndResetMonthlyTrafficAt(ctx context.Context, now time.Time) error {
	if s.nodeRepo == nil {
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
			if err := s.nodeRepo.ResetCycleTraffic(ctx, n.ID, currentYearMonth, true); err != nil {
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
	s.purgeNodeMemoryState(id)
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
# Note: Compiled remote Xray configs bind the local gRPC API on 127.0.0.1:8080 (matching xray-agent default --xray-grpc).
MASTER_URL="%s"
NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"

if [ "$NODE_TOKEN" = "<YOUR_NODE_TOKEN>" ]; then
  echo "Error: set NODE_TOKEN to the one-time token issued for this node." >&2
  exit 1
fi

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported CPU architecture: ${ARCH_RAW} (supported: amd64, arm64)" >&2
    exit 1
    ;;
esac

if ! command -v xray >/dev/null 2>&1 && [ ! -x /usr/local/bin/xray ]; then
  echo "[WARN] xray binary not found in PATH or /usr/local/bin/xray. Please install Xray-core before expecting proxy traffic." >&2
fi

AGENT_BIN_URL="${AGENT_BIN_URL:-https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}}"
TMP_BIN="$(mktemp /tmp/xray-agent.XXXXXX)"
trap 'rm -f "$TMP_BIN"' EXIT

echo "Downloading xray-agent (${ARCH}) from ${AGENT_BIN_URL} ..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL --retry 3 --connect-timeout 15 "$AGENT_BIN_URL" -o "$TMP_BIN"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP_BIN" "$AGENT_BIN_URL"
else
  echo "Error: neither curl nor wget is installed." >&2
  exit 1
fi

if [ ! -s "$TMP_BIN" ]; then
  echo "Error: downloaded xray-agent binary is empty." >&2
  exit 1
fi

if systemctl is-active --quiet xray-agent 2>/dev/null; then
  systemctl stop xray-agent
fi
install -m 0755 "$TMP_BIN" /usr/local/bin/xray-agent

install -d -m 0750 /etc/xray-agent /var/lib/xray-agent
(
  umask 077
  cat >/etc/xray-agent/agent.env <<EOF
MASTER_URL=${MASTER_URL}
AGENT_TOKEN=${NODE_TOKEN}
EOF
)
chmod 0600 /etc/xray-agent/agent.env

cat >/etc/systemd/system/xray-agent.service <<EOF
[Unit]
Description=Xray Panel Thin Agent
After=network-online.target xray.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/xray-agent/agent.env
ExecStart=/usr/local/bin/xray-agent --spool-dir /var/lib/xray-agent --interval 60s
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

chmod 0644 /etc/systemd/system/xray-agent.service
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

	if req.TrafficBatch != nil && strings.TrimSpace(req.TrafficBatch.BatchID) != "" {
		date := time.Now().Format("2006-01-02")
		applied, affectedUsers, err := s.nodeRepo.ApplyNodeTrafficBatch(ctx, nodeID, req.TrafficBatch, date)
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
	for i := range req.CommandResults {
		res := req.CommandResults[i]
		if reportErr := s.ReportAgentCommandResult(ctx, nodeID, &res); reportErr != nil && !errors.Is(reportErr, domain.ErrNotFound) {
			slog.Warn("Failed to record agent fallback command result",
				slog.Uint64("node_id", uint64(nodeID)),
				slog.String("command_id", res.ID),
				slog.String("error", reportErr.Error()),
			)
		}
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

	if latestNode.TrafficLimitBytes > 0 && latestNode.AutoStateReason != domain.AutoReasonManual {
		switch {
		case domain.ReachedQuotaThreshold(usedBytes, latestNode.TrafficLimitBytes, breakPct):
			if oldState != domain.NodeStateCircuitBroken {
				if err := s.nodeRepo.UpdateAutoState(ctx, nodeID, domain.NodeStateCircuitBroken, domain.AutoReasonBreakQuota); err != nil {
					return nil, fmt.Errorf("update node auto break state failed: %w", err)
				}
				desiredState = domain.NodeStateCircuitBroken
				latestNode.DesiredState = desiredState
				latestNode.AutoStateReason = domain.AutoReasonBreakQuota
				s.notifyStateChange(ctx, latestNode, oldState, desiredState, domain.AutoReasonBreakQuota)
			}
		case domain.ReachedQuotaThreshold(usedBytes, latestNode.TrafficLimitBytes, drainPct):
			if oldState == domain.NodeStateActive {
				if err := s.nodeRepo.UpdateAutoState(ctx, nodeID, domain.NodeStateDraining, domain.AutoReasonDrainQuota); err != nil {
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
	diskConfigStale := req.DiskConfigHash != "" && req.DiskConfigHash != snap.FullConfigHash

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
	if !configUnchanged || diskConfigStale {
		resp.ConfigJSON = snap.ConfigJSON
	}
	if !usersUnchanged {
		resp.Users = snap.Users
	}
	s.mu.Lock()
	resp.Commands = s.popPendingCommandsLocked(nodeID, s.nowLocked())
	s.mu.Unlock()

	return resp, nil
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

	var nodeInbounds []domain.Inbound
	var err error
	if s.inboundRepo != nil {
		nodeInbounds, err = s.inboundRepo.FindByNodeID(ctx, nodeID)
		if err != nil {
			return nil, fmt.Errorf("load node %d inbounds failed: %w", nodeID, err)
		}
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
	rawRouting, err := parseNodeRouting(node.RoutingJSON)
	if err != nil {
		return nil, fmt.Errorf("parse node %d routing failed: %w", nodeID, err)
	}
	routing, err := normalizeNodeRouting(rawRouting, nodeInbounds)
	if err != nil {
		return nil, fmt.Errorf("normalize node %d routing failed: %w", nodeID, err)
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
	structuralBytes, err := s.compiler.CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, nil, staticClients)
	if err != nil {
		return nil, fmt.Errorf("compile structural config for node %d failed: %w", nodeID, err)
	}
	fullConfigBytes, err := s.compiler.CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, allUsers, staticClients)
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
		ConfigHash:     sha256HexBytes(structuralBytes),
		FullConfigHash: sha256HexBytes(fullConfigBytes),
		UsersHash:      sha256HexBytes(usersBytes),
		ConfigJSON:     string(fullConfigBytes),
		Users:          syncUsers,
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

func commandDedupKey(cmdType string, nodeID uint, service string) string {
	if cmdType == domain.NodeCommandTypeLogTail {
		return fmt.Sprintf("%s:%d:%s", cmdType, nodeID, service)
	}
	return fmt.Sprintf("%s:%d", cmdType, nodeID)
}

func isActiveCommandStatus(status string) bool {
	return status == domain.NodeCommandStatusQueued || status == domain.NodeCommandStatusDispatched
}

func isTerminalCommandStatus(status string) bool {
	return status == domain.NodeCommandStatusSucceeded ||
		status == domain.NodeCommandStatusFailed ||
		status == domain.NodeCommandStatusExpired
}

func nonNilLines(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	out := make([]string, len(lines))
	copy(out, lines)
	return out
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

func formatUnixRFC3339(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func generateCommandID(now time.Time) string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("cmd-%d", now.UnixNano())
	}
	return fmt.Sprintf("cmd-%d-%s", now.UnixNano(), hex.EncodeToString(buf))
}

func (s *NodeService) effectiveCommandTTLsLocked() (time.Duration, time.Duration) {
	cmdTTL := s.commandTTL
	if cmdTTL <= 0 {
		cmdTTL = defaultNodeCommandTTL
	}
	resTTL := s.commandResultTTL
	if resTTL <= 0 {
		resTTL = defaultNodeCommandResultTTL
	}
	return cmdTTL, resTTL
}

func (s *NodeService) pruneExpiredCommandsLocked(now time.Time) {
	cmdTTL, resTTL := s.effectiveCommandTTLsLocked()
	for id, rec := range s.commands {
		if rec == nil {
			delete(s.commands, id)
			continue
		}
		if isActiveCommandStatus(rec.Status) && now.Sub(rec.CreatedAt) > cmdTTL {
			rec.Status = domain.NodeCommandStatusExpired
			rec.UpdatedAt = now
			rec.Error = "command timed out waiting for agent"
			key := commandDedupKey(rec.Command.Type, rec.NodeID, rec.Command.Service)
			if s.activeCmdByKey[key] == id {
				delete(s.activeCmdByKey, key)
			}
		}
		if isTerminalCommandStatus(rec.Status) && now.Sub(rec.UpdatedAt) > resTTL {
			delete(s.commands, id)
		}
	}
	for key, id := range s.activeCmdByKey {
		rec, ok := s.commands[id]
		if !ok || rec == nil || !isActiveCommandStatus(rec.Status) {
			delete(s.activeCmdByKey, key)
		}
	}
}

func (s *NodeService) purgeNodeMemoryState(nodeID uint) {
	if s == nil || nodeID == 0 {
		return
	}
	logPrefix := fmt.Sprintf("%d:", nodeID)
	s.mu.Lock()
	s.nodeEpochs[nodeID]++
	delete(s.snapshots, nodeID)
	for id, rec := range s.commands {
		if rec == nil || rec.NodeID == nodeID {
			delete(s.commands, id)
		}
	}
	for key, id := range s.activeCmdByKey {
		if _, exists := s.commands[id]; !exists {
			delete(s.activeCmdByKey, key)
		}
	}
	for cacheKey := range s.logCache {
		if strings.HasPrefix(cacheKey, logPrefix) {
			delete(s.logCache, cacheKey)
		}
	}
	s.mu.Unlock()
}

// RequestNodeLogs returns cached remote logs when valid, or enqueues/reuses an async log_tail command immediately.
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
	dedupKey := commandDedupKey(domain.NodeCommandTypeLogTail, nodeID, svc)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.nowLocked()
	s.pruneExpiredCommandsLocked(now)

	if !forceRefresh {
		if cached, ok := s.logCache[cacheKey]; ok && now.Sub(cached.CachedAt) <= defaultRemoteLogCacheTTL && cached.RequestedLines >= lines {
			return &RemoteNodeLogsResponse{
				NodeID:      nodeID,
				Service:     svc,
				Lines:       tailLines(cached.Lines, lines),
				Cached:      true,
				Pending:     false,
				CollectedAt: formatUnixRFC3339(cached.CollectedAtUnix),
				Error:       cached.Error,
			}, nil
		}
	}

	if activeID, ok := s.activeCmdByKey[dedupKey]; ok {
		if rec, exists := s.commands[activeID]; exists && isActiveCommandStatus(rec.Status) {
			if rec.Status == domain.NodeCommandStatusQueued && rec.Command.Lines < lines {
				rec.Command.Lines = lines
				rec.UpdatedAt = now
			}
			return &RemoteNodeLogsResponse{
				NodeID:    nodeID,
				Service:   svc,
				Lines:     []string{},
				Cached:    false,
				Pending:   true,
				CommandID: rec.ID,
				Status:    rec.Status,
			}, nil
		}
	}

	s.cmdSeq++
	cmdID := generateCommandID(now)
	rec := &nodeCommandRecord{
		Seq:    s.cmdSeq,
		ID:     cmdID,
		NodeID: nodeID,
		Command: domain.AgentCommand{
			ID:      cmdID,
			Type:    domain.NodeCommandTypeLogTail,
			Service: svc,
			Lines:   lines,
		},
		Status:    domain.NodeCommandStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.commands[cmdID] = rec
	s.activeCmdByKey[dedupKey] = cmdID

	return &RemoteNodeLogsResponse{
		NodeID:    nodeID,
		Service:   svc,
		Lines:     []string{},
		Cached:    false,
		Pending:   true,
		CommandID: rec.ID,
		Status:    rec.Status,
	}, nil
}

// RequestNodeGeoUpdate enqueues or reuses an async geodata_update command for a remote node and returns immediately.
func (s *NodeService) RequestNodeGeoUpdate(ctx context.Context, nodeID uint) (*RemoteGeoUpdateResponse, error) {
	if nodeID == 0 {
		return nil, fmt.Errorf("%w: nodeId must be > 0", domain.ErrInvalidInput)
	}
	if _, err := s.nodeRepo.GetByID(ctx, nodeID); err != nil {
		return nil, err
	}

	dedupKey := commandDedupKey(domain.NodeCommandTypeGeoDataUpdate, nodeID, "")

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.nowLocked()
	s.pruneExpiredCommandsLocked(now)

	if activeID, ok := s.activeCmdByKey[dedupKey]; ok {
		if rec, exists := s.commands[activeID]; exists && isActiveCommandStatus(rec.Status) {
			return &RemoteGeoUpdateResponse{
				NodeID:    nodeID,
				Pending:   true,
				CommandID: rec.ID,
				Status:    rec.Status,
			}, nil
		}
	}

	s.cmdSeq++
	cmdID := generateCommandID(now)
	rec := &nodeCommandRecord{
		Seq:    s.cmdSeq,
		ID:     cmdID,
		NodeID: nodeID,
		Command: domain.AgentCommand{
			ID:   cmdID,
			Type: domain.NodeCommandTypeGeoDataUpdate,
		},
		Status:    domain.NodeCommandStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.commands[cmdID] = rec
	s.activeCmdByKey[dedupKey] = cmdID

	return &RemoteGeoUpdateResponse{
		NodeID:    nodeID,
		Pending:   true,
		CommandID: rec.ID,
		Status:    rec.Status,
	}, nil
}

func (s *NodeService) popPendingCommandsLocked(nodeID uint, now time.Time) []domain.AgentCommand {
	s.pruneExpiredCommandsLocked(now)

	var pending []*nodeCommandRecord
	for _, rec := range s.commands {
		if rec != nil && rec.NodeID == nodeID && rec.Status == domain.NodeCommandStatusQueued {
			pending = append(pending, rec)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	sort.Slice(pending, func(i, j int) bool {
		if pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].Seq < pending[j].Seq
		}
		return pending[i].CreatedAt.Before(pending[j].CreatedAt)
	})

	cmds := make([]domain.AgentCommand, 0, len(pending))
	for _, rec := range pending {
		rec.Status = domain.NodeCommandStatusDispatched
		rec.UpdatedAt = now
		cmds = append(cmds, rec.Command)
	}
	return cmds
}

// ReportAgentCommandResult records the terminal execution result reported by an authenticated node agent.
func (s *NodeService) ReportAgentCommandResult(_ context.Context, nodeID uint, res *domain.AgentCommandResult) error {
	if nodeID == 0 || res == nil || strings.TrimSpace(res.ID) == "" {
		return fmt.Errorf("%w: valid nodeId and command id are required", domain.ErrInvalidInput)
	}
	cmdID := strings.TrimSpace(res.ID)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.nowLocked()
	s.pruneExpiredCommandsLocked(now)

	rec, ok := s.commands[cmdID]
	if !ok || rec == nil || rec.NodeID != nodeID {
		return domain.ErrNotFound
	}
	if isTerminalCommandStatus(rec.Status) {
		return nil
	}

	status := strings.TrimSpace(res.Status)
	if status != domain.NodeCommandStatusSucceeded && status != domain.NodeCommandStatusFailed {
		status = domain.NodeCommandStatusFailed
	}

	rec.Status = status
	rec.Error = res.Error
	rec.UpdatedAt = now

	dedupKey := commandDedupKey(rec.Command.Type, rec.NodeID, rec.Command.Service)
	if s.activeCmdByKey[dedupKey] == rec.ID {
		delete(s.activeCmdByKey, dedupKey)
	}

	if status == domain.NodeCommandStatusSucceeded {
		switch rec.Command.Type {
		case domain.NodeCommandTypeLogTail:
			svc := strings.TrimSpace(res.Service)
			if svc == "" {
				svc = rec.Command.Service
			}
			collectedAt := res.CollectedAtUnix
			if collectedAt <= 0 {
				collectedAt = now.Unix()
			}
			linesCopy := nonNilLines(res.Lines)
			rec.LogResult = &NodeLogCommandResult{
				Service:     svc,
				Lines:       linesCopy,
				CollectedAt: formatUnixRFC3339(collectedAt),
			}
			if isValidRemoteLogService(svc) {
				reqLines := rec.Command.Lines
				if reqLines <= 0 {
					reqLines = len(linesCopy)
				}
				s.logCache[remoteLogCacheKey(nodeID, svc)] = &cachedRemoteLog{
					Service:         svc,
					Lines:           nonNilLines(linesCopy),
					RequestedLines:  reqLines,
					Error:           "",
					CollectedAtUnix: collectedAt,
					CachedAt:        now,
				}
			}
		case domain.NodeCommandTypeGeoDataUpdate:
			updatedAt := res.UpdatedAtUnix
			if updatedAt <= 0 {
				updatedAt = now.Unix()
			}
			rec.GeoResult = &NodeGeoCommandResult{
				GeoIPSize:   res.GeoIPSize,
				GeoSiteSize: res.GeoSiteSize,
				DurationMs:  res.DurationMs,
				UpdatedAt:   formatUnixRFC3339(updatedAt),
			}
		}
	}

	return nil
}

// GetNodeCommandStatus returns a read-only snapshot of an async node command scoped strictly to nodeID.
func (s *NodeService) GetNodeCommandStatus(_ context.Context, nodeID uint, commandID string) (*NodeCommandSnapshot, error) {
	if nodeID == 0 || strings.TrimSpace(commandID) == "" {
		return nil, fmt.Errorf("%w: valid nodeId and commandId are required", domain.ErrInvalidInput)
	}
	cmdID := strings.TrimSpace(commandID)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredCommandsLocked(s.nowLocked())

	rec, ok := s.commands[cmdID]
	if !ok || rec == nil || rec.NodeID != nodeID {
		return nil, domain.ErrNotFound
	}

	snap := &NodeCommandSnapshot{
		ID:        rec.ID,
		NodeID:    rec.NodeID,
		Type:      rec.Command.Type,
		Status:    rec.Status,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
		Error:     rec.Error,
	}
	if rec.LogResult != nil {
		snap.LogResult = &NodeLogCommandResult{
			Service:     rec.LogResult.Service,
			Lines:       nonNilLines(rec.LogResult.Lines),
			CollectedAt: rec.LogResult.CollectedAt,
		}
	}
	if rec.GeoResult != nil {
		geoCopy := *rec.GeoResult
		snap.GeoResult = &geoCopy
	}
	return snap, nil
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
