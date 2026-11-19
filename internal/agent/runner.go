package agent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	defaultSyncInterval      = 60 * time.Second
	maxPendingCommandResults = 32
)

// RunnerConfig configures the Agent sync loop.
type RunnerConfig struct {
	AgentVersion string
	Interval     time.Duration
}

// Runner orchestrates periodic Master heartbeats, traffic spooling, config reloads,
// and differential gRPC user updates.
type Runner struct {
	client       *Client
	spool        *Spool
	cfgMgr       *ConfigManager
	userSync     *UserSyncer
	traffic      TrafficCollector
	metrics      *MetricsCollector
	quotaStore   *QuotaStore
	logCollector LogCollector
	geoUpdater   GeoDataUpdater

	mu            sync.Mutex
	agentVersion  string
	xrayVersion   string
	interval      time.Duration
	configHash    string
	usersHash     string
	lastError     string
	circuitBroken bool

	cmdMu          sync.Mutex
	inFlightByID   map[string]bool
	inFlightByType map[string]bool
	pendingResults []CommandResult
	cmdWG          sync.WaitGroup
}

// NewRunner constructs a Runner with all required collaborators.
func NewRunner(
	cfg RunnerConfig,
	client *Client,
	spool *Spool,
	cfgMgr *ConfigManager,
	userSync *UserSyncer,
	traffic TrafficCollector,
	metrics *MetricsCollector,
) (*Runner, error) {
	if client == nil {
		return nil, errors.New("agent client is required")
	}
	if spool == nil {
		return nil, errors.New("agent spool is required")
	}
	if cfgMgr == nil {
		return nil, errors.New("config manager is required")
	}
	if userSync == nil {
		return nil, errors.New("user syncer is required")
	}
	if metrics == nil {
		metrics = NewMetricsCollector()
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultSyncInterval
	}
	return &Runner{
		client:         client,
		spool:          spool,
		cfgMgr:         cfgMgr,
		userSync:       userSync,
		traffic:        traffic,
		metrics:        metrics,
		agentVersion:   cfg.AgentVersion,
		interval:       interval,
		inFlightByID:   make(map[string]bool),
		inFlightByType: make(map[string]bool),
	}, nil
}

// WithQuotaStore attaches an optional local QuotaStore for autonomous offline circuit breaking.
func (r *Runner) WithQuotaStore(store *QuotaStore) *Runner {
	r.mu.Lock()
	r.quotaStore = store
	r.mu.Unlock()
	return r
}

// WithLogCollector attaches an optional LogCollector for on-demand remote log retrieval.
func (r *Runner) WithLogCollector(c LogCollector) *Runner {
	r.mu.Lock()
	r.logCollector = c
	r.mu.Unlock()
	return r
}

// WithGeoDataUpdater attaches an optional GeoDataUpdater for on-demand remote GeoData updates.
func (r *Runner) WithGeoDataUpdater(u GeoDataUpdater) *Runner {
	r.mu.Lock()
	r.geoUpdater = u
	r.mu.Unlock()
	return r
}

// WaitCommands blocks until all background asynchronous command goroutines have exited.
func (r *Runner) WaitCommands() {
	r.cmdWG.Wait()
}

// Run executes an immediate sync and then polls Master on the configured interval until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.mu.Lock()
	r.xrayVersion = r.cfgMgr.DetectXrayVersion(ctx)
	curInterval := r.interval
	r.mu.Unlock()

	if err := r.SyncOnce(ctx); err != nil && ctx.Err() == nil {
		slog.Warn("Initial agent sync failed", slog.String("error", err.Error()))
	}

	ticker := time.NewTicker(curInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.cmdWG.Wait()
			return nil
		case <-ticker.C:
			if err := r.SyncOnce(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("Agent sync cycle failed", slog.String("error", err.Error()))
			}
			r.mu.Lock()
			nextInterval := r.interval
			r.mu.Unlock()
			if nextInterval > 0 && nextInterval != curInterval {
				curInterval = nextInterval
				ticker.Reset(curInterval)
			}
		}
	}
}

// SyncOnce executes a single heartbeat + traffic upload + state reconciliation cycle.
func (r *Runner) SyncOnce(ctx context.Context) error {
	pending, loadErr := r.spool.Load()
	if loadErr != nil {
		slog.Warn("Failed to load spooled traffic batch", slog.String("error", loadErr.Error()))
	}

	var freshTraffic *TrafficBatch
	if pending == nil && r.traffic != nil {
		collected, colErr := r.traffic.CollectTrafficStats(ctx)
		if colErr != nil {
			slog.Warn("Failed to collect xray traffic stats", slog.String("error", colErr.Error()))
		} else if collected != nil {
			if saveErr := r.spool.Save(collected); saveErr != nil {
				slog.Error("Failed to persist traffic batch to spool", slog.String("error", saveErr.Error()))
			}
			pending = collected
			freshTraffic = collected
		}
	}

	hostMetrics := r.metrics.Collect(ctx)
	sentResults := r.snapshotPendingResults()

	r.mu.Lock()
	if r.xrayVersion == "" {
		r.xrayVersion = r.cfgMgr.DetectXrayVersion(ctx)
	}
	req := &SyncRequest{
		ConfigHash:     r.configHash,
		UsersHash:      r.usersHash,
		AgentVersion:   r.agentVersion,
		XrayVersion:    r.xrayVersion,
		LastError:      r.lastError,
		HostMetrics:    &hostMetrics,
		TrafficBatch:   pending,
		CommandResults: sentResults,
	}
	r.mu.Unlock()

	resp, err := r.client.Sync(ctx, req)
	if err != nil {
		r.setLastError(err.Error())
		r.handleOfflineQuota(ctx, hostMetrics, freshTraffic)
		return err
	}

	if len(sentResults) > 0 {
		r.ackPendingResults(sentResults)
	}

	if pending != nil {
		if ackErr := r.spool.Ack(pending.BatchID); ackErr != nil {
			slog.Warn("Failed to ack spooled traffic batch", slog.String("batch_id", pending.BatchID), slog.String("error", ackErr.Error()))
		}
	}

	if resp.SyncIntervalSec > 0 {
		r.mu.Lock()
		r.interval = time.Duration(resp.SyncIntervalSec) * time.Second
		r.mu.Unlock()
	}

	r.syncQuotaStoreOnline(resp)

	applied, reconcileErr := r.reconcileSyncResponse(ctx, resp)
	if applied {
		r.sendConfigAck(ctx)
	}
	if len(resp.Commands) > 0 {
		r.spawnCommands(ctx, resp.Commands)
	}
	return reconcileErr
}

func (r *Runner) reconcileSyncResponse(ctx context.Context, resp *SyncResponse) (bool, error) {
	if resp.DesiredState == DesiredStateCircuitBroken {
		r.mu.Lock()
		alreadyBroken := r.circuitBroken
		r.mu.Unlock()

		if !alreadyBroken {
			if cbErr := r.cfgMgr.ApplyCircuitBreaker(ctx); cbErr != nil {
				r.setLastError(cbErr.Error())
				return false, cbErr
			}
			r.userSync.ResetBaseline(nil)
			r.mu.Lock()
			r.circuitBroken = true
			r.configHash = ""
			r.usersHash = ""
			r.lastError = ""
			r.mu.Unlock()
			slog.Warn("Applied circuit-breaker silence config")
		} else {
			r.setLastError("")
		}
		return false, nil
	}

	if !resp.ConfigUnchanged && resp.ConfigJSON != "" {
		if applyErr := r.cfgMgr.Apply(ctx, resp.ConfigJSON); applyErr != nil {
			r.setLastError(applyErr.Error())
			return false, applyErr
		}
		r.mu.Lock()
		r.configHash = resp.ConfigHash
		r.circuitBroken = false
		r.lastError = ""
		if !resp.UsersUnchanged {
			r.userSync.ResetBaseline(resp.Users)
			r.usersHash = resp.UsersHash
		}
		r.mu.Unlock()
		slog.Info("Applied updated xray config", slog.String("config_hash", resp.ConfigHash))
		return true, nil
	}

	if !resp.UsersUnchanged {
		added, removed, recErr := r.userSync.Reconcile(ctx, resp.Users)
		if recErr != nil {
			r.setLastError(recErr.Error())
			return false, recErr
		}
		r.mu.Lock()
		r.usersHash = resp.UsersHash
		r.circuitBroken = false
		r.lastError = ""
		r.mu.Unlock()
		slog.Info("Reconciled xray users via gRPC", slog.Int("added", added), slog.Int("removed", removed))
		return true, nil
	}

	r.mu.Lock()
	r.circuitBroken = false
	r.lastError = ""
	r.mu.Unlock()
	return false, nil
}

func commandGuardKey(cmd Command) string {
	switch cmd.Type {
	case CommandTypeGeoDataUpdate:
		return CommandTypeGeoDataUpdate
	case CommandTypeLogTail:
		return CommandTypeLogTail + ":" + strings.TrimSpace(cmd.Service)
	default:
		return ""
	}
}

func (r *Runner) spawnCommands(ctx context.Context, cmds []Command) {
	for _, cmd := range cmds {
		r.spawnCommand(ctx, cmd)
	}
}

func (r *Runner) spawnCommand(ctx context.Context, cmd Command) {
	cmdID := strings.TrimSpace(cmd.ID)
	if cmdID == "" {
		return
	}
	cmd.ID = cmdID
	guardKey := commandGuardKey(cmd)

	r.cmdMu.Lock()
	if r.inFlightByID == nil {
		r.inFlightByID = make(map[string]bool)
	}
	if r.inFlightByType == nil {
		r.inFlightByType = make(map[string]bool)
	}
	if r.inFlightByID[cmdID] {
		r.cmdMu.Unlock()
		return
	}
	if guardKey != "" && r.inFlightByType[guardKey] {
		r.inFlightByID[cmdID] = true
		r.cmdWG.Add(1)
		r.cmdMu.Unlock()
		go func() {
			defer func() {
				r.cmdMu.Lock()
				delete(r.inFlightByID, cmdID)
				r.cmdMu.Unlock()
				r.cmdWG.Done()
			}()
			if ctx.Err() != nil {
				return
			}
			r.deliverCommandResult(ctx, CommandResult{
				ID:     cmdID,
				Type:   cmd.Type,
				Status: CommandStatusFailed,
				Error:  "command already in progress",
			})
		}()
		return
	}

	r.inFlightByID[cmdID] = true
	if guardKey != "" {
		r.inFlightByType[guardKey] = true
	}
	r.cmdWG.Add(1)
	r.cmdMu.Unlock()

	go func() {
		defer func() {
			r.cmdMu.Lock()
			delete(r.inFlightByID, cmdID)
			if guardKey != "" {
				delete(r.inFlightByType, guardKey)
			}
			r.cmdMu.Unlock()
			r.cmdWG.Done()
		}()

		res := r.executeCommand(ctx, cmd)
		if ctx.Err() != nil {
			return
		}
		r.deliverCommandResult(ctx, res)
	}()
}

func (r *Runner) executeCommand(ctx context.Context, cmd Command) CommandResult {
	switch cmd.Type {
	case CommandTypeLogTail:
		r.mu.Lock()
		collector := r.logCollector
		r.mu.Unlock()

		res := CommandResult{
			ID:              cmd.ID,
			Type:            cmd.Type,
			Service:         cmd.Service,
			CollectedAtUnix: time.Now().Unix(),
		}
		if collector == nil {
			res.Status = CommandStatusFailed
			res.Error = "log collector not configured"
			return res
		}
		lines, err := collector.Collect(ctx, cmd.Service, cmd.Lines)
		if err != nil {
			res.Status = CommandStatusFailed
			res.Error = err.Error()
			return res
		}
		res.Status = CommandStatusSucceeded
		res.Lines = lines
		return res

	case CommandTypeGeoDataUpdate:
		r.mu.Lock()
		updater := r.geoUpdater
		r.mu.Unlock()

		start := time.Now()
		res := CommandResult{
			ID:   cmd.ID,
			Type: cmd.Type,
		}
		if updater == nil {
			res.Status = CommandStatusFailed
			res.Error = "geodata updater not configured"
		} else {
			geoipSize, geositeSize, err := updater.Update(ctx)
			res.GeoIPSize = geoipSize
			res.GeoSiteSize = geositeSize
			if err != nil {
				res.Status = CommandStatusFailed
				res.Error = err.Error()
			} else {
				res.Status = CommandStatusSucceeded
			}
		}
		res.DurationMs = time.Since(start).Milliseconds()
		res.UpdatedAtUnix = time.Now().Unix()
		return res

	default:
		return CommandResult{
			ID:     cmd.ID,
			Type:   cmd.Type,
			Status: CommandStatusFailed,
			Error:  "unsupported command type: " + cmd.Type,
		}
	}
}

func (r *Runner) deliverCommandResult(ctx context.Context, res CommandResult) {
	reportCtx, cancel := context.WithTimeout(ctx, defaultHTTPTimeout)
	defer cancel()

	if err := r.client.ReportCommandResult(reportCtx, &res); err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("Failed to report command result immediately, queuing for next sync",
			slog.String("command_id", res.ID),
			slog.String("type", res.Type),
			slog.String("error", err.Error()),
		)
		r.enqueuePendingResult(res)
	}
}

func (r *Runner) enqueuePendingResult(res CommandResult) {
	r.cmdMu.Lock()
	defer r.cmdMu.Unlock()
	for i := range r.pendingResults {
		if r.pendingResults[i].ID == res.ID {
			r.pendingResults[i] = res
			return
		}
	}
	if len(r.pendingResults) >= maxPendingCommandResults {
		copy(r.pendingResults, r.pendingResults[1:])
		r.pendingResults[len(r.pendingResults)-1] = res
		return
	}
	r.pendingResults = append(r.pendingResults, res)
}

func (r *Runner) snapshotPendingResults() []CommandResult {
	r.cmdMu.Lock()
	defer r.cmdMu.Unlock()
	if len(r.pendingResults) == 0 {
		return nil
	}
	out := make([]CommandResult, len(r.pendingResults))
	copy(out, r.pendingResults)
	return out
}

func (r *Runner) ackPendingResults(acked []CommandResult) {
	if len(acked) == 0 {
		return
	}
	ackedIDs := make(map[string]struct{}, len(acked))
	for _, res := range acked {
		ackedIDs[res.ID] = struct{}{}
	}
	r.cmdMu.Lock()
	defer r.cmdMu.Unlock()
	if len(r.pendingResults) == 0 {
		return
	}
	remaining := r.pendingResults[:0]
	for _, res := range r.pendingResults {
		if _, ok := ackedIDs[res.ID]; !ok {
			remaining = append(remaining, res)
		}
	}
	if len(remaining) == 0 {
		r.pendingResults = nil
	} else {
		r.pendingResults = remaining
	}
}

func (r *Runner) sendConfigAck(ctx context.Context) {
	r.mu.Lock()
	ackReq := &SyncRequest{
		ConfigHash:   r.configHash,
		UsersHash:    r.usersHash,
		AgentVersion: r.agentVersion,
		XrayVersion:  r.xrayVersion,
		LastError:    r.lastError,
		HostMetrics:  nil,
		TrafficBatch: nil,
	}
	r.mu.Unlock()

	ackResp, err := r.client.Sync(ctx, ackReq)
	if err != nil {
		slog.Warn("Failed to send lightweight ack to master", slog.String("error", err.Error()))
		return
	}
	if ackResp != nil && len(ackResp.Commands) > 0 {
		r.spawnCommands(ctx, ackResp.Commands)
	}
}

func (r *Runner) setLastError(msg string) {
	r.mu.Lock()
	r.lastError = msg
	r.mu.Unlock()
}

func (r *Runner) syncQuotaStoreOnline(resp *SyncResponse) {
	r.mu.Lock()
	qs := r.quotaStore
	r.mu.Unlock()
	if qs == nil || resp == nil {
		return
	}
	masterBroken := resp.DesiredState == DesiredStateCircuitBroken
	if resp.QuotaPolicy != nil {
		if err := qs.UpdatePolicy(*resp.QuotaPolicy, masterBroken); err != nil {
			slog.Warn("Failed to persist quota policy", slog.String("error", err.Error()))
		}
		return
	}
	if !masterBroken {
		if err := qs.ClearBroken(); err != nil {
			slog.Warn("Failed to clear local circuit-broken state", slog.String("error", err.Error()))
		}
	}
}

func (r *Runner) handleOfflineQuota(ctx context.Context, hostMetrics HostMetrics, freshTraffic *TrafficBatch) {
	r.mu.Lock()
	qs := r.quotaStore
	alreadyBroken := r.circuitBroken
	r.mu.Unlock()

	if qs == nil {
		return
	}

	up, down := hostMetrics.NicUpDelta, hostMetrics.NicDownDelta
	if up <= 0 && down <= 0 && freshTraffic != nil {
		up, down = sumTrafficBatch(freshTraffic)
	}

	_, shouldBreak, err := qs.RecordOfflineUsage(up, down)
	if err != nil {
		slog.Warn("Failed to record offline quota usage", slog.String("error", err.Error()))
		shouldBreak = qs.ShouldBreak()
	}

	if shouldBreak && !alreadyBroken {
		if cbErr := r.cfgMgr.ApplyCircuitBreaker(ctx); cbErr != nil {
			slog.Error("Failed to apply autonomous circuit breaker", slog.String("error", cbErr.Error()))
			r.setLastError(cbErr.Error())
			return
		}
		r.userSync.ResetBaseline(nil)
		r.mu.Lock()
		r.circuitBroken = true
		r.configHash = ""
		r.usersHash = ""
		r.mu.Unlock()
		if markErr := qs.MarkBroken(); markErr != nil {
			slog.Warn("Failed to persist local circuit-broken state", slog.String("error", markErr.Error()))
		}
		slog.Warn("Applied autonomous offline circuit-breaker silence config")
	}
}

func sumTrafficBatch(batch *TrafficBatch) (up, down int64) {
	if batch == nil {
		return 0, 0
	}
	if len(batch.Inbounds) > 0 {
		for _, ib := range batch.Inbounds {
			up += ib.UpBytes
			down += ib.DownBytes
		}
		return up, down
	}
	for _, u := range batch.Users {
		up += u.UpBytes
		down += u.DownBytes
	}
	return up, down
}
