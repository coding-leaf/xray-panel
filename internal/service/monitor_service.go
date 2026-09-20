package service

import (
	"context"
	"sync"
	"time"

	"panel/internal/domain"
)

type DashboardData struct {
	Metrics     *domain.SystemMetrics `json:"metrics"`
	Service     domain.ServiceStatus  `json:"service"`
	UserCount   int                   `json:"userCount"`
	ActiveUsers int                   `json:"activeUsers"`
	Inbounds    []domain.Inbound      `json:"inbounds"`
	TotalUp     int64                 `json:"totalUp"`
	TotalDown   int64                 `json:"totalDown"`
}

// XrayStatusProvider 定义监控服务所需的最小 Xray 状态查询接口 (遵循 ISP 接口隔离原则)
type XrayStatusProvider interface {
	GetServiceStatus(ctx context.Context) (domain.ServiceStatus, error)
	GetVersion(ctx context.Context) (string, error)
}

type MonitorService struct {
	monitor     domain.HostMonitor
	xrayStatus  XrayStatusProvider
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository

	cacheMu        sync.RWMutex
	cachedData     *DashboardData
	cacheExpiresAt time.Time
	cacheTTL       time.Duration
}

func NewMonitorService(
	monitor domain.HostMonitor,
	xrayStatus XrayStatusProvider,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
) *MonitorService {
	return &MonitorService{
		monitor:     monitor,
		xrayStatus:  xrayStatus,
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		cacheTTL:    2 * time.Second,
	}
}

// InvalidateCache 主动失效当前 Dashboard 快照缓存
func (s *MonitorService) InvalidateCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cachedData = nil
	s.cacheExpiresAt = time.Time{}
}

// SetCacheTTL 自定义 Dashboard 快照缓存有效时长 (默认 2 秒)
func (s *MonitorService) SetCacheTTL(ttl time.Duration) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cacheTTL = ttl
}

func (s *MonitorService) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	now := time.Now()

	// 1. 快速读取锁检查缓存
	s.cacheMu.RLock()
	if s.cachedData != nil && now.Before(s.cacheExpiresAt) {
		res := *s.cachedData
		s.cacheMu.RUnlock()
		return &res, nil
	}
	s.cacheMu.RUnlock()

	// 2. 升写锁二次双重检查（DCL），阻断并发雪崩穿透数据库
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now = time.Now()
	if s.cachedData != nil && now.Before(s.cacheExpiresAt) {
		res := *s.cachedData
		return &res, nil
	}

	metrics, err := s.monitor.GetSystemMetrics(ctx)
	if err != nil {
		metrics = &domain.SystemMetrics{}
	}

	serviceStatus, _ := s.xrayStatus.GetServiceStatus(ctx)
	xrayVer, _ := s.xrayStatus.GetVersion(ctx)
	metrics.XrayRunning = serviceStatus.Active
	metrics.XrayVersion = xrayVer

	users, _ := s.userRepo.ListAll(ctx)
	inbounds, _ := s.inboundRepo.ListAll(ctx)

	var totalUp, totalDown int64
	var activeCount int
	for _, u := range users {
		totalUp += u.UpBytes
		totalDown += u.DownBytes
		if u.IsActive() {
			activeCount++
		}
	}

	data := &DashboardData{
		Metrics:     metrics,
		Service:     serviceStatus,
		UserCount:   len(users),
		ActiveUsers: activeCount,
		Inbounds:    inbounds,
		TotalUp:     totalUp,
		TotalDown:   totalDown,
	}

	ttl := s.cacheTTL
	if ttl <= 0 {
		ttl = 2 * time.Second
	}
	s.cachedData = data
	s.cacheExpiresAt = now.Add(ttl)

	ret := *data
	return &ret, nil
}


func (s *MonitorService) GetServiceStatus(ctx context.Context) (domain.ServiceStatus, string, error) {
	serviceStatus, err := s.xrayStatus.GetServiceStatus(ctx)
	xrayVer, _ := s.xrayStatus.GetVersion(ctx)
	return serviceStatus, xrayVer, err
}
