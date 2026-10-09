package service

import (
	"context"
	"errors"
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

type mockConfigurableInboundRepo struct {
	domain.InboundRepository
	inbounds  []domain.Inbound
	returnErr error
}

func (m *mockConfigurableInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.inbounds, nil
}

type mockConfigurableUserRepo struct {
	domain.UserRepository
	users     []domain.User
	returnErr error
}

func (m *mockConfigurableUserRepo) ListAll(ctx context.Context) ([]domain.User, error) {
	if m.returnErr != nil {
		return nil, m.returnErr
	}
	return m.users, nil
}

func TestMonitorService_DashboardTraffic_InboundAccounting(t *testing.T) {
	ctx := context.Background()

	// 1. 测试常规与本机入口、禁用入站计入，管理 api 排除，且用户累计不等于入站累计
	uRepo := &mockConfigurableUserRepo{
		users: []domain.User{
			{ID: 1, Email: "user1@example.com", UpBytes: 50, DownBytes: 100, Enabled: true},
		},
	}
	inbRepo := &mockConfigurableInboundRepo{
		inbounds: []domain.Inbound{
			{ID: 1, Tag: "local", Protocol: "socks", UpBytes: 5000, DownBytes: 10000, Enabled: true},
			{ID: 2, Tag: "vless-in", Protocol: "vless", UpBytes: 2000, DownBytes: 4000, Enabled: true},
			{ID: 3, Tag: "disabled-in", Protocol: "trojan", UpBytes: 1000, DownBytes: 2000, Enabled: false},
			{ID: 4, Tag: "api", Protocol: "dokodemo-door", UpBytes: 9999, DownBytes: 9999, Enabled: true},
		},
	}

	svc := NewMonitorService(&mockDashHostMonitor{}, &mockXrayStatusProvider{}, uRepo, inbRepo)
	data, err := svc.GetDashboardData(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 用户人数与活跃用户仍由用户仓储统计
	if data.UserCount != 1 || data.ActiveUsers != 1 {
		t.Errorf("expected UserCount 1, ActiveUsers 1; got UserCount %d, ActiveUsers %d", data.UserCount, data.ActiveUsers)
	}

	// TotalUp / TotalDown 必须为现存业务入站累计 (5000+2000+1000 = 8000, 10000+4000+2000 = 16000), 排除 api (9999) 且不等于用户统计 (50, 100)
	if data.TotalUp != 8000 {
		t.Errorf("expected TotalUp 8000 (inbounds only), got %d", data.TotalUp)
	}
	if data.TotalDown != 16000 {
		t.Errorf("expected TotalDown 16000 (inbounds only), got %d", data.TotalDown)
	}

	// 2. 无业务入站时必须为 0，严禁回退到用户累计
	svc.InvalidateCache()
	inbRepo.inbounds = []domain.Inbound{
		{ID: 4, Tag: "api", UpBytes: 9999, DownBytes: 9999, Enabled: true},
	}
	dataEmptyInb, err := svc.GetDashboardData(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dataEmptyInb.TotalUp != 0 || dataEmptyInb.TotalDown != 0 {
		t.Errorf("expected 0 for empty business inbounds without falling back to users, got up=%d, down=%d", dataEmptyInb.TotalUp, dataEmptyInb.TotalDown)
	}

	// 3. 入站仓储读取失败时必须显式返回错误，禁止返回看似正常的虚假零值
	svc.InvalidateCache()
	inbRepo.returnErr = errors.New("sqlite disk i/o error")
	_, err = svc.GetDashboardData(ctx)
	if err == nil {
		t.Fatalf("expected error when inbound ListAll fails, got nil")
	}
}
