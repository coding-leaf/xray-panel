package cron_test

import (
	"context"
	"testing"
	"time"

	deliveryCron "panel/internal/delivery/cron"
	"panel/internal/domain"
)

type mockXrayManager struct {
	queryCount int
}

func (m *mockXrayManager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	return nil
}
func (m *mockXrayManager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	return nil
}
func (m *mockXrayManager) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	m.queryCount++
	return nil, nil
}
func (m *mockXrayManager) ValidateConfig(ctx context.Context, rawJSON []byte) error {
	return nil
}
func (m *mockXrayManager) ApplyConfigAndReload(ctx context.Context, rawJSON []byte) error {
	return nil
}
func (m *mockXrayManager) GetServiceStatus(ctx context.Context) (domain.ServiceStatus, error) {
	return domain.ServiceStatus{Active: true}, nil
}
func (m *mockXrayManager) RestartService(ctx context.Context) error {
	return nil
}
func (m *mockXrayManager) GetVersion(ctx context.Context) (string, error) {
	return "v1.0.0", nil
}

func TestTrafficSyncJob_Lifecycle(t *testing.T) {
	mockXray := &mockXrayManager{}
	job := deliveryCron.NewTrafficSyncJob(mockXray, nil, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	job.Start(ctx)

	// Cancel to ensure both decoupled goroutines exit cleanly without hanging
	cancel()
	time.Sleep(50 * time.Millisecond)
}
