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
	"net/url"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
)

const defaultNodeSyncIntervalSec = 20

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
	inboundRepo     domain.InboundRepository
	userRepo        domain.UserRepository
	compiler        RemoteNodeCompiler
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
	return &NodeService{
		nodeRepo:        nodeRepo,
		batchRepo:       batchRepo,
		inboundRepo:     inboundRepo,
		userRepo:        userRepo,
		compiler:        compiler,
		syncIntervalSec: defaultNodeSyncIntervalSec,
		snapshots:       make(map[uint]*cachedNodeSnapshot),
		nodeEpochs:      make(map[uint]uint64),
	}
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

	plain, hash, prefix, err := generateNodeToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	node := &domain.Node{
		Name:         name,
		Host:         host,
		Region:       strings.TrimSpace(dto.Region),
		TokenHash:    hash,
		TokenPrefix:  prefix,
		Enabled:      enabled,
		DesiredState: desiredState,
		Remark:       strings.TrimSpace(dto.Remark),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, fmt.Errorf("create node failed: %w", err)
	}

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
		node.DesiredState = state
	}
	if dto.Remark != nil {
		node.Remark = strings.TrimSpace(*dto.Remark)
	}
	node.UpdatedAt = time.Now()

	if err := s.nodeRepo.Update(ctx, node); err != nil {
		return nil, fmt.Errorf("update node failed: %w", err)
	}

	node.RefreshOnline(time.Now())
	return node, nil
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

	if err := s.nodeRepo.UpdateHeartbeat(ctx, nodeID, req, time.Now()); err != nil {
		return nil, fmt.Errorf("update node heartbeat failed: %w", err)
	}
	if strings.TrimSpace(req.LastError) != "" {
		slog.Warn("Remote node agent reported sync error",
			slog.Uint64("node_id", uint64(nodeID)),
			slog.String("last_error", req.LastError),
		)
	}

	snap, err := s.getOrBuildSnapshot(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	desiredState := node.DesiredState
	if desiredState == "" {
		desiredState = domain.NodeStateActive
	}

	configUnchanged := req.ConfigHash != "" && req.ConfigHash == snap.ConfigHash
	usersUnchanged := req.UsersHash != "" && req.UsersHash == snap.UsersHash

	resp := &domain.AgentSyncResponse{
		OK:              true,
		DesiredState:    desiredState,
		SyncIntervalSec: s.syncIntervalSec,
		ConfigHash:      snap.ConfigHash,
		UsersHash:       snap.UsersHash,
		ConfigUnchanged: configUnchanged,
		UsersUnchanged:  usersUnchanged,
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
