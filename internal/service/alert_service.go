package service

import (
	"context"
	"fmt"
	"log/slog"

	"panel/internal/domain"
	"panel/internal/pkg/logger"
)

type AlertService struct {
	notifier domain.Notifier
	userRepo domain.UserRepository
	monitor  domain.HostMonitor
}

func NewAlertService(notifier domain.Notifier, userRepo domain.UserRepository, monitor domain.HostMonitor) *AlertService {
	return &AlertService{
		notifier: notifier,
		userRepo: userRepo,
		monitor:  monitor,
	}
}

func (s *AlertService) CheckTrafficQuotas(ctx context.Context) error {
	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.TotalBytes <= 0 || !u.Enabled {
			continue
		}
		used := u.UpBytes + u.DownBytes
		ratio := float64(used) / float64(u.TotalBytes)

		// 达到 80% 或 100% 触发告警
		if ratio >= 0.8 {
			alert := domain.TrafficAlert{
				Email:      u.Email,
				UsedBytes:  used,
				TotalBytes: u.TotalBytes,
				UsageRatio: ratio,
			}
			if err := s.notifier.SendTrafficAlert(ctx, alert); err != nil {
				logger.FromContext(ctx).Warn("Send traffic alert failed", slog.String("email", u.Email))
			}
		}
	}
	return nil
}

func (s *AlertService) CheckSystemLoad(ctx context.Context) error {
	metrics, err := s.monitor.GetSystemMetrics(ctx)
	if err != nil {
		return err
	}

	// CPU 报警阈值 90%
	if metrics.CPUUsagePercent > 90.0 {
		_ = s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
			Metric:      "CPU 使用率",
			CurrentVal:  metrics.CPUUsagePercent,
			Threshold:   90.0,
			Description: "CPU 负载过高，可能存在异常进程消耗",
		})
	}

	// 内存报警阈值 90%
	if metrics.MemoryUsagePct > 90.0 {
		_ = s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
			Metric:      "内存占用率",
			CurrentVal:  metrics.MemoryUsagePct,
			Threshold:   90.0,
			Description: fmt.Sprintf("物理内存占用已达 %.1f%%", metrics.MemoryUsagePct),
		})
	}

	// 磁盘报警阈值 85%
	if metrics.DiskUsagePct > 85.0 {
		_ = s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
			Metric:      "磁盘占用率",
			CurrentVal:  metrics.DiskUsagePct,
			Threshold:   85.0,
			Description: "磁盘可用空间不足，请及时清理日志或扩容",
		})
	}

	return nil
}
