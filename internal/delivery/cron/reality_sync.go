package cron

import (
	"context"
	"log/slog"
	"time"

	"panel/internal/app"
	"panel/internal/domain"
)

var _ app.Service = (*RealitySyncJob)(nil)

// RealityInspector defines the inspection contract for Reality destinations.
type RealityInspector interface {
	CheckAll(ctx context.Context) (domain.RealitySummaryStatus, error)
}

// RealitySyncJob periodically triggers background Reality domain compliance checks.
type RealitySyncJob struct {
	inspector RealityInspector
	interval  time.Duration
}

// NewRealitySyncJob creates a new background RealitySyncJob.
// Defaults to 12 hours if interval is non-positive.
func NewRealitySyncJob(inspector RealityInspector, interval time.Duration) *RealitySyncJob {
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	return &RealitySyncJob{
		inspector: inspector,
		interval:  interval,
	}
}

// Start begins the inspection lifecycle: performs an immediate async probe,
// then enters a ticker loop until ctx is canceled.
func (j *RealitySyncJob) Start(ctx context.Context) error {
	slog.Info("Reality sync job started", slog.Duration("interval", j.interval))

	// 1. 启动时立即异步执行一次探测，避免面板启动初期缓存空白
	if j.inspector != nil {
		go func() {
			initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			slog.Info("Reality sync job: running initial inspection")
			if _, err := j.inspector.CheckAll(initCtx); err != nil && ctx.Err() == nil {
				slog.Warn("Reality sync job initial inspection completed with error", slog.String("error", err.Error()))
			}
		}()
	}

	// 2. 周期调度循环
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Reality sync job received stop signal, shutting down cleanly")
			return nil
		case <-ticker.C:
			slog.Info("Reality sync job: executing scheduled inspection")
			func() {
				checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				if _, err := j.inspector.CheckAll(checkCtx); err != nil && ctx.Err() == nil {
					slog.Warn("Reality sync job scheduled inspection failed", slog.String("error", err.Error()))
				}
			}()
		}
	}
}

// Stop gracefully stops any external resources if needed.
func (j *RealitySyncJob) Stop(ctx context.Context) error {
	return nil
}
