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
	removedCalls  []struct{ Tag, Email string }
}

func (m *mockXrayManager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	return nil
}
func (m *mockXrayManager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	m.removedCalls = append(m.removedCalls, struct{ Tag, Email string }{Tag: inboundTag, Email: email})
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
	addTrafficCalls   int
	addedTrafficEmail string
	addedUp           int64
	addedDown         int64
	ctxHadError       bool
}

func (r *mockUserRepo) AddTraffic(ctx context.Context, email string, upBytes, downBytes int64) error {
	r.addTrafficCalls++
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

func TestTrafficSyncJob_Aggregation(t *testing.T) {
	mockXray := &mockXrayManager{
		statsToReturn: []domain.TrafficStat{
			{
				Tag:      "alice@example.com",
				Value:    1024,
				IsUplink: true,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "alice@example.com",
				Value:    2048,
				IsUplink: false,
				Type:     domain.TrafficStatTypeUser,
			},
		},
	}
	mockUser := &mockUserRepo{}
	job := deliveryCron.NewTrafficSyncJob(mockXray, mockUser, nil, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so Start performs only final flush

	err := job.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that 2 stats (up + down) for the same user resulted in exactly 1 AddTraffic call
	if mockUser.addTrafficCalls != 1 {
		t.Errorf("expected exactly 1 AddTraffic call due to aggregation, got %d", mockUser.addTrafficCalls)
	}
	if mockUser.addedTrafficEmail != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", mockUser.addedTrafficEmail)
	}
	if mockUser.addedUp != 1024 || mockUser.addedDown != 2048 {
		t.Errorf("expected up=1024 down=2048, got up=%d down=%d", mockUser.addedUp, mockUser.addedDown)
	}
}

type mockBatchUserRepo struct {
	domain.UserRepository
	batchCalls     int
	lastUserDeltas []domain.UserTrafficDelta
	lastInDeltas   []domain.InboundTrafficDelta
	usersToReturn  []domain.User
	ctxHadError    bool
}

func (m *mockBatchUserRepo) BatchSyncTraffic(ctx context.Context, userDeltas []domain.UserTrafficDelta, inboundDeltas []domain.InboundTrafficDelta, date string) ([]domain.User, error) {
	m.batchCalls++
	m.lastUserDeltas = userDeltas
	m.lastInDeltas = inboundDeltas
	if ctx.Err() != nil {
		m.ctxHadError = true
	}
	return m.usersToReturn, nil
}

func TestTrafficSyncJob_BatchSync(t *testing.T) {
	mockXray := &mockXrayManager{
		statsToReturn: []domain.TrafficStat{
			{
				Tag:      "user1@example.com",
				Value:    1000,
				IsUplink: true,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "user1@example.com",
				Value:    2000,
				IsUplink: false,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "user2@example.com",
				Value:    3000,
				IsUplink: true,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "inbound-tag-1",
				Value:    5000,
				IsUplink: true,
				Type:     domain.TrafficStatTypeInbound,
			},
		},
	}

	inactiveUser := domain.User{
		Email:       "user2@example.com",
		Enabled:     false,
		InboundTags: "tag-a,tag-b",
	}

	mockBatchRepo := &mockBatchUserRepo{
		usersToReturn: []domain.User{
			{Email: "user1@example.com", Enabled: true},
			inactiveUser,
		},
	}

	job := deliveryCron.NewTrafficSyncJob(mockXray, mockBatchRepo, nil, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // trigger immediate final flush

	err := job.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockBatchRepo.batchCalls != 1 {
		t.Fatalf("expected 1 BatchSyncTraffic call, got %d", mockBatchRepo.batchCalls)
	}
	if mockBatchRepo.ctxHadError {
		t.Errorf("writeCtx should not have error despite parent context cancellation")
	}

	// Verify aggregated deltas
	if len(mockBatchRepo.lastUserDeltas) != 2 {
		t.Fatalf("expected 2 user deltas, got %d", len(mockBatchRepo.lastUserDeltas))
	}
	if len(mockBatchRepo.lastInDeltas) != 1 {
		t.Fatalf("expected 1 inbound delta, got %d", len(mockBatchRepo.lastInDeltas))
	}

	// Check inactive user was removed from all inbound tags
	if len(mockXray.removedCalls) != 2 {
		t.Fatalf("expected 2 RemoveUser calls for inactive user, got %d", len(mockXray.removedCalls))
	}
	expectedTags := map[string]bool{"tag-a": true, "tag-b": true}
	for _, call := range mockXray.removedCalls {
		if call.Email != "user2@example.com" {
			t.Errorf("expected removed email user2@example.com, got %s", call.Email)
		}
		if !expectedTags[call.Tag] {
			t.Errorf("unexpected removed tag %s", call.Tag)
		}
	}
}

type mockInboundRepoForCron struct {
	domain.InboundRepository
	inbounds []domain.Inbound
}

func (m *mockInboundRepoForCron) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}

func TestTrafficSyncJob_BoundUserInboundTrafficAccounting(t *testing.T) {
	mockXray := &mockXrayManager{
		statsToReturn: []domain.TrafficStat{
			{
				Tag:      "local-socks",
				Value:    1048576, // 1MB uplink
				IsUplink: true,
				Type:     domain.TrafficStatTypeInbound,
			},
			{
				Tag:      "local-socks",
				Value:    2097152, // 2MB downlink
				IsUplink: false,
				Type:     domain.TrafficStatTypeInbound,
			},
			{
				Tag:      "unbound-inbound",
				Value:    500000,
				IsUplink: true,
				Type:     domain.TrafficStatTypeInbound,
			},
		},
	}

	mockInboundRepo := &mockInboundRepoForCron{
		inbounds: []domain.Inbound{
			{
				Tag:            "local-socks",
				Port:           10808,
				Protocol:       "socks",
				BoundUserEmail: "local@yezineko.top",
			},
			{
				Tag:            "unbound-inbound",
				Port:           10809,
				Protocol:       "socks",
				BoundUserEmail: "",
			},
		},
	}

	mockBatchRepo := &mockBatchUserRepo{
		usersToReturn: []domain.User{
			{
				Email:   "local@yezineko.top",
				Enabled: true,
			},
		},
	}

	job := deliveryCron.NewTrafficSyncJob(mockXray, mockBatchRepo, mockInboundRepo, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Trigger immediate final flush

	err := job.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockBatchRepo.batchCalls != 1 {
		t.Fatalf("expected 1 BatchSyncTraffic call, got %d", mockBatchRepo.batchCalls)
	}

	// 1. Inbound deltas check: local-socks should have up=1048576, down=2097152; unbound-inbound up=500000
	if len(mockBatchRepo.lastInDeltas) != 2 {
		t.Fatalf("expected 2 inbound deltas, got %d", len(mockBatchRepo.lastInDeltas))
	}
	inMap := make(map[string]domain.InboundTrafficDelta)
	for _, inD := range mockBatchRepo.lastInDeltas {
		inMap[inD.Tag] = inD
	}
	if inMap["local-socks"].Up != 1048576 || inMap["local-socks"].Down != 2097152 {
		t.Errorf("unexpected local-socks delta: %+v", inMap["local-socks"])
	}
	if inMap["unbound-inbound"].Up != 500000 || inMap["unbound-inbound"].Down != 0 {
		t.Errorf("unexpected unbound-inbound delta: %+v", inMap["unbound-inbound"])
	}

	// 2. User deltas check: local@yezineko.top should have received the bound inbound traffic!
	if len(mockBatchRepo.lastUserDeltas) != 1 {
		t.Fatalf("expected 1 user delta for bound user, got %d", len(mockBatchRepo.lastUserDeltas))
	}
	userDelta := mockBatchRepo.lastUserDeltas[0]
	if userDelta.Email != "local@yezineko.top" {
		t.Errorf("expected user delta email local@yezineko.top, got %s", userDelta.Email)
	}
	if userDelta.Up != 1048576 || userDelta.Down != 2097152 {
		t.Errorf("expected user delta up=1048576 down=2097152, got up=%d down=%d", userDelta.Up, userDelta.Down)
	}
}
