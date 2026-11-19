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
	client   *Client
	spool    *Spool
	cfgMgr   *ConfigManager
	userSync *UserSyncer
	traffic  TrafficCollector
	metrics  *MetricsCollector

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

	if pending == nil && r.traffic != nil {
		collected, colErr := r.traffic.CollectTrafficStats(ctx)
		if colErr != nil {
			slog.Warn("Failed to collect xray traffic stats", slog.String("error", colErr.Error()))
		} else if collected != nil {
			if saveErr := r.spool.Save(collected); saveErr != nil {
				slog.Error("Failed to persist traffic batch to spool", slog.String("error", saveErr.Error()))
			}
			pending = collected
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
		HostMetrics:  hostMetrics,
		TrafficBatch: pending,
	}
	r.mu.Unlock()

	resp, err := r.client.Sync(ctx, req)
	if err != nil {
		r.setLastError(err.Error())
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

func (r *Runner) setLastError(msg string) {
	r.mu.Lock()
	r.lastError = msg
	r.mu.Unlock()
}
