package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"panel/internal/domain"
)

type mockDashHostMonitor struct{}

func (m *mockDashHostMonitor) GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error) {
	return &domain.SystemMetrics{
		CPUUsagePercent: 12.5,
		MemoryUsagePct:  45.0,
	}, nil
}

func (m *mockDashHostMonitor) GetNetworkSpeed(ctx context.Context) (uint64, uint64, error) {
	return 0, 0, nil
}

type mockXrayStatusProvider struct{}

func (m *mockXrayStatusProvider) GetServiceStatus(ctx context.Context) (domain.ServiceStatus, error) {
	return domain.ServiceStatus{Active: true}, nil
}

func (m *mockXrayStatusProvider) GetVersion(ctx context.Context) (string, error) {
	return "1.8.0", nil
}

type mockCountingUserRepo struct {
	listAllCount int64
}

func (m *mockCountingUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockCountingUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (m *mockCountingUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	return nil
}
func (m *mockCountingUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockCountingUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	return nil, nil
}
func (m *mockCountingUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	return nil, nil
}
func (m *mockCountingUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}
func (m *mockCountingUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	return nil, nil
}
func (m *mockCountingUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (m *mockCountingUserRepo) ListAll(ctx context.Context) ([]domain.User, error) {
	atomic.AddInt64(&m.listAllCount, 1)
	return []domain.User{
		{ID: 1, Email: "u1@test.com", UpBytes: 100, DownBytes: 200, Enabled: true},
	}, nil
}
func (m *mockCountingUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (m *mockCountingUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

type mockCountingInboundRepo struct {
	listAllCount int64
}

func (m *mockCountingInboundRepo) Create(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockCountingInboundRepo) Update(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockCountingInboundRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockCountingInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockCountingInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockCountingInboundRepo) GetByPort(ctx context.Context, port int) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockCountingInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	atomic.AddInt64(&m.listAllCount, 1)
	return []domain.Inbound{
		{ID: 1, Tag: "vless-in", Port: 443},
	}, nil
}
func (m *mockCountingInboundRepo) AddTraffic(ctx context.Context, tag string, up, down int64) error {
	return nil
}

func TestMonitorService_DashboardCache(t *testing.T) {
	uRepo := &mockCountingUserRepo{}
	inbRepo := &mockCountingInboundRepo{}
	svc := NewMonitorService(&mockDashHostMonitor{}, &mockXrayStatusProvider{}, uRepo, inbRepo)

	ctx := context.Background()

	// 1. 并发 20 个请求
	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			data, err := svc.GetDashboardData(ctx)
			if err != nil {
				t.Errorf("GetDashboardData error: %v", err)
				return
			}
			if data.UserCount != 1 {
				t.Errorf("expected UserCount 1, got %d", data.UserCount)
			}
		}()
	}
	wg.Wait()

	// 缓存命中下，ListAll 应仅被调用 1 次
	uCount := atomic.LoadInt64(&uRepo.listAllCount)
	inbCount := atomic.LoadInt64(&inbRepo.listAllCount)
	if uCount != 1 {
		t.Fatalf("expected 1 user ListAll call within cache TTL, got %d", uCount)
	}
	if inbCount != 1 {
		t.Fatalf("expected 1 inbound ListAll call within cache TTL, got %d", inbCount)
	}

	// 2. 测试主动失效 InvalidateCache 与动态 TTL
	svc.SetCacheTTL(50 * time.Millisecond)
	svc.InvalidateCache()
	_, err := svc.GetDashboardData(ctx)
	if err != nil {
		t.Fatalf("GetDashboardData failed after InvalidateCache: %v", err)
	}
	if uCountAfter := atomic.LoadInt64(&uRepo.listAllCount); uCountAfter != 2 {
		t.Fatalf("expected 2 calls after InvalidateCache, got %d", uCountAfter)
	}

	// 3. 测试短 TTL 自然过期
	time.Sleep(70 * time.Millisecond)
	_, err = svc.GetDashboardData(ctx)
	if err != nil {
		t.Fatalf("GetDashboardData failed after TTL expire: %v", err)
	}
	if uCountExpired := atomic.LoadInt64(&uRepo.listAllCount); uCountExpired != 3 {
		t.Fatalf("expected 3 calls after TTL expired, got %d", uCountExpired)
	}
}
