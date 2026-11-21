package agent

// DesiredState constants matching Master's domain.Node DesiredState values.
const (
	DesiredStateActive        = "active"
	DesiredStateDraining      = "draining"
	DesiredStateCircuitBroken = "circuit_broken"

	CommandTypeLogTail       = "log_tail"
	CommandTypeGeoDataUpdate = "geodata_update"

	CommandStatusSucceeded = "succeeded"
	CommandStatusFailed    = "failed"
)

// SyncRequest is the locked HTTP JSON payload sent by cmd/agent to POST /api/agent/sync.
type SyncRequest struct {
	ConfigHash     string          `json:"configHash"`
	UsersHash      string          `json:"usersHash"`
	DiskConfigHash string          `json:"diskConfigHash,omitempty"`
	AgentVersion   string          `json:"agentVersion"`
	XrayVersion    string          `json:"xrayVersion"`
	LastError      string          `json:"lastError"`
	HostMetrics    *HostMetrics    `json:"hostMetrics,omitempty"`
	TrafficBatch   *TrafficBatch   `json:"trafficBatch,omitempty"`
	CommandResults []CommandResult `json:"commandResults,omitempty"`
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
	Commands        []Command    `json:"commands,omitempty"`
}

// Command represents an asynchronous instruction dispatched by Master during sync.
type Command struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Service string `json:"service,omitempty"`
	Lines   int    `json:"lines,omitempty"`
}

// CommandResult carries the outcome of an asynchronous command reported back to Master.
type CommandResult struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Error           string   `json:"error,omitempty"`
	Service         string   `json:"service,omitempty"`
	Lines           []string `json:"lines,omitempty"`
	CollectedAtUnix int64    `json:"collectedAtUnix,omitempty"`
	GeoIPSize       int64    `json:"geoipSize,omitempty"`
	GeoSiteSize     int64    `json:"geositeSize,omitempty"`
	DurationMs      int64    `json:"durationMs,omitempty"`
	UpdatedAtUnix   int64    `json:"updatedAtUnix,omitempty"`
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
	Method     string `json:"method,omitempty"`
}
