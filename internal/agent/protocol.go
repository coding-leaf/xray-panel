package agent

// DesiredState constants matching Master's domain.Node DesiredState values.
const (
	DesiredStateActive        = "active"
	DesiredStateDraining      = "draining"
	DesiredStateCircuitBroken = "circuit_broken"
)

// SyncRequest is the locked HTTP JSON payload sent by cmd/agent to POST /api/agent/sync.
type SyncRequest struct {
	ConfigHash   string        `json:"configHash"`
	UsersHash    string        `json:"usersHash"`
	AgentVersion string        `json:"agentVersion"`
	XrayVersion  string        `json:"xrayVersion"`
	LastError    string        `json:"lastError"`
	HostMetrics  HostMetrics   `json:"hostMetrics"`
	TrafficBatch *TrafficBatch `json:"trafficBatch,omitempty"`
}

// HostMetrics carries host CPU, memory, and primary NIC byte deltas.
type HostMetrics struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemPercent   float64 `json:"memPercent"`
	NicUpDelta   int64   `json:"nicUpDelta"`
	NicDownDelta int64   `json:"nicDownDelta"`
}

// TrafficBatch represents an idempotent traffic delta batch spooled on disk until acked.
type TrafficBatch struct {
	BatchID  string                `json:"batchId"`
	Users    []UserTrafficDelta    `json:"users,omitempty"`
	Inbounds []InboundTrafficDelta `json:"inbounds,omitempty"`
}

// UserTrafficDelta holds per-user uplink/downlink byte increments.
type UserTrafficDelta struct {
	Email     string `json:"email"`
	UpBytes   int64  `json:"upBytes"`
	DownBytes int64  `json:"downBytes"`
}

// InboundTrafficDelta holds per-inbound uplink/downlink byte increments.
type InboundTrafficDelta struct {
	Tag       string `json:"tag"`
	UpBytes   int64  `json:"upBytes"`
	DownBytes int64  `json:"downBytes"`
}

// SyncResponse is the locked HTTP JSON response returned by Master for POST /api/agent/sync.
type SyncResponse struct {
	OK              bool         `json:"ok"`
	DesiredState    string       `json:"desiredState"`
	SyncIntervalSec int          `json:"syncIntervalSec"`
	ConfigHash      string       `json:"configHash"`
	UsersHash       string       `json:"usersHash"`
	ConfigUnchanged bool         `json:"configUnchanged"`
	UsersUnchanged  bool         `json:"usersUnchanged"`
	ConfigJSON      string       `json:"configJson,omitempty"`
	Users           []RemoteUser `json:"users,omitempty"`
	QuotaPolicy     *QuotaPolicy `json:"quotaPolicy,omitempty"`
}

// QuotaPolicy describes the node's monthly quota and autonomous circuit-breaker policy pushed by Master.
type QuotaPolicy struct {
	TrafficLimitBytes int64   `json:"trafficLimitBytes"`
	BreakThresholdPct int     `json:"breakThresholdPct"`
	CountMode         string  `json:"countMode"`
	Multiplier        float64 `json:"multiplier"`
	CycleUsedBytes    int64   `json:"cycleUsedBytes"`
	CycleMonth        int     `json:"cycleMonth"`
}

// RemoteUser describes an authorized user entry for differential gRPC updates.
type RemoteUser struct {
	InboundTag string `json:"inboundTag"`
	Protocol   string `json:"protocol"`
	Email      string `json:"email"`
	UUID       string `json:"uuid"`
	Flow       string `json:"flow,omitempty"`
}
