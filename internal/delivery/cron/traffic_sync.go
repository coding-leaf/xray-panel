package cron

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"panel/internal/app"
	"panel/internal/domain"
	"panel/internal/service"
)

var _ app.Service = (*TrafficSyncJob)(nil)

type TrafficSyncJob struct {
	xrayManager    domain.XrayManager
	userRepo       domain.UserRepository
	inboundRepo    domain.InboundRepository
	trafficLogRepo domain.TrafficLogRepository
	alertSvc       *service.AlertService
	userSvc        *service.UserService
	interval       time.Duration
}

func NewTrafficSyncJob(
	xrayManager domain.XrayManager,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	trafficLogRepo domain.TrafficLogRepository,
	alertSvc *service.AlertService,
	userSvc *service.UserService,
	interval time.Duration,
) *TrafficSyncJob {
	if interval < 3*time.Second {
		interval = 5 * time.Second
	}
	return &TrafficSyncJob{
		xrayManager:    xrayManager,
		userRepo:       userRepo,
		inboundRepo:    inboundRepo,
		trafficLogRepo: trafficLogRepo,
		alertSvc:       alertSvc,
		userSvc:        userSvc,
		interval:       interval,
	}
}

func (j *TrafficSyncJob) Start(ctx context.Context) error {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	alertTicker := time.NewTicker(5 * time.Minute)

	slog.Info("Traffic sync job started", slog.Duration("interval", j.interval))

	var wg sync.WaitGroup

	// 1. 独立外部告警与周期维护协程：执行 Telegram 外部网络通知与月度流量重置
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer alertTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-alertTicker.C:
				if j.alertSvc != nil {
					_ = j.alertSvc.CheckTrafficQuotas(ctx)
					_ = j.alertSvc.CheckSystemLoad(ctx)
					_ = j.alertSvc.CheckCertificates(ctx)
				}
				if j.userSvc != nil {
					_ = j.userSvc.CheckAndResetMonthlyTraffic(ctx)
				}
			}
		}
	}()

	// 2. 核心流量监控采集循环：基于 time.Ticker 循环采集，收到 ctx.Done() 后执行最后一次数据刷盘
	for {
		select {
		case <-ctx.Done():
			slog.Info("Traffic sync job received stop signal, executing final data flush...")
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			j.syncOnce(flushCtx)
			cancel()

			wg.Wait()
			slog.Info("Traffic sync job stopped gracefully")
			return nil
		case <-ticker.C:
			j.syncOnce(ctx)
		}
	}
}

func (j *TrafficSyncJob) syncOnce(ctx context.Context) {
	if j.xrayManager == nil {
		return
	}

	// 查询增量统计数据并重置计数
	stats, err := j.xrayManager.QueryTrafficStats(ctx, true)
	if err != nil {
		return
	}

	today := time.Now().Format("2006-01-02")
	sec := int64(j.interval.Seconds())
	if sec <= 0 {
		sec = 15
	}
	nowMs := time.Now().UnixMilli()

	// 临时聚合本次轮询的速率
	userDeltaUp := make(map[string]int64)
	userDeltaDown := make(map[string]int64)

	// 确保重置 Xray 统计后，数据的写入持久化不受父级 ctx 取消影响，使用独立的超时保护以杜绝数据丢失
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

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
			// 若该用户在最近 6 秒内刚被执行重置，跳过本轮增量累加（过滤重置前的在途残留增量）
			if domain.IsUserRecentlyReset(s.Tag, 6000) {
				continue
			}

			if j.userRepo != nil {
				_ = j.userRepo.AddTraffic(writeCtx, s.Tag, up, down)
			}
			if up > 0 {
				userDeltaUp[s.Tag] += up
			}
			if down > 0 {
				userDeltaDown[s.Tag] += down
			}

			// 异步/同步写入每日历史记录
			if j.userRepo != nil {
				user, err := j.userRepo.GetByEmail(writeCtx, s.Tag)
				if err == nil && user != nil {
					if j.trafficLogRepo != nil {
						_ = j.trafficLogRepo.RecordTraffic(writeCtx, user.ID, s.Tag, up, down, today)
					}

					// 检查用户是否处于非活跃状态（禁用、过期或超额），非活跃则立即从 Xray 所有节点剔除
					if !user.IsActive() {
						for _, t := range user.GetInboundTagList() {
							_ = j.xrayManager.RemoveUser(writeCtx, t, user.Email)
						}
					}
				}
			}
		} else if s.Type == domain.TrafficStatTypeInbound {
			if j.inboundRepo != nil {
				_ = j.inboundRepo.AddTraffic(writeCtx, s.Tag, up, down)
			}
		}
	}

	// 更新速度追踪器 (有增量计算速率，无增量即时将瞬时速率置零)
	allTracked := domain.GetAllUserRuntimeSpeeds()
	for email := range allTracked {
		up := userDeltaUp[email]
		down := userDeltaDown[email]
		if up > 0 || down > 0 {
			domain.SetUserRuntimeSpeed(email, up/sec, down/sec, nowMs)
		} else {
			// 本轮周期无新增流量，立即将瞬时速率置零
			domain.SetUserRuntimeSpeed(email, 0, 0, 0)
		}
	}
	for email, up := range userDeltaUp {
		if _, ok := allTracked[email]; !ok {
			down := userDeltaDown[email]
			domain.SetUserRuntimeSpeed(email, up/sec, down/sec, nowMs)
		}
	}
}
