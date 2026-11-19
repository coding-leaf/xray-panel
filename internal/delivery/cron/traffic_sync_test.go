package cron_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/repository"
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

func TestTrafficSyncJob_LegacyBoundUserNotCopied(t *testing.T) {
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
		},
	}

	mockInboundRepo := &mockInboundRepoForCron{
		inbounds: []domain.Inbound{
			{
				Tag:            "local-socks",
				Port:           10808,
				Protocol:       "socks",
				BoundUserEmail: "legacy@example.com",
			},
		},
	}

	mockBatchRepo := &mockBatchUserRepo{
		usersToReturn: []domain.User{
			{
				Email:   "legacy@example.com",
				Enabled: true,
			},
		},
	}

	job := deliveryCron.NewTrafficSyncJob(mockXray, mockBatchRepo, mockInboundRepo, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := job.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockBatchRepo.batchCalls != 1 {
		t.Fatalf("expected 1 BatchSyncTraffic call, got %d", mockBatchRepo.batchCalls)
	}

	// 1. Inbound deltas check: local-socks should have up=1048576, down=2097152
	if len(mockBatchRepo.lastInDeltas) != 1 {
		t.Fatalf("expected 1 inbound delta, got %d", len(mockBatchRepo.lastInDeltas))
	}
	if mockBatchRepo.lastInDeltas[0].Tag != "local-socks" || mockBatchRepo.lastInDeltas[0].Up != 1048576 || mockBatchRepo.lastInDeltas[0].Down != 2097152 {
		t.Errorf("unexpected local-socks delta: %+v", mockBatchRepo.lastInDeltas[0])
	}

	// 2. User deltas check: legacy BoundUserEmail must NOT be copied into user deltas!
	if len(mockBatchRepo.lastUserDeltas) != 0 {
		t.Fatalf("expected 0 user deltas (legacy bound user must not be copied), got %d: %+v", len(mockBatchRepo.lastUserDeltas), mockBatchRepo.lastUserDeltas)
	}
}

func TestTrafficSyncJob_NativeUserAndInboundNoDoubleCounting(t *testing.T) {
	mockXray := &mockXrayManager{
		statsToReturn: []domain.TrafficStat{
			{
				Tag:      "alice@example.com",
				Value:    100,
				IsUplink: true,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "alice@example.com",
				Value:    200,
				IsUplink: false,
				Type:     domain.TrafficStatTypeUser,
			},
			{
				Tag:      "local-socks",
				Value:    1000,
				IsUplink: true,
				Type:     domain.TrafficStatTypeInbound,
			},
			{
				Tag:      "local-socks",
				Value:    2000,
				IsUplink: false,
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
				BoundUserEmail: "alice@example.com",
			},
		},
	}

	mockBatchRepo := &mockBatchUserRepo{
		usersToReturn: []domain.User{
			{
				Email:   "alice@example.com",
				Enabled: true,
			},
		},
	}

	job := deliveryCron.NewTrafficSyncJob(mockXray, mockBatchRepo, mockInboundRepo, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := job.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User deltas check: alice should ONLY receive native stats (100, 200), NOT double-counted with inbound
	if len(mockBatchRepo.lastUserDeltas) != 1 {
		t.Fatalf("expected 1 user delta, got %d", len(mockBatchRepo.lastUserDeltas))
	}
	ud := mockBatchRepo.lastUserDeltas[0]
	if ud.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %s", ud.Email)
	}
	if ud.Up != 100 || ud.Down != 200 {
		t.Errorf("expected native stats only (up=100, down=200), got up=%d, down=%d (double counted!)", ud.Up, ud.Down)
	}

	// Inbound delta check
	if len(mockBatchRepo.lastInDeltas) != 1 {
		t.Fatalf("expected 1 inbound delta, got %d", len(mockBatchRepo.lastInDeltas))
	}
	if mockBatchRepo.lastInDeltas[0].Tag != "local-socks" || mockBatchRepo.lastInDeltas[0].Up != 1000 || mockBatchRepo.lastInDeltas[0].Down != 2000 {
		t.Errorf("unexpected inbound delta: %+v", mockBatchRepo.lastInDeltas[0])
	}
}

type mockTrafficLogRepoForCron struct {
	pruneCalls     int
	lastBeforeDate string
}

func (m *mockTrafficLogRepoForCron) RecordTraffic(ctx context.Context, userID uint, email string, upBytes, downBytes int64, date string) error {
	return nil
}
func (m *mockTrafficLogRepoForCron) GetHistoryByUserID(ctx context.Context, userID uint, days int) ([]domain.TrafficLog, error) {
	return nil, nil
}
func (m *mockTrafficLogRepoForCron) GetHistoryByEmail(ctx context.Context, email string, days int) ([]domain.TrafficLog, error) {
	return nil, nil
}
func (m *mockTrafficLogRepoForCron) DeleteByUser(ctx context.Context, userID uint, email string) error {
	return nil
}
func (m *mockTrafficLogRepoForCron) PruneBefore(ctx context.Context, beforeDate string) error {
	m.pruneCalls++
	m.lastBeforeDate = beforeDate
	return nil
}

func TestTrafficSyncJob_PrunesOldTrafficLogsOncePerDay(t *testing.T) {
	mockXray := &mockXrayManager{}
	mockTrafficLog := &mockTrafficLogRepoForCron{}
	job := deliveryCron.NewTrafficSyncJob(mockXray, nil, nil, mockTrafficLog, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := job.Start(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCutoff := time.Now().AddDate(0, 0, -180).Format("2006-01-02")
	if mockTrafficLog.pruneCalls != 1 {
		t.Fatalf("expected 1 PruneBefore call on first sync, got %d", mockTrafficLog.pruneCalls)
	}
	if mockTrafficLog.lastBeforeDate != expectedCutoff {
		t.Fatalf("expected cutoff date %s, got %s", expectedCutoff, mockTrafficLog.lastBeforeDate)
	}

	// Second run on the same calendar day should not prune again
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if err := job.Start(ctx2); err != nil {
		t.Fatalf("unexpected error on second start: %v", err)
	}
	if mockTrafficLog.pruneCalls != 1 {
		t.Fatalf("expected PruneBefore to remain 1 call on same day, got %d", mockTrafficLog.pruneCalls)
	}
}

type failingXrayAndRepo struct {
	domain.UserRepository
	removeCalls int
	addCalls    int
}

func (f *failingXrayAndRepo) AddTraffic(ctx context.Context, email string, upBytes, downBytes int64) error {
	f.addCalls++
	return errors.New("db write error")
}

func (f *failingXrayAndRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return &domain.User{ID: 1, Email: email, Enabled: false, InboundTags: "vless-in"}, nil
}

type failingXrayMgr struct {
	mockXrayManager
	removeErr error
}

func (m *failingXrayMgr) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	m.removedCalls = append(m.removedCalls, struct{ Tag, Email string }{Tag: inboundTag, Email: email})
	return m.removeErr
}

func TestTrafficSyncJob_HandlesRepoAndXrayErrorsGracefully(t *testing.T) {
	mockXray := &failingXrayMgr{
		mockXrayManager: mockXrayManager{
			statsToReturn: []domain.TrafficStat{
				{Type: domain.TrafficStatTypeUser, Tag: "disabled@example.com", IsUplink: true, Value: 512},
			},
		},
		removeErr: errors.New("grpc unavailable"),
	}
	repo := &failingXrayAndRepo{}
	job := deliveryCron.NewTrafficSyncJob(mockXray, repo, nil, nil, nil, nil, nil, 3*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := job.Start(ctx); err != nil {
		t.Fatalf("expected graceful exit even on repo/xray errors, got: %v", err)
	}
	if repo.addCalls != 1 {
		t.Fatalf("expected 1 AddTraffic attempt, got %d", repo.addCalls)
	}
	if len(mockXray.removedCalls) != 1 {
		t.Fatalf("expected 1 RemoveUser attempt for disabled user, got %d", len(mockXray.removedCalls))
	}
}

type mockNodeBatchRepoForCron struct {
	mu           sync.Mutex
	pruneCalls   int
	lastBefore   time.Time
	errToReturn  error
}

func (m *mockNodeBatchRepoForCron) ApplyNodeTrafficBatch(ctx context.Context, nodeID uint, batch *domain.AgentTrafficBatch, date string) (bool, []domain.User, error) {
	return false, nil, nil
}

func (m *mockNodeBatchRepoForCron) PruneBatchesBefore(ctx context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneCalls++
	m.lastBefore = before
	return m.errToReturn
}

func TestTrafficSyncJob_PrunesOldNodeTrafficBatchesWithoutXrayManager(t *testing.T) {
	mockBatch := &mockNodeBatchRepoForCron{
		errToReturn: errors.New("transient sqlite busy"),
	}
	// xrayManager is intentionally nil to verify remote-only control plane still prunes node_traffic_batches.
	job := deliveryCron.NewTrafficSyncJob(nil, nil, nil, nil, nil, nil, nil, 3*time.Second).
		WithNodeBatchRepo(mockBatch)

	// 1. First run fails -> must not mark today as pruned so subsequent attempt retries.
	ctx1, cancel1 := context.WithCancel(context.Background())
	cancel1()
	if err := job.Start(ctx1); err != nil {
		t.Fatalf("unexpected error on start #1: %v", err)
	}
	mockBatch.mu.Lock()
	callsAfterErr := mockBatch.pruneCalls
	mockBatch.errToReturn = nil
	mockBatch.mu.Unlock()
	if callsAfterErr != 1 {
		t.Fatalf("expected 1 PruneBatchesBefore call on start #1, got %d", callsAfterErr)
	}

	// 2. Second run succeeds -> records today's prune and passes ~7-day cutoff.
	beforeStart := time.Now().AddDate(0, 0, -7).Add(-5 * time.Second)
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if err := job.Start(ctx2); err != nil {
		t.Fatalf("unexpected error on start #2: %v", err)
	}
	afterStart := time.Now().AddDate(0, 0, -7).Add(5 * time.Second)

	mockBatch.mu.Lock()
	callsAfterSuccess := mockBatch.pruneCalls
	cutoff := mockBatch.lastBefore
	mockBatch.mu.Unlock()
	if callsAfterSuccess != 2 {
		t.Fatalf("expected retry to bring PruneBatchesBefore count to 2, got %d", callsAfterSuccess)
	}
	if cutoff.Before(beforeStart) || cutoff.After(afterStart) {
		t.Fatalf("expected 7-day cutoff around [%v, %v], got %v", beforeStart, afterStart, cutoff)
	}

	// 3. Third run on the same calendar day -> deduplicated, must remain 2.
	ctx3, cancel3 := context.WithCancel(context.Background())
	cancel3()
	if err := job.Start(ctx3); err != nil {
		t.Fatalf("unexpected error on start #3: %v", err)
	}
	mockBatch.mu.Lock()
	callsAfterDedup := mockBatch.pruneCalls
	mockBatch.mu.Unlock()
	if callsAfterDedup != 2 {
		t.Fatalf("expected same-day dedup to keep PruneBatchesBefore count at 2, got %d", callsAfterDedup)
	}

	// 4. SQLite integration check: 8-day-old batch is pruned while 2-day-old batch is preserved.
	dbPath := filepath.Join(t.TempDir(), "cron_node_batch_prune.db")
	db, err := repository.InitSQLite(dbPath)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	nodeRepo := repository.NewNodeRepository(db)
	now := time.Now()
	if err := db.Create(&domain.NodeTrafficBatch{NodeID: 1, BatchID: "old-batch", CreatedAt: now.AddDate(0, 0, -8)}).Error; err != nil {
		t.Fatalf("seed old batch failed: %v", err)
	}
	if err := db.Create(&domain.NodeTrafficBatch{NodeID: 1, BatchID: "recent-batch", CreatedAt: now.AddDate(0, 0, -2)}).Error; err != nil {
		t.Fatalf("seed recent batch failed: %v", err)
	}

	realJob := deliveryCron.NewTrafficSyncJob(nil, nil, nil, nil, nil, nil, nil, 3*time.Second).
		WithNodeBatchRepo(nodeRepo)
	ctx4, cancel4 := context.WithCancel(context.Background())
	cancel4()
	if err := realJob.Start(ctx4); err != nil {
		t.Fatalf("realJob.Start failed: %v", err)
	}

	var remaining []domain.NodeTrafficBatch
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("query remaining batches failed: %v", err)
	}
	if len(remaining) != 1 || remaining[0].BatchID != "recent-batch" {
		t.Fatalf("expected only recent-batch to survive 7-day prune, got %+v", remaining)
	}
}
