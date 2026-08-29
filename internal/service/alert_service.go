package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/pkg/logger"

	gocache "github.com/patrickmn/go-cache"
)

type AlertService struct {
	notifier  domain.Notifier
	userRepo  domain.UserRepository
	monitor   domain.HostMonitor
	configMgr *xray.ConfigManager
	cache     *gocache.Cache
}

func NewAlertService(notifier domain.Notifier, userRepo domain.UserRepository, monitor domain.HostMonitor, configMgr *xray.ConfigManager) *AlertService {
	return &AlertService{
		notifier:  notifier,
		userRepo:  userRepo,
		monitor:   monitor,
		configMgr: configMgr,
		cache:     gocache.New(24*time.Hour, 1*time.Hour),
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
			cacheKey := fmt.Sprintf("traffic_alert:%s", u.Email)
			if _, found := s.cache.Get(cacheKey); found {
				continue // 处于冷却期内，跳过重复告警
			}

			alert := domain.TrafficAlert{
				Email:      u.Email,
				UsedBytes:  used,
				TotalBytes: u.TotalBytes,
				UsageRatio: ratio,
			}
			if err := s.notifier.SendTrafficAlert(ctx, alert); err == nil {
				s.cache.Set(cacheKey, true, 24*time.Hour)
			} else {
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
		cacheKey := "system_alert:cpu"
		if _, found := s.cache.Get(cacheKey); !found {
			if err := s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
				Metric:      "CPU 使用率",
				CurrentVal:  metrics.CPUUsagePercent,
				Threshold:   90.0,
				Description: "CPU 负载过高，可能存在异常进程消耗",
			}); err == nil {
				s.cache.Set(cacheKey, true, 1*time.Hour)
			}
		}
	}

	// 内存报警阈值 90%
	if metrics.MemoryUsagePct > 90.0 {
		cacheKey := "system_alert:memory"
		if _, found := s.cache.Get(cacheKey); !found {
			if err := s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
				Metric:      "内存占用率",
				CurrentVal:  metrics.MemoryUsagePct,
				Threshold:   90.0,
				Description: fmt.Sprintf("物理内存占用已达 %.1f%%", metrics.MemoryUsagePct),
			}); err == nil {
				s.cache.Set(cacheKey, true, 1*time.Hour)
			}
		}
	}

	// 磁盘报警阈值 85%
	if metrics.DiskUsagePct > 85.0 {
		cacheKey := "system_alert:disk"
		if _, found := s.cache.Get(cacheKey); !found {
			if err := s.notifier.SendSystemAlert(ctx, domain.SystemAlert{
				Metric:      "磁盘占用率",
				CurrentVal:  metrics.DiskUsagePct,
				Threshold:   85.0,
				Description: "磁盘可用空间不足，请及时清理日志或扩容",
			}); err == nil {
				s.cache.Set(cacheKey, true, 1*time.Hour)
			}
		}
	}

	return nil
}

func (s *AlertService) CheckCertificates(ctx context.Context) error {
	if s.configMgr == nil {
		return nil
	}
	certPaths := s.configMgr.GetCertificatePaths()
	for _, p := range certPaths {
		certInfo, err := xray.ParseCertFile(p)
		if err != nil {
			continue
		}
		// 剩余天数 <= 15 天时触发预警
		if certInfo.DaysLeft <= 15 {
			cacheKey := fmt.Sprintf("cert_alert:%s", certInfo.DomainName)
			if _, found := s.cache.Get(cacheKey); !found {
				alert := domain.CertAlert{
					DomainName: certInfo.DomainName,
					DaysLeft:   certInfo.DaysLeft,
					NotAfter:   certInfo.NotAfter.Format("2006-01-02 15:04:05"),
					Path:       certInfo.Path,
				}
				if err := s.notifier.SendCertAlert(ctx, alert); err == nil {
					s.cache.Set(cacheKey, true, 24*time.Hour)
				}
			}
		}
	}
	return nil
}

