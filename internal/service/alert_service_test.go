package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockNotifier struct {
	trafficAlerts []domain.TrafficAlert
	systemAlerts  []domain.SystemAlert
	certAlerts    []domain.CertAlert
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
