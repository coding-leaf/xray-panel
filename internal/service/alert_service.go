package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"html"
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
	nodeRepo  domain.NodeRepository
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

// WithNodeQuota injects the node repository for monthly quota warnings and depletion forecasting.
func (s *AlertService) WithNodeQuota(nodeRepo domain.NodeRepository) *AlertService {
	s.nodeRepo = nodeRepo
	return s
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

// NotifyRealityAbnormal sends a notification when a Reality domain check yields warning or error,
// with a 24-hour debounce cooldown per (inboundTag, serverName, errorType).
func (s *AlertService) NotifyRealityAbnormal(ctx context.Context, item domain.RealityCheckItem) error {
	if item.Status == domain.RealityStatusOk || item.Status == "" {
		return nil
	}
	if s.notifier == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("reality_alert:%s:%s:%s", item.InboundTag, item.ServerName, item.ErrorType)
	if _, found := s.cache.Get(cacheKey); found {
		return nil // 冷却期内，跳过重复告警
	}

	statusBadge := "🟠 预警 (Warning)"
	if item.Status == domain.RealityStatusError {
		statusBadge = "🔴 异常 (Error)"
	}

	text := fmt.Sprintf(
		"⚠️ <b>Reality 域名合规性告警</b>\n\n"+
			"🏷️ <b>入站标签:</b> <code>%s</code>\n"+
			"🌐 <b>目标域名:</b> <code>%s</code>\n"+
			"🚨 <b>错误类型:</b> %s\n"+
			"📊 <b>健康状态:</b> %s\n"+
			"📝 <b>详细描述:</b> %s\n"+
			"⏰ <b>检测时间:</b> %s",
		item.InboundTag,
		item.ServerName,
		item.ErrorType,
		statusBadge,
		item.Details,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	if err := s.notifier.SendMessage(ctx, text); err != nil {
		logger.FromContext(ctx).Warn("Send reality abnormal alert failed",
			slog.String("inbound_tag", item.InboundTag),
			slog.String("server_name", item.ServerName),
			slog.String("error_type", item.ErrorType),
			slog.String("error", err.Error()),
		)
		return err
	}

	s.cache.Set(cacheKey, true, 24*time.Hour)
	return nil
}

// CheckNodeQuotas checks all enabled quota-limited nodes against their warn threshold with 24h cooldown.
func (s *AlertService) CheckNodeQuotas(ctx context.Context) error {
	return s.CheckNodeQuotasAt(ctx, time.Now())
}

// CheckNodeQuotasAt evaluates node quota warnings and 24h depletion forecasts relative to now.
func (s *AlertService) CheckNodeQuotasAt(ctx context.Context, now time.Time) error {
	if s.nodeRepo == nil || s.notifier == nil {
		return nil
	}

	nodes, err := s.nodeRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	defaultCycleMonth := now.Year()*100 + int(now.Month())
	for _, n := range nodes {
		if !n.Enabled || n.TrafficLimitBytes <= 0 {
			continue
		}
		used := n.CalcCycleUsedBytes()
		warnPct, _, _ := n.EffectiveThresholds()
		if !domain.ReachedQuotaThreshold(used, n.TrafficLimitBytes, warnPct) {
			continue
		}

		cycleMonth := n.LastResetMonth
		if cycleMonth <= 0 {
			cycleMonth = defaultCycleMonth
		}
		cacheKey := fmt.Sprintf("node_alert:%d:warn:%d", n.ID, cycleMonth)
		if _, found := s.cache.Get(cacheKey); found {
			continue
		}

		usagePct := float64(used) * 100.0 / float64(n.TrafficLimitBytes)
		forecast := s.estimateNodeDepletion(ctx, n, used, now)
		usedGB := float64(used) / (1024 * 1024 * 1024)
		limitGB := float64(n.TrafficLimitBytes) / (1024 * 1024 * 1024)
		state := n.DesiredState
		if state == "" {
			state = domain.NodeStateActive
		}

		text := fmt.Sprintf(
			"⚠️ <b>节点流量配额预警</b>\n\n"+
				"🖥️ <b>节点:</b> #%d <code>%s</code> (%s)\n"+
				"📊 <b>周期已用:</b> %.2f GB / %.2f GB (%.1f%%, 阈值 %d%%)\n"+
				"🚦 <b>当前状态:</b> <code>%s</code>\n"+
				"⏳ <b>按近24h速率预计剩余:</b> %s",
			n.ID,
			html.EscapeString(n.Name),
			html.EscapeString(n.Region),
			usedGB,
			limitGB,
			usagePct,
			warnPct,
			html.EscapeString(state),
			html.EscapeString(forecast),
		)

		if err := s.notifier.SendMessage(ctx, text); err == nil {
			s.cache.Set(cacheKey, true, 24*time.Hour)
		} else {
			logger.FromContext(ctx).Warn("Send node quota alert failed",
				slog.Uint64("node_id", uint64(n.ID)),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}

func (s *AlertService) estimateNodeDepletion(ctx context.Context, node domain.Node, usedBytes int64, now time.Time) string {
	remaining := node.TrafficLimitBytes - usedBytes
	if remaining <= 0 {
		return "已耗尽"
	}
	if s.nodeRepo == nil {
		return "暂无足够速率样本"
	}
	dailies, err := s.nodeRepo.ListRecentDailyTraffic(ctx, node.ID, 2)
	if err != nil || len(dailies) == 0 {
		return "暂无足够速率样本"
	}

	todayDate := now.Format("2006-01-02")
	yesterdayDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	var todayBytes, yesterdayBytes int64
	for _, d := range dailies {
		up, down := d.XrayUpBytes, d.XrayDownBytes
		if d.NicUpBytes > 0 || d.NicDownBytes > 0 {
			up, down = d.NicUpBytes, d.NicDownBytes
		}
		billable := domain.CalcTrafficByMode(up, down, node.EffectiveTrafficCountMode(), node.EffectiveMultiplier())
		switch d.Date {
		case todayDate:
			todayBytes = billable
		case yesterdayDate:
			yesterdayBytes = billable
		}
	}

	elapsedHours := now.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).Hours()
	var dailyRate float64
	switch {
	case todayBytes > 0 && yesterdayBytes > 0:
		dailyRate = float64(todayBytes) + float64(yesterdayBytes)*((24.0-elapsedHours)/24.0)
	case todayBytes > 0:
		if elapsedHours < 1.0 {
			elapsedHours = 1.0
		}
		dailyRate = float64(todayBytes) * (24.0 / elapsedHours)
	case yesterdayBytes > 0:
		dailyRate = float64(yesterdayBytes)
	}

	if dailyRate <= 0 {
		return "暂无足够速率样本"
	}
	daysLeft := float64(remaining) / dailyRate
	return fmt.Sprintf("约 %.1f 天", daysLeft)
}

// NotifyNodeStateChange sends an immediate Telegram notification for automatic node quota transitions or recovery.
func (s *AlertService) NotifyNodeStateChange(ctx context.Context, node *domain.Node, oldState, newState, reason string) {
	if s == nil || s.notifier == nil || node == nil {
		return
	}

	used := node.CalcCycleUsedBytes()
	usedGB := float64(used) / (1024 * 1024 * 1024)
	limitStr := "无限制"
	pctStr := "-"
	if node.TrafficLimitBytes > 0 {
		limitGB := float64(node.TrafficLimitBytes) / (1024 * 1024 * 1024)
		limitStr = fmt.Sprintf("%.2f GB", limitGB)
		pctStr = fmt.Sprintf("%.1f%%", float64(used)*100.0/float64(node.TrafficLimitBytes))
	}

	title := "🔔 <b>节点状态自动变更</b>"
	switch newState {
	case domain.NodeStateCircuitBroken:
		title = "🔴 <b>节点触发自动硬熔断 (停止业务入站)</b>"
	case domain.NodeStateDraining:
		title = "🟠 <b>节点触发自动软摘流 (停止新订阅分发)</b>"
	case domain.NodeStateActive:
		title = "🟢 <b>节点配额重置并自动恢复</b>"
	}

	text := fmt.Sprintf(
		"%s\n\n"+
			"🖥️ <b>节点:</b> #%d <code>%s</code>\n"+
			"🔄 <b>状态流转:</b> <code>%s</code> → <code>%s</code>\n"+
			"📊 <b>周期用量:</b> %.2f GB / %s (%s)\n"+
			"📝 <b>触发原因:</b> <code>%s</code>",
		title,
		node.ID,
		html.EscapeString(node.Name),
		html.EscapeString(oldState),
		html.EscapeString(newState),
		usedGB,
		limitStr,
		pctStr,
		html.EscapeString(reason),
	)

	if err := s.notifier.SendMessage(ctx, text); err != nil {
		logger.FromContext(ctx).Warn("Send node state change notification failed",
			slog.Uint64("node_id", uint64(node.ID)),
			slog.String("new_state", newState),
			slog.String("error", err.Error()),
		)
	}
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
