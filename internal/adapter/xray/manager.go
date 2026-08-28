package xray

import (
	"context"

	"panel/internal/domain"
)

type Manager struct {
	grpcClient  *GRPCClient
	configMgr   *ConfigManager
	supervisor  *SystemdSupervisor
	inboundRepo domain.InboundRepository
}

func NewManager(grpcClient *GRPCClient, configMgr *ConfigManager, supervisor *SystemdSupervisor, inboundRepo domain.InboundRepository) *Manager {
	return &Manager{
		grpcClient:  grpcClient,
		configMgr:   configMgr,
		supervisor:  supervisor,
		inboundRepo: inboundRepo,
	}
}

func (m *Manager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag)
	if err != nil {
		return err
	}
	return m.grpcClient.AddUser(ctx, inboundTag, user, inbound.Protocol)
}

func (m *Manager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	return m.grpcClient.RemoveUser(ctx, inboundTag, email)
}

func (m *Manager) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	return m.grpcClient.QueryTrafficStats(ctx, reset)
}

func (m *Manager) ValidateConfig(ctx context.Context, rawJSON []byte) error {
	return m.configMgr.ValidateConfig(ctx, rawJSON)
}

func (m *Manager) ApplyConfigAndReload(ctx context.Context, rawJSON []byte) error {
	if err := m.configMgr.WriteConfig(ctx, rawJSON); err != nil {
		return err
	}
	return m.supervisor.Reload(ctx)
}

func (m *Manager) GetServiceStatus(ctx context.Context) (domain.ServiceStatus, error) {
	return m.supervisor.GetStatus(ctx)
}

func (m *Manager) RestartService(ctx context.Context) error {
	return m.supervisor.Restart(ctx)
}

func (m *Manager) GetVersion(ctx context.Context) (string, error) {
	return m.supervisor.GetVersion(ctx)
}
