package cron

import (
	"context"
	"log/slog"
	"time"

	"panel/internal/domain"
	"panel/internal/service"
)

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

func (j *TrafficSyncJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	alertTicker := time.NewTicker(5 * time.Minute)

	slog.Info("Traffic sync job started", slog.Duration("interval", j.interval))

	// 1. 专属核心高频协程：执行 5s 流量同步、实时速率计算与到期/超额踢人 (完全隔绝外部网络 I/O 阻塞)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.syncOnce(ctx)
			}
		}
	}()

	// 2. 独立外部告警与周期维护协程：执行 Telegram 外部网络通知与月度流量重置
	go func() {
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
}

func (j *TrafficSyncJob) syncOnce(ctx context.Context) {
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

			_ = j.userRepo.AddTraffic(ctx, s.Tag, up, down)
			if up > 0 {
				userDeltaUp[s.Tag] += up
			}
			if down > 0 {
				userDeltaDown[s.Tag] += down
			}

			// 异步/同步写入每日历史记录
			user, err := j.userRepo.GetByEmail(ctx, s.Tag)
			if err == nil && user != nil {
				if j.trafficLogRepo != nil {
					_ = j.trafficLogRepo.RecordTraffic(ctx, user.ID, s.Tag, up, down, today)
				}

				// 检查用户是否处于非活跃状态（禁用、过期或超额），非活跃则立即从 Xray 所有节点剔除
				if !user.IsActive() {
					for _, t := range user.GetInboundTagList() {
						_ = j.xrayManager.RemoveUser(ctx, t, user.Email)
					}
				}
			}
		} else if s.Type == domain.TrafficStatTypeInbound {
			_ = j.inboundRepo.AddTraffic(ctx, s.Tag, up, down)
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
