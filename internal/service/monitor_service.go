package service

import (
	"context"

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

type MonitorService struct {
	monitor     domain.HostMonitor
	xrayManager domain.XrayManager
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
}

func NewMonitorService(
	monitor domain.HostMonitor,
	xrayManager domain.XrayManager,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
) *MonitorService {
	return &MonitorService{
		monitor:     monitor,
		xrayManager: xrayManager,
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
	}
}

func (s *MonitorService) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	metrics, err := s.monitor.GetSystemMetrics(ctx)
	if err != nil {
		metrics = &domain.SystemMetrics{}
	}

	serviceStatus, _ := s.xrayManager.GetServiceStatus(ctx)
	xrayVer, _ := s.xrayManager.GetVersion(ctx)
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

	return &DashboardData{
		Metrics:     metrics,
		Service:     serviceStatus,
		UserCount:   len(users),
		ActiveUsers: activeCount,
		Inbounds:    inbounds,
		TotalUp:     totalUp,
		TotalDown:   totalDown,
	}, nil
}
