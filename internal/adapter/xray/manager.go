package xray

import (
	"context"
	"log/slog"
	"strings"

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

func isIgnorableUserManagerError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not a usermanager") ||
		strings.Contains(msg, "does not support dynamic account operations")
}

func (m *Manager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag)
	if err != nil {
		return err
	}
	if !domain.SupportsDynamicUser(inbound.Protocol) {
		slog.Debug("skip AddUser: inbound does not support dynamic user management", "protocol", inbound.Protocol, "tag", inboundTag)
		return nil
	}
	err = m.grpcClient.AddUser(ctx, inbound, user)
	if err != nil && isIgnorableUserManagerError(err) {
		slog.Debug("ignoring dynamic user error on AddUser", "protocol", inbound.Protocol, "tag", inboundTag, "error", err)
		return nil
	}
	return err
}

func (m *Manager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	if m.inboundRepo != nil {
		if inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag); err == nil && inbound != nil {
			if !domain.SupportsDynamicUser(inbound.Protocol) {
				return nil
			}
		}
	}
	err := m.grpcClient.RemoveUser(ctx, inboundTag, email)
	if err != nil && isIgnorableUserManagerError(err) {
		return nil
	}
	return err
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
