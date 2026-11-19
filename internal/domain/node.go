package domain

import (
	"errors"
	"math"
	"time"
)

const (
	NodeStateActive        = "active"
	NodeStateDraining      = "draining"
	NodeStateCircuitBroken = "circuit_broken"

	TrafficCountModeBoth     = "both"
	TrafficCountModeOutbound = "outbound"
	TrafficCountModeMax      = "max"

	DefaultWarnThresholdPct  = 80
	DefaultDrainThresholdPct = 90
	DefaultBreakThresholdPct = 95

	AutoReasonDrainQuota = "auto_drain_quota"
	AutoReasonBreakQuota = "auto_break_quota"
	AutoReasonManual     = "manual"
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

// IsValidTrafficCountMode checks whether mode is a supported node quota accounting mode.
func IsValidTrafficCountMode(mode string) bool {
	switch mode {
	case TrafficCountModeBoth, TrafficCountModeOutbound, TrafficCountModeMax:
		return true
	default:
		return false
	}
}

// IsAutoQuotaStateReason reports whether reason indicates an automatic quota state transition.
func IsAutoQuotaStateReason(reason string) bool {
	return reason == AutoReasonDrainQuota || reason == AutoReasonBreakQuota
}

// CalcTrafficByMode computes billable bytes from upload/download counters using count mode and multiplier.
func CalcTrafficByMode(up, down int64, mode string, multiplier float64) int64 {
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	var base int64
	switch mode {
	case TrafficCountModeOutbound:
		base = up
	case TrafficCountModeMax:
		if up >= down {
			base = up
		} else {
			base = down
		}
	default:
		base = up + down
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return int64(math.Round(float64(base) * multiplier))
}

// ReachedQuotaThreshold reports whether used bytes reached pct% of limit bytes without int64 overflow.
func ReachedQuotaThreshold(used, limit int64, pct int) bool {
	if limit <= 0 || pct <= 0 || used <= 0 {
		return false
	}
	if used <= math.MaxInt64/100 && limit <= math.MaxInt64/int64(pct) {
		return used*100 >= limit*int64(pct)
	}
	return float64(used)*100 >= float64(limit)*float64(pct)
}

// Node represents a managed Xray execution node (NodeID == 0 denotes the local Master node).
type Node struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name               string     `gorm:"type:varchar(128);not null" json:"name"`
	Host               string     `gorm:"type:varchar(255);not null" json:"host"`
	Region             string     `gorm:"type:varchar(64);default:''" json:"region"`
	TokenHash          string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
	TokenPrefix        string     `gorm:"type:varchar(16);default:''" json:"tokenPrefix"`
	Enabled            bool       `gorm:"default:true" json:"enabled"`
	DesiredState       string     `gorm:"type:varchar(32);default:'active'" json:"desiredState"`
	AutoStateReason    string     `gorm:"type:varchar(64);default:''" json:"autoStateReason"`
	LastSeenAt         *time.Time `json:"lastSeenAt"`
	AgentVersion       string     `gorm:"type:varchar(64);default:''" json:"agentVersion"`
	XrayVersion        string     `gorm:"type:varchar(64);default:''" json:"xrayVersion"`
	LastConfigHash     string     `gorm:"type:varchar(64);default:''" json:"lastConfigHash"`
	LastUsersHash      string     `gorm:"type:varchar(64);default:''" json:"lastUsersHash"`
	LastSyncError      string     `gorm:"type:text;default:''" json:"lastSyncError"`
	CPUPercent         float64    `gorm:"default:0" json:"cpuPercent"`
	MemPercent         float64    `gorm:"default:0" json:"memPercent"`
	NicUpBytes         int64      `gorm:"default:0" json:"nicUpBytes"`
	NicDownBytes       int64      `gorm:"default:0" json:"nicDownBytes"`
	XrayUpBytes        int64      `gorm:"default:0" json:"xrayUpBytes"`
	XrayDownBytes      int64      `gorm:"default:0" json:"xrayDownBytes"`
	TrafficLimitBytes  int64      `gorm:"default:0" json:"trafficLimitBytes"`
	BillingResetDay    int        `gorm:"default:0" json:"billingResetDay"`
	LastResetMonth     int        `gorm:"default:0" json:"lastResetMonth"`
	TrafficCountMode   string     `gorm:"type:varchar(16);default:'both'" json:"trafficCountMode"`
	TrafficMultiplier  float64    `gorm:"default:1.0" json:"trafficMultiplier"`
	WarnThresholdPct   int        `gorm:"default:80" json:"warnThresholdPct"`
	DrainThresholdPct  int        `gorm:"default:90" json:"drainThresholdPct"`
	BreakThresholdPct  int        `gorm:"default:95" json:"breakThresholdPct"`
	CycleXrayUpBytes   int64      `gorm:"default:0" json:"cycleXrayUpBytes"`
	CycleXrayDownBytes int64      `gorm:"default:0" json:"cycleXrayDownBytes"`
	CycleNicUpBytes    int64      `gorm:"default:0" json:"cycleNicUpBytes"`
	CycleNicDownBytes  int64      `gorm:"default:0" json:"cycleNicDownBytes"`
	Remark             string     `gorm:"type:varchar(255);default:''" json:"remark"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`

	Online         bool  `gorm:"-" json:"online"`         // 动态判定: LastSeenAt 距今 <= 90s
	CycleUsedBytes int64 `gorm:"-" json:"cycleUsedBytes"` // 动态计算当前周期已用计费流量
}

// EffectiveTrafficCountMode returns the normalized traffic count mode (defaults to "both").
func (n *Node) EffectiveTrafficCountMode() string {
	if n != nil && IsValidTrafficCountMode(n.TrafficCountMode) {
		return n.TrafficCountMode
	}
	return TrafficCountModeBoth
}

// EffectiveMultiplier returns the normalized traffic multiplier (defaults to 1.0).
func (n *Node) EffectiveMultiplier() float64 {
	if n != nil && n.TrafficMultiplier > 0 && !math.IsNaN(n.TrafficMultiplier) && !math.IsInf(n.TrafficMultiplier, 0) {
		return n.TrafficMultiplier
	}
	return 1.0
}

// EffectiveThresholds returns the normalized warn/drain/break percentage thresholds.
func (n *Node) EffectiveThresholds() (warn, drain, brk int) {
	warn = DefaultWarnThresholdPct
	drain = DefaultDrainThresholdPct
	brk = DefaultBreakThresholdPct
	if n == nil {
		return warn, drain, brk
	}
	if n.WarnThresholdPct > 0 {
		warn = n.WarnThresholdPct
	}
	if n.DrainThresholdPct > 0 {
		drain = n.DrainThresholdPct
	}
	if n.BreakThresholdPct > 0 {
		brk = n.BreakThresholdPct
	}
	return warn, drain, brk
}

// CalcCycleUsedBytes computes current cycle billable bytes, preferring NIC counters and falling back to Xray counters.
func (n *Node) CalcCycleUsedBytes() int64 {
	if n == nil {
		return 0
	}
	up, down := n.CycleXrayUpBytes, n.CycleXrayDownBytes
	if n.CycleNicUpBytes > 0 || n.CycleNicDownBytes > 0 {
		up, down = n.CycleNicUpBytes, n.CycleNicDownBytes
	}
	return CalcTrafficByMode(up, down, n.EffectiveTrafficCountMode(), n.EffectiveMultiplier())
}

// RefreshOnline updates transient Online and CycleUsedBytes fields relative to now.
func (n *Node) RefreshOnline(now time.Time) {
	if n == nil {
		return
	}
	n.CycleUsedBytes = n.CalcCycleUsedBytes()
	if n.LastSeenAt == nil {
		n.Online = false
		return
	}
	n.Online = now.Sub(*n.LastSeenAt) <= 90*time.Second
}

// RefreshComputed populates all transient computed fields on Node.
func (n *Node) RefreshComputed(now time.Time) {
	n.RefreshOnline(now)
}

// NodeTrafficDaily records daily aggregated Xray and NIC traffic per node for rate/depletion forecasting.
type NodeTrafficDaily struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NodeID        uint      `gorm:"uniqueIndex:idx_node_daily;not null" json:"nodeId"`
	Date          string    `gorm:"type:varchar(10);uniqueIndex:idx_node_daily;not null" json:"date"` // YYYY-MM-DD
	XrayUpBytes   int64     `gorm:"default:0" json:"xrayUpBytes"`
	XrayDownBytes int64     `gorm:"default:0" json:"xrayDownBytes"`
	NicUpBytes    int64     `gorm:"default:0" json:"nicUpBytes"`
	NicDownBytes  int64     `gorm:"default:0" json:"nicDownBytes"`
	UpdatedAt     time.Time `json:"updatedAt"`
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
	Name              string   `json:"name" binding:"required"`
	Host              string   `json:"host" binding:"required"`
	Region            string   `json:"region"`
	Enabled           *bool    `json:"enabled"`
	DesiredState      string   `json:"desiredState"`
	Remark            string   `json:"remark"`
	TrafficLimitBytes *int64   `json:"trafficLimitBytes"`
	BillingResetDay   *int     `json:"billingResetDay"`
	TrafficCountMode  string   `json:"trafficCountMode"`
	TrafficMultiplier *float64 `json:"trafficMultiplier"`
	WarnThresholdPct  *int     `json:"warnThresholdPct"`
	DrainThresholdPct *int     `json:"drainThresholdPct"`
	BreakThresholdPct *int     `json:"breakThresholdPct"`
}

// CreateNodeResponse returns the created node along with its one-time plaintext token.
type CreateNodeResponse struct {
	Node
	Token string `json:"token"`
}

// UpdateNodeDTO is the admin payload for updating node metadata and quota policy.
type UpdateNodeDTO struct {
	Name              *string  `json:"name"`
	Host              *string  `json:"host"`
	Region            *string  `json:"region"`
	Enabled           *bool    `json:"enabled"`
	DesiredState      *string  `json:"desiredState"`
	Remark            *string  `json:"remark"`
	TrafficLimitBytes *int64   `json:"trafficLimitBytes"`
	BillingResetDay   *int     `json:"billingResetDay"`
	TrafficCountMode  *string  `json:"trafficCountMode"`
	TrafficMultiplier *float64 `json:"trafficMultiplier"`
	WarnThresholdPct  *int     `json:"warnThresholdPct"`
	DrainThresholdPct *int     `json:"drainThresholdPct"`
	BreakThresholdPct *int     `json:"breakThresholdPct"`
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

// AgentQuotaPolicy delivers the node's quota policy and current cycle baseline to the agent.
type AgentQuotaPolicy struct {
	TrafficLimitBytes int64   `json:"trafficLimitBytes"`
	BreakThresholdPct int     `json:"breakThresholdPct"`
	CountMode         string  `json:"countMode"`
	Multiplier        float64 `json:"multiplier"`
	CycleUsedBytes    int64   `json:"cycleUsedBytes"`
	CycleMonth        int     `json:"cycleMonth"`
}

// AgentSyncResponse is the locked POST /api/agent/sync response contract.
type AgentSyncResponse struct {
	OK              bool              `json:"ok"`
	DesiredState    string            `json:"desiredState"`
	SyncIntervalSec int               `json:"syncIntervalSec"`
	ConfigHash      string            `json:"configHash"`
	UsersHash       string            `json:"usersHash"`
	ConfigUnchanged bool              `json:"configUnchanged"`
	UsersUnchanged  bool              `json:"usersUnchanged"`
	ConfigJSON      string            `json:"configJson,omitempty"`
	Users           []AgentSyncUser   `json:"users,omitempty"`
	QuotaPolicy     *AgentQuotaPolicy `json:"quotaPolicy,omitempty"`
}
