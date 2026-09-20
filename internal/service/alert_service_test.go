package service

import (
	"context"
	"strings"
	"testing"

	"panel/internal/domain"
)

type mockNotifier struct {
	trafficAlerts []domain.TrafficAlert
	systemAlerts  []domain.SystemAlert
	certAlerts    []domain.CertAlert
	messages      []string
}

func (m *mockNotifier) SendTrafficAlert(ctx context.Context, alert domain.TrafficAlert) error {
	m.trafficAlerts = append(m.trafficAlerts, alert)
	return nil
}

func (m *mockNotifier) SendSystemAlert(ctx context.Context, alert domain.SystemAlert) error {
	m.systemAlerts = append(m.systemAlerts, alert)
	return nil
}

func (m *mockNotifier) SendServiceStatusAlert(ctx context.Context, status domain.ServiceStatus) error {
	return nil
}

func (m *mockNotifier) SendCertAlert(ctx context.Context, alert domain.CertAlert) error {
	m.certAlerts = append(m.certAlerts, alert)
	return nil
}

func (m *mockNotifier) SendMessage(ctx context.Context, text string) error {
	m.messages = append(m.messages, text)
	return nil
}

type mockHostMonitor struct {
	metrics *domain.SystemMetrics
}

func (m *mockHostMonitor) GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error) {
	return m.metrics, nil
}

func (m *mockHostMonitor) GetNetworkSpeed(ctx context.Context) (uint64, uint64, error) {
	return 0, 0, nil
}

func TestAlertService_DebounceTrafficAlerts(t *testing.T) {
	notifier := &mockNotifier{}
	userRepo := &mockUserRepo{
		users: []domain.User{
			{
				ID:         1,
				Email:      "user1@test.com",
				Enabled:    true,
				UpBytes:    500,
				DownBytes:  400,
				TotalBytes: 1000, // 90% usage
			},
		},
	}

	svc := NewAlertService(notifier, userRepo, nil, nil)

	// 第一次检查，应该触发告警
	if err := svc.CheckTrafficQuotas(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.trafficAlerts) != 1 {
		t.Fatalf("expected 1 traffic alert, got %d", len(notifier.trafficAlerts))
	}

	// 紧接着第二次检查，命中缓存防抖，不应触发新告警
	if err := svc.CheckTrafficQuotas(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.trafficAlerts) != 1 {
		t.Fatalf("expected still 1 traffic alert due to debounce cache, got %d", len(notifier.trafficAlerts))
	}
}

func TestAlertService_DebounceSystemLoad(t *testing.T) {
	notifier := &mockNotifier{}
	monitor := &mockHostMonitor{
		metrics: &domain.SystemMetrics{
			CPUUsagePercent: 95.0,
			MemoryUsagePct:  95.0,
			DiskUsagePct:    90.0,
		},
	}

	svc := NewAlertService(notifier, nil, monitor, nil)

	// 第一次检查，触发 CPU、Memory、Disk 告警
	if err := svc.CheckSystemLoad(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.systemAlerts) != 3 {
		t.Fatalf("expected 3 system alerts, got %d", len(notifier.systemAlerts))
	}

	// 第二次检查，防抖命中，不重复发送
	if err := svc.CheckSystemLoad(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.systemAlerts) != 3 {
		t.Fatalf("expected still 3 system alerts, got %d", len(notifier.systemAlerts))
	}
}

type mockCertInspector struct {
	paths []string
}

func (m *mockCertInspector) GetCertificatePaths() []string {
	return m.paths
}

func TestAlertService_CheckCertificates_Empty(t *testing.T) {
	notifier := &mockNotifier{}
	inspector := &mockCertInspector{paths: []string{}}
	svc := NewAlertService(notifier, nil, nil, inspector)

	if err := svc.CheckCertificates(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.certAlerts) != 0 {
		t.Fatalf("expected 0 cert alerts, got %d", len(notifier.certAlerts))
	}
}

func TestNotifyRealityAbnormal(t *testing.T) {
	notifier := &mockNotifier{}
	svc := NewAlertService(notifier, nil, nil, nil)
	ctx := context.Background()

	item1 := domain.RealityCheckItem{
		InboundTag: "vless-reality-1",
		ServerName: "apple.com",
		ErrorType:  domain.ErrTypeTLSVersionLow,
		Status:     domain.RealityStatusError,
		Details:    "TLS 协商版本过低",
	}

	// 1. 首次触发 Error，应该成功发送
	if err := svc.NotifyRealityAbnormal(ctx, item1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(notifier.messages))
	}

	// 检查消息内容
	msg := notifier.messages[0]
	if !strings.Contains(msg, "vless-reality-1") ||
		!strings.Contains(msg, "apple.com") ||
		!strings.Contains(msg, domain.ErrTypeTLSVersionLow) ||
		!strings.Contains(msg, "TLS 协商版本过低") {
		t.Fatalf("message missing critical fields: %s", msg)
	}

	// 2. 相同 tag, serverName, errorType 再次触发，应被 24h 冷却拦截
	if err := svc.NotifyRealityAbnormal(ctx, item1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.messages) != 1 {
		t.Fatalf("expected message count to stay 1 due to 24h debounce, got %d", len(notifier.messages))
	}

	// 3. 相同 tag, 相同 serverName, 但不同 errorType，应能成功发送
	itemDifferentErr := domain.RealityCheckItem{
		InboundTag: "vless-reality-1",
		ServerName: "apple.com",
		ErrorType:  domain.ErrTypeCDNDetected,
		Status:     domain.RealityStatusError,
		Details:    "目标套用 Cloudflare CDN",
	}
	if err := svc.NotifyRealityAbnormal(ctx, itemDifferentErr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(notifier.messages))
	}

	// 4. 相同 tag, 不同 serverName，应能成功发送
	itemDifferentSN := domain.RealityCheckItem{
		InboundTag: "vless-reality-1",
		ServerName: "microsoft.com",
		ErrorType:  domain.ErrTypeTLSVersionLow,
		Status:     domain.RealityStatusError,
		Details:    "TLS 协商版本过低",
	}
	if err := svc.NotifyRealityAbnormal(ctx, itemDifferentSN); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(notifier.messages))
	}

	// 5. 状态为 Ok 时不发送
	itemOk := domain.RealityCheckItem{
		InboundTag: "vless-reality-2",
		ServerName: "google.com",
		Status:     domain.RealityStatusOk,
	}
	if err := svc.NotifyRealityAbnormal(ctx, itemOk); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.messages) != 3 {
		t.Fatalf("expected ok item not to trigger message, got %d", len(notifier.messages))
	}
}
