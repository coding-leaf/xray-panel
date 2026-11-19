package domain

import (
	"errors"
	"time"
)

const (
	NodeStateActive        = "active"
	NodeStateDraining      = "draining"
	NodeStateCircuitBroken = "circuit_broken"
)

var ErrNodeHasInbounds = errors.New("node still has bound inbounds")

// IsValidNodeState checks whether state is a supported desired node state.
func IsValidNodeState(state string) bool {
	switch state {
	case NodeStateActive, NodeStateDraining, NodeStateCircuitBroken:
		return true
	default:
		return false
	}
}

// Node represents a managed Xray execution node (NodeID == 0 denotes the local Master node).
type Node struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string     `gorm:"type:varchar(128);not null" json:"name"`
	Host           string     `gorm:"type:varchar(255);not null" json:"host"`
	Region         string     `gorm:"type:varchar(64);default:''" json:"region"`
	TokenHash      string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
	TokenPrefix    string     `gorm:"type:varchar(16);default:''" json:"tokenPrefix"`
	Enabled        bool       `gorm:"default:true" json:"enabled"`
	DesiredState   string     `gorm:"type:varchar(32);default:'active'" json:"desiredState"`
	LastSeenAt     *time.Time `json:"lastSeenAt"`
	AgentVersion   string     `gorm:"type:varchar(64);default:''" json:"agentVersion"`
	XrayVersion    string     `gorm:"type:varchar(64);default:''" json:"xrayVersion"`
	LastConfigHash string     `gorm:"type:varchar(64);default:''" json:"lastConfigHash"`
	LastUsersHash  string     `gorm:"type:varchar(64);default:''" json:"lastUsersHash"`
	LastSyncError  string     `gorm:"type:text;default:''" json:"lastSyncError"`
	CPUPercent     float64    `gorm:"default:0" json:"cpuPercent"`
	MemPercent     float64    `gorm:"default:0" json:"memPercent"`
	NicUpBytes     int64      `gorm:"default:0" json:"nicUpBytes"`
	NicDownBytes   int64      `gorm:"default:0" json:"nicDownBytes"`
	XrayUpBytes    int64      `gorm:"default:0" json:"xrayUpBytes"`
	XrayDownBytes  int64      `gorm:"default:0" json:"xrayDownBytes"`
	Remark         string     `gorm:"type:varchar(255);default:''" json:"remark"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`

	Online bool `gorm:"-" json:"online"` // 动态判定: LastSeenAt 距今 <= 90s
}

// RefreshOnline updates the transient Online status based on LastSeenAt relative to now.
func (n *Node) RefreshOnline(now time.Time) {
	if n == nil || n.LastSeenAt == nil {
		if n != nil {
			n.Online = false
		}
		return
	}
	n.Online = now.Sub(*n.LastSeenAt) <= 90*time.Second
}

// NodeTrafficBatch records processed agent traffic batch IDs for strict idempotency.
type NodeTrafficBatch struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	NodeID    uint      `gorm:"uniqueIndex:idx_node_batch;not null"`
	BatchID   string    `gorm:"type:varchar(96);uniqueIndex:idx_node_batch;not null"`
	CreatedAt time.Time `gorm:"index"`
}

// CreateNodeDTO is the admin payload for creating a managed node.
type CreateNodeDTO struct {
	Name         string `json:"name" binding:"required"`
	Host         string `json:"host" binding:"required"`
	Region       string `json:"region"`
	Enabled      *bool  `json:"enabled"`
	DesiredState string `json:"desiredState"`
	Remark       string `json:"remark"`
}

// CreateNodeResponse returns the created node along with its one-time plaintext token.
type CreateNodeResponse struct {
	Node
	Token string `json:"token"`
}

// UpdateNodeDTO is the admin payload for updating node metadata.
type UpdateNodeDTO struct {
	Name         *string `json:"name"`
	Host         *string `json:"host"`
	Region       *string `json:"region"`
	Enabled      *bool   `json:"enabled"`
	DesiredState *string `json:"desiredState"`
	Remark       *string `json:"remark"`
}

// RotateTokenResponse returns the newly issued one-time plaintext token for a node.
type RotateTokenResponse struct {
	NodeID      uint   `json:"nodeId"`
	TokenPrefix string `json:"tokenPrefix"`
	Token       string `json:"token"`
}

// AgentHostMetrics carries host resource and NIC traffic deltas from an agent.
type AgentHostMetrics struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemPercent   float64 `json:"memPercent"`
	NicUpDelta   int64   `json:"nicUpDelta"`
	NicDownDelta int64   `json:"nicDownDelta"`
}

// AgentUserTrafficDelta carries per-user traffic increments reported by an agent.
type AgentUserTrafficDelta struct {
	Email     string `json:"email"`
	UpBytes   int64  `json:"upBytes"`
	DownBytes int64  `json:"downBytes"`
}

// AgentInboundTrafficDelta carries per-inbound traffic increments reported by an agent.
type AgentInboundTrafficDelta struct {
	Tag       string `json:"tag"`
	UpBytes   int64  `json:"upBytes"`
	DownBytes int64  `json:"downBytes"`
}

// AgentTrafficBatch carries an idempotent traffic batch identified by BatchID.
type AgentTrafficBatch struct {
	BatchID  string                     `json:"batchId"`
	Users    []AgentUserTrafficDelta    `json:"users,omitempty"`
	Inbounds []AgentInboundTrafficDelta `json:"inbounds,omitempty"`
}

// AgentSyncRequest is the locked POST /api/agent/sync request contract.
type AgentSyncRequest struct {
	ConfigHash   string             `json:"configHash"`
	UsersHash    string             `json:"usersHash"`
	AgentVersion string             `json:"agentVersion"`
	XrayVersion  string             `json:"xrayVersion"`
	LastError    string             `json:"lastError"`
	HostMetrics  *AgentHostMetrics  `json:"hostMetrics,omitempty"`
	TrafficBatch *AgentTrafficBatch `json:"trafficBatch,omitempty"`
}

// AgentSyncUser represents an authorized active user credential for a node inbound.
type AgentSyncUser struct {
	InboundTag string `json:"inboundTag"`
	Protocol   string `json:"protocol"`
	Email      string `json:"email"`
	UUID       string `json:"uuid"`
	Flow       string `json:"flow,omitempty"`
}

// AgentSyncResponse is the locked POST /api/agent/sync response contract.
type AgentSyncResponse struct {
	OK              bool            `json:"ok"`
	DesiredState    string          `json:"desiredState"`
	SyncIntervalSec int             `json:"syncIntervalSec"`
	ConfigHash      string          `json:"configHash"`
	UsersHash       string          `json:"usersHash"`
	ConfigUnchanged bool            `json:"configUnchanged"`
	UsersUnchanged  bool            `json:"usersUnchanged"`
	ConfigJSON      string          `json:"configJson,omitempty"`
	Users           []AgentSyncUser `json:"users,omitempty"`
}
