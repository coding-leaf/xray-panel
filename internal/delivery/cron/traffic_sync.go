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

// XrayTrafficManager 定义定时任务所需的最小 Xray 交互接口 (遵循 ISP 接口隔离原则)
type XrayTrafficManager interface {
	QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error)
	RemoveUser(ctx context.Context, inboundTag string, email string) error
}

type TrafficSyncJob struct {
	xrayManager    XrayTrafficManager
	userRepo       domain.UserRepository
	inboundRepo    domain.InboundRepository
	trafficLogRepo domain.TrafficLogRepository
	alertSvc       *service.AlertService
	userSvc        *service.UserService
	ticketRepo     domain.TicketRepository
	interval       time.Duration
}

func NewTrafficSyncJob(
	xrayManager XrayTrafficManager,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	trafficLogRepo domain.TrafficLogRepository,
	alertSvc *service.AlertService,
	userSvc *service.UserService,
	ticketRepo domain.TicketRepository,
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
		ticketRepo:     ticketRepo,
		interval:       interval,
	}
}

func (j *TrafficSyncJob) Start(ctx context.Context) error {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	alertTicker := time.NewTicker(5 * time.Minute)
	ticketCleanTicker := time.NewTicker(1 * time.Minute)

	slog.Info("Traffic sync job started", slog.Duration("interval", j.interval))

	var wg sync.WaitGroup

	// 1. 独立外部告警与周期维护协程：执行 Telegram 外部网络通知与月度流量重置
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer alertTicker.Stop()
		defer ticketCleanTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticketCleanTicker.C:
				if j.ticketRepo != nil {
					_ = j.ticketRepo.CleanExpired(ctx)
				}
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

	type trafficDelta struct {
		up   int64
		down int64
	}
	userDeltas := make(map[string]*trafficDelta)
	inboundDeltas := make(map[string]*trafficDelta)

	for _, s := range stats {
		if s.Value <= 0 {
			continue
		}

		if s.Type == domain.TrafficStatTypeUser {
			// 若该用户在最近 6 秒内刚被执行重置，跳过本轮增量累加（过滤重置前的在途残留增量）
			if domain.IsUserRecentlyReset(s.Tag, 6000) {
				continue
			}

			d, ok := userDeltas[s.Tag]
			if !ok {
				d = &trafficDelta{}
				userDeltas[s.Tag] = d
			}
			if s.IsUplink {
				d.up += s.Value
				userDeltaUp[s.Tag] += s.Value
			} else {
				d.down += s.Value
				userDeltaDown[s.Tag] += s.Value
			}
		} else if s.Type == domain.TrafficStatTypeInbound {
			d, ok := inboundDeltas[s.Tag]
			if !ok {
				d = &trafficDelta{}
				inboundDeltas[s.Tag] = d
			}
			if s.IsUplink {
				d.up += s.Value
			} else {
				d.down += s.Value
			}
		}
	}

	// 1. 持久化流量增量：优先使用单一原子事务批量落盘，消除多句独立写入的 fsync 延迟与锁竞争
	batchRepo, hasBatch := j.userRepo.(domain.TrafficBatchRepository)
	if hasBatch {
		uDeltas := make([]domain.UserTrafficDelta, 0, len(userDeltas))
		for email, d := range userDeltas {
			if d.up > 0 || d.down > 0 {
				uDeltas = append(uDeltas, domain.UserTrafficDelta{
					Email: email,
					Up:    d.up,
					Down:  d.down,
				})
			}
		}

		inDeltas := make([]domain.InboundTrafficDelta, 0, len(inboundDeltas))
		for tag, d := range inboundDeltas {
			if d.up > 0 || d.down > 0 {
				inDeltas = append(inDeltas, domain.InboundTrafficDelta{
					Tag:  tag,
					Up:   d.up,
					Down: d.down,
				})
			}
		}

		if len(uDeltas) > 0 || len(inDeltas) > 0 {
			updatedUsers, err := batchRepo.BatchSyncTraffic(writeCtx, uDeltas, inDeltas, today)
			if err != nil {
				slog.Error("Failed to batch sync traffic in single transaction", slog.String("error", err.Error()))
			} else {
				// 检查用户是否处于非活跃状态（禁用、过期或超额），非活跃则立即从 Xray 所有节点剔除
				for _, user := range updatedUsers {
					if !user.IsActive() {
						for _, t := range user.GetInboundTagList() {
							_ = j.xrayManager.RemoveUser(writeCtx, t, user.Email)
						}
					}
				}
			}
		}
	} else {
		// 回退兼容：逐条单句更新 (保证 mock 仓储测试与非批量实现的兼容)
		for email, d := range userDeltas {
			if d.up <= 0 && d.down <= 0 {
				continue
			}

			if j.userRepo != nil {
				_ = j.userRepo.AddTraffic(writeCtx, email, d.up, d.down)
				user, err := j.userRepo.GetByEmail(writeCtx, email)
				if err == nil && user != nil {
					if j.trafficLogRepo != nil {
						_ = j.trafficLogRepo.RecordTraffic(writeCtx, user.ID, email, d.up, d.down, today)
					}

					// 检查用户是否处于非活跃状态（禁用、过期或超额），非活跃则立即从 Xray 所有节点剔除
					if !user.IsActive() {
						for _, t := range user.GetInboundTagList() {
							_ = j.xrayManager.RemoveUser(writeCtx, t, user.Email)
						}
					}
				}
			}
		}

		// 统一持久化入站流量增量
		for tag, d := range inboundDeltas {
			if d.up <= 0 && d.down <= 0 {
				continue
			}
			if j.inboundRepo != nil {
				_ = j.inboundRepo.AddTraffic(writeCtx, tag, d.up, d.down)
			}
		}
	}

	// 更新速度追踪器：单次原子批量更新，消除逐用户抢占写锁的颠簸
	speedUpdates := make([]domain.UserTrafficDeltaUpdate, 0, len(userDeltaUp)+len(userDeltaDown))
	seenEmails := make(map[string]struct{}, len(userDeltaUp)+len(userDeltaDown))
	for email, up := range userDeltaUp {
		seenEmails[email] = struct{}{}
		speedUpdates = append(speedUpdates, domain.UserTrafficDeltaUpdate{
			Email: email,
			Up:    up,
			Down:  userDeltaDown[email],
		})
	}
	for email, down := range userDeltaDown {
		if _, ok := seenEmails[email]; !ok {
			speedUpdates = append(speedUpdates, domain.UserTrafficDeltaUpdate{
				Email: email,
				Up:    0,
				Down:  down,
			})
		}
	}
	domain.BatchUpdateUserRuntimeSpeeds(speedUpdates, sec, nowMs)
}
