package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"panel/internal/domain"
	"panel/internal/pkg/cache"
	"panel/internal/pkg/logger"
)

type CertificateInspector interface {
	GetCertificatePaths() []string
}

type certInfo struct {
	DomainName string
	DaysLeft   int
	NotAfter   time.Time
	Path       string
}

type AlertService struct {
	notifier  domain.Notifier
	userRepo  domain.UserRepository
	monitor   domain.HostMonitor
	configMgr CertificateInspector
	cache     *cache.Cache[bool]
}

func NewAlertService(notifier domain.Notifier, userRepo domain.UserRepository, monitor domain.HostMonitor, configMgr CertificateInspector) *AlertService {
	return &AlertService{
		notifier:  notifier,
		userRepo:  userRepo,
		monitor:   monitor,
		configMgr: configMgr,
		cache:     cache.New[bool](24*time.Hour, 1*time.Hour),
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
		info, err := parseCertFile(p)
		if err != nil {
			continue
		}
		// 剩余天数 <= 15 天时触发预警
		if info.DaysLeft <= 15 {
			cacheKey := fmt.Sprintf("cert_alert:%s", info.DomainName)
			if _, found := s.cache.Get(cacheKey); !found {
				alert := domain.CertAlert{
					DomainName: info.DomainName,
					DaysLeft:   info.DaysLeft,
					NotAfter:   info.NotAfter.Format("2006-01-02 15:04:05"),
					Path:       info.Path,
				}
				if err := s.notifier.SendCertAlert(ctx, alert); err == nil {
					s.cache.Set(cacheKey, true, 24*time.Hour)
				}
			}
		}
	}
	return nil
}

func parseCertFile(certPath string) (*certInfo, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read cert file failed: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block from %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse x509 cert failed: %w", err)
	}

	domainName := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		domainName = strings.Join(cert.DNSNames, ", ")
	}

	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	return &certInfo{
		DomainName: domainName,
		DaysLeft:   daysLeft,
		NotAfter:   cert.NotAfter,
		Path:       certPath,
	}, nil
}
