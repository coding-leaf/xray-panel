package cron_test

import (
	"context"
	"testing"
	"time"

	deliveryCron "panel/internal/delivery/cron"
	"panel/internal/domain"
)

type mockXrayManager struct {
	queryCount    int
	statsToReturn []domain.TrafficStat
}

func (m *mockXrayManager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	return nil
}
func (m *mockXrayManager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	return nil
}
func (m *mockXrayManager) QueryTrafficStats(ctx context.Context, reset bool) ([]domain.TrafficStat, error) {
	m.queryCount++
	res := m.statsToReturn
	if reset {
		m.statsToReturn = nil
	}
	return res, nil
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

type mockUserRepo struct {
	domain.UserRepository
	addedTrafficEmail string
	addedUp           int64
	addedDown         int64
	ctxHadError       bool
}

func (r *mockUserRepo) AddTraffic(ctx context.Context, email string, upBytes, downBytes int64) error {
	r.addedTrafficEmail = email
	r.addedUp = upBytes
	r.addedDown = downBytes
	if ctx.Err() != nil {
		r.ctxHadError = true
	}
	return nil
}

func (r *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}

func TestTrafficSyncJob_Lifecycle(t *testing.T) {
	mockXray := &mockXrayManager{}
	mockUser := &mockUserRepo{}
	job := deliveryCron.NewTrafficSyncJob(mockXray, mockUser, nil, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- job.Start(ctx)
	}()

	// Brief wait to ensure Start is running
	time.Sleep(20 * time.Millisecond)

	// Configure mock to return traffic stats upon final flush
	mockXray.statsToReturn = []domain.TrafficStat{
		{
			Tag:      "test@example.com",
			Value:    2048,
			IsUplink: true,
			Type:     domain.TrafficStatTypeUser,
		},
	}

	// Cancel context to trigger graceful shutdown and final flush
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on graceful exit, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TrafficSyncJob failed to exit within timeout")
	}

	// Verify that final data flush occurred upon ctx.Done()
	if mockXray.queryCount == 0 {
		t.Errorf("expected final data flush to call QueryTrafficStats at least once, got %d", mockXray.queryCount)
	}

	// Verify that userRepo.AddTraffic was invoked with valid data
	if mockUser.addedTrafficEmail != "test@example.com" || mockUser.addedUp != 2048 {
		t.Errorf("expected traffic to be added for test@example.com with up=2048, got email=%s up=%d",
			mockUser.addedTrafficEmail, mockUser.addedUp)
	}

	// Crucial: verify that the write context was protected from parent cancellation
	if mockUser.ctxHadError {
		t.Errorf("expected write context to be protected from parent context cancellation, but ctx.Err() was set!")
	}
}
