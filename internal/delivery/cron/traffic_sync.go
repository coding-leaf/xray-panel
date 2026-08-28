package cron

import (
	"context"
	"log/slog"
	"time"

	"panel/internal/domain"
	"panel/internal/service"
)

type TrafficSyncJob struct {
	xrayManager domain.XrayManager
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
	alertSvc    *service.AlertService
	interval    time.Duration
}

func NewTrafficSyncJob(
	xrayManager domain.XrayManager,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	alertSvc *service.AlertService,
	interval time.Duration,
) *TrafficSyncJob {
	if interval < 5*time.Second {
		interval = 15 * time.Second
	}
	return &TrafficSyncJob{
		xrayManager: xrayManager,
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		alertSvc:    alertSvc,
		interval:    interval,
	}
}

func (j *TrafficSyncJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	alertTicker := time.NewTicker(5 * time.Minute)

	slog.Info("Traffic sync job started", slog.Duration("interval", j.interval))

	go func() {
		defer ticker.Stop()
		defer alertTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.syncOnce(ctx)
			case <-alertTicker.C:
				_ = j.alertSvc.CheckTrafficQuotas(ctx)
				_ = j.alertSvc.CheckSystemLoad(ctx)
			}
		}
	}()
}

func (j *TrafficSyncJob) syncOnce(ctx context.Context) {
	// 查询增量统计数据并重置计数
	stats, err := j.xrayManager.QueryTrafficStats(ctx, true)
	if err != nil {
		return
	}

	for _, s := range stats {
		if s.Value <= 0 {
			continue
		}

		var up, down int64
		if s.IsUplink {
			up = s.Value
		} else {
			down = s.Value
		}

		if s.Type == domain.TrafficStatTypeUser {
			_ = j.userRepo.AddTraffic(ctx, s.Tag, up, down)

			// 检查是否超出限额，超出则立即从 Xray 剔除
			user, err := j.userRepo.GetByEmail(ctx, s.Tag)
			if err == nil && user.IsTrafficExceeded() {
				_ = j.xrayManager.RemoveUser(ctx, user.InboundTag, user.Email)
			}
		} else if s.Type == domain.TrafficStatTypeInbound {
			_ = j.inboundRepo.AddTraffic(ctx, s.Tag, up, down)
		}
	}
}
