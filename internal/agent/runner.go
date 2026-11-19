package agent

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const defaultSyncInterval = 20 * time.Second

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

	mu            sync.Mutex
	agentVersion  string
	xrayVersion   string
	interval      time.Duration
	configHash    string
	usersHash     string
	lastError     string
	circuitBroken bool
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
		client:       client,
		spool:        spool,
		cfgMgr:       cfgMgr,
		userSync:     userSync,
		traffic:      traffic,
		metrics:      metrics,
		agentVersion: cfg.AgentVersion,
		interval:     interval,
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

	r.mu.Lock()
	if r.xrayVersion == "" {
		r.xrayVersion = r.cfgMgr.DetectXrayVersion(ctx)
	}
	req := &SyncRequest{
		ConfigHash:   r.configHash,
		UsersHash:    r.usersHash,
		AgentVersion: r.agentVersion,
		XrayVersion:  r.xrayVersion,
		LastError:    r.lastError,
		HostMetrics:  &hostMetrics,
		TrafficBatch: pending,
	}
	r.mu.Unlock()

	resp, err := r.client.Sync(ctx, req)
	if err != nil {
		r.setLastError(err.Error())
		r.handleOfflineQuota(ctx, hostMetrics, freshTraffic)
		return err
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

	reconcileErr := r.reconcileSyncResponse(ctx, resp)
	if resp.LogRequest != nil {
		r.handleLogRequest(ctx, resp.LogRequest)
	}
	return reconcileErr
}

func (r *Runner) reconcileSyncResponse(ctx context.Context, resp *SyncResponse) error {
	if resp.DesiredState == DesiredStateCircuitBroken {
		r.mu.Lock()
		alreadyBroken := r.circuitBroken
		r.mu.Unlock()

		if !alreadyBroken {
			if cbErr := r.cfgMgr.ApplyCircuitBreaker(ctx); cbErr != nil {
				r.setLastError(cbErr.Error())
				return cbErr
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
		return nil
	}

	if !resp.ConfigUnchanged && resp.ConfigJSON != "" {
		if applyErr := r.cfgMgr.Apply(ctx, resp.ConfigJSON); applyErr != nil {
			r.setLastError(applyErr.Error())
			return applyErr
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
		return nil
	}

	if !resp.UsersUnchanged {
		added, removed, recErr := r.userSync.Reconcile(ctx, resp.Users)
		if recErr != nil {
			r.setLastError(recErr.Error())
			return recErr
		}
		r.mu.Lock()
		r.usersHash = resp.UsersHash
		r.circuitBroken = false
		r.lastError = ""
		r.mu.Unlock()
		slog.Info("Reconciled xray users via gRPC", slog.Int("added", added), slog.Int("removed", removed))
		return nil
	}

	r.mu.Lock()
	r.circuitBroken = false
	r.lastError = ""
	r.mu.Unlock()
	return nil
}

func (r *Runner) handleLogRequest(ctx context.Context, logReq *LogRequest) {
	if logReq == nil {
		return
	}
	r.mu.Lock()
	collector := r.logCollector
	r.mu.Unlock()

	result := &LogResult{
		RequestID:       logReq.RequestID,
		Service:         logReq.Service,
		CollectedAtUnix: time.Now().Unix(),
	}

	if collector == nil {
		result.Error = "log collector not configured"
	} else {
		lines, err := collector.Collect(ctx, logReq.Service, logReq.Lines)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Lines = lines
		}
	}

	r.mu.Lock()
	ackReq := &SyncRequest{
		ConfigHash:   r.configHash,
		UsersHash:    r.usersHash,
		AgentVersion: r.agentVersion,
		XrayVersion:  r.xrayVersion,
		HostMetrics:  nil,
		TrafficBatch: nil,
		LogResult:    result,
	}
	r.mu.Unlock()

	if _, err := r.client.Sync(ctx, ackReq); err != nil {
		slog.Warn("Failed to send on-demand log result to master",
			slog.String("request_id", logReq.RequestID),
			slog.String("service", logReq.Service),
			slog.String("error", err.Error()),
		)
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
