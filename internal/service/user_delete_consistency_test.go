package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type mockSupervisorForConsistency struct {
	mu          sync.Mutex
	reloadCalls int
}

func (m *mockSupervisorForConsistency) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCalls++
	return nil
}

func (m *mockSupervisorForConsistency) Restart(ctx context.Context) error {
	return nil
}

type xrayHistoryCall struct {
	op    string
	tag   string
	email string
}

type mockXrayManagerForConsistency struct {
	mu              sync.Mutex
	calls           []xrayHistoryCall
	failAddTag      string
	failRemoveTag   string
	failAddErr      error
	failRemoveErr   error
	canceledCtxSeen bool
}

func (m *mockXrayManagerForConsistency) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		m.canceledCtxSeen = true
	}
	m.calls = append(m.calls, xrayHistoryCall{op: "add", tag: inboundTag, email: user.Email})
	if m.failAddTag != "" && m.failAddTag == inboundTag {
		if m.failAddErr != nil {
			return m.failAddErr
		}
		return errors.New("simulated xray add user error")
	}
	return nil
}

func (m *mockXrayManagerForConsistency) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		m.canceledCtxSeen = true
	}
	m.calls = append(m.calls, xrayHistoryCall{op: "remove", tag: inboundTag, email: email})
	if m.failRemoveTag != "" && m.failRemoveTag == inboundTag {
		if m.failRemoveErr != nil {
			return m.failRemoveErr
		}
		return errors.New("simulated xray remove user error")
	}
	return nil
}

func (m *mockXrayManagerForConsistency) GetCalls() []xrayHistoryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]xrayHistoryCall, len(m.calls))
	copy(copied, m.calls)
	return copied
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.User{},
		&domain.Inbound{},
		&domain.ConfigSnapshot{},
	); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}
	return db
}

// 1. TestUserDeleteConsistency_RestartResurrectionDefense
// 端到端真实环境：创建用户 -> 落盘 -> 删除用户 -> 物理文件与 DB 双清空 -> SyncFromFile 模拟重启 -> 验证幽灵用户绝不复活
func TestUserDeleteConsistency_RestartResurrectionDefense(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	snapshotRepo := repository.NewGormConfigSnapshotRepository(db)

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to init config file: %v", err)
	}

	configMgr := xray.NewConfigManager(cfgPath, "")
	mockSup := &mockSupervisorForConsistency{}
	compiler := xray.NewXrayCompiler(8080)
	configSvc := service.NewConfigService(configMgr, mockSup, inboundRepo, userRepo, snapshotRepo, compiler)

	mockXray := &mockXrayManagerForConsistency{}
	userSvc := service.NewUserService(userRepo, inboundRepo, nil, mockXray, configSvc)

	ctx := context.Background()

	// 创建 Inbound
	inb := &domain.Inbound{
		Tag:          "vless-tcp-node",
		Listen:       "0.0.0.0",
		Port:         4430,
		Protocol:     "vless",
		SettingsJSON: `{"decryption":"none"}`,
		Enabled:      true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := inboundRepo.Create(ctx, inb); err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	// 步骤 1: 创建有效用户
	createdUser, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "alice@example.com",
		InboundTags: []string{"vless-tcp-node"},
		Flow:        "xtls-rprx-vision",
		TotalBytes:  1024 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 步骤 2: 校验数据库与 config.json 均包含该用户
	uInDB, err := userRepo.GetByID(ctx, createdUser.ID)
	if err != nil || uInDB == nil {
		t.Fatalf("expected user in DB, got err: %v", err)
	}

	rawCfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read raw config: %v", err)
	}
	var rootCfg map[string]interface{}
	if err := json.Unmarshal(rawCfg, &rootCfg); err != nil {
		t.Fatalf("failed to unmarshal raw config: %v", err)
	}
	inboundsArr := rootCfg["inbounds"].([]interface{})
	inb0 := inboundsArr[0].(map[string]interface{})
	settings0 := inb0["settings"].(map[string]interface{})
	clients0 := settings0["clients"].([]interface{})
	if len(clients0) != 1 {
		t.Fatalf("expected 1 client in config.json, got %d", len(clients0))
	}
	c0 := clients0[0].(map[string]interface{})
	if c0["email"] != "alice@example.com" {
		t.Fatalf("expected alice@example.com in config.json, got %v", c0["email"])
	}

	// 步骤 3: 调用 DeleteUser 删除用户
	if err := userSvc.DeleteUser(ctx, createdUser.ID); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// 步骤 4: 断言 SQLite 与 config.json 物理文件均已彻底清除该用户
	deletedInDB, err := userRepo.GetByID(ctx, createdUser.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unexpected error querying deleted user: %v", err)
	}
	if deletedInDB != nil {
		t.Fatalf("user still exists in DB after deletion")
	}

	rawCfgAfter, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read raw config after delete: %v", err)
	}
	var rootAfter map[string]interface{}
	if err := json.Unmarshal(rawCfgAfter, &rootAfter); err != nil {
		t.Fatalf("failed to unmarshal raw config after delete: %v", err)
	}
	inboundsAfter := rootAfter["inbounds"].([]interface{})
	inbAfter0 := inboundsAfter[0].(map[string]interface{})
	settingsAfter0 := inbAfter0["settings"].(map[string]interface{})
	clientsAfter0 := settingsAfter0["clients"].([]interface{})
	if len(clientsAfter0) != 0 {
		t.Fatalf("expected 0 clients in config.json after delete, got %d", len(clientsAfter0))
	}

	// 步骤 5: 模拟面板冷重启调用 SyncFromFile
	if err := configSvc.SyncFromFile(ctx); err != nil {
		t.Fatalf("SyncFromFile failed: %v", err)
	}

	// 步骤 6: 断言用户在 SQLite 中绝不重新出现（0 用户，彻底解决幽灵复活）
	allUsers, err := userRepo.ListAll(ctx)
	if err != nil {
		t.Fatalf("userRepo.ListAll failed: %v", err)
	}
	if len(allUsers) != 0 {
		t.Fatalf("ghost user resurrected! expected 0 users in DB after SyncFromFile, got %d: %+v", len(allUsers), allUsers)
	}
}

type faultyConfigSynchronizer struct {
	failSyncUserToFile error
	syncCalls          int
	mu                 sync.Mutex
}

func (f *faultyConfigSynchronizer) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncCalls++
	if f.failSyncUserToFile != nil {
		return f.failSyncUserToFile
	}
	return nil
}

func (f *faultyConfigSynchronizer) SaveConfigQuietly(ctx context.Context, remark string) error {
	return nil
}

// 2. TestUserDeleteConsistency_SagaCompensateOnDiskFailure
// 单测验证当删除过程中写盘失败时，SQLite 数据库用户被成功恢复，且 Xray 已移除的节点被成功重新注入
func TestUserDeleteConsistency_SagaCompensateOnDiskFailure(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)

	ctx := context.Background()

	inb := &domain.Inbound{
		Tag:      "trojan-in",
		Listen:   "0.0.0.0",
		Port:     4431,
		Protocol: "trojan",
		Enabled:  true,
	}
	if err := inboundRepo.Create(ctx, inb); err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	user := &domain.User{
		ID:          42,
		UUID:        "uuid-trojan-42",
		Email:       "bob@example.com",
		InboundTag:  "trojan-in",
		InboundTags: "trojan-in",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to insert initial user: %v", err)
	}

	mockXray := &mockXrayManagerForConsistency{}
	faultyCfg := &faultyConfigSynchronizer{
		failSyncUserToFile: errors.New("simulated disk full error"),
	}
	userSvc := service.NewUserService(userRepo, inboundRepo, nil, mockXray, faultyCfg)

	err := userSvc.DeleteUser(ctx, 42)
	if err == nil {
		t.Fatalf("expected DeleteUser to fail when disk sync fails")
	}

	// 1. 验证 SQLite 数据库用户被成功恢复
	restoredUser, err := userRepo.GetByID(ctx, 42)
	if err != nil || restoredUser == nil {
		t.Fatalf("expected user 42 to be restored in DB after Saga rollback, got err: %v", err)
	}
	if restoredUser.Email != "bob@example.com" {
		t.Fatalf("expected restored user email bob@example.com, got %s", restoredUser.Email)
	}

	// 2. 验证 Xray 节点被重新 AddUser 恢复
	calls := mockXray.GetCalls()
	foundRemove := false
	foundCompAdd := false
	for _, c := range calls {
		if c.op == "remove" && c.tag == "trojan-in" && c.email == "bob@example.com" {
			foundRemove = true
		}
		if c.op == "add" && c.tag == "trojan-in" && c.email == "bob@example.com" {
			foundCompAdd = true
		}
	}
	if !foundRemove || !foundCompAdd {
		t.Fatalf("expected remove followed by compensatory add on Xray, got calls: %+v", calls)
	}
}

// 3. TestUserDeleteConsistency_ContextCanceledResilience
// 验证在 ctx 被提前 cancel 的极端场景下，Saga 补偿逻辑依旧能凭借独立 Context 成功执行清理与回滚
func TestUserDeleteConsistency_ContextCanceledResilience(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)

	ctx := context.Background()

	inb := &domain.Inbound{
		Tag:      "vless-cancel-test",
		Listen:   "0.0.0.0",
		Port:     4432,
		Protocol: "vless",
		Enabled:  true,
	}
	if err := inboundRepo.Create(ctx, inb); err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	user := &domain.User{
		ID:          99,
		UUID:        "uuid-cancel-99",
		Email:       "charlie@example.com",
		InboundTag:  "vless-cancel-test",
		InboundTags: "vless-cancel-test",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to insert initial user: %v", err)
	}

	var capturedCompCtx context.Context
	var capturedCompCtxErr error
	mockXray := &mockXrayManagerForConsistency{}
	// 包装 mockXray 以检查补偿发生时传入的 Context 是否没有被父 context 的 Canceled 传染
	wrappedXray := &compensatingContextInspectorXray{
		mock: mockXray,
		onAdd: func(ctx context.Context) {
			capturedCompCtx = ctx
			capturedCompCtxErr = ctx.Err()
		},
	}

	// 创建可取消的 context，在进入 SyncUserToFile 时主动 cancel 模拟网络/请求超时中断
	testCtx, cancel := context.WithCancel(context.Background())
	faultyCfg := &faultyConfigSynchronizer{
		failSyncUserToFile: errors.New("simulated sync error"),
	}
	// 在 SyncUserToFile 调用前把 context cancel 掉
	wrappedCfg := &cancelOnSyncConfigSynchronizer{
		faulty: faultyCfg,
		cancel: cancel,
	}

	userSvc := service.NewUserService(userRepo, inboundRepo, nil, wrappedXray, wrappedCfg)

	err := userSvc.DeleteUser(testCtx, 99)
	t.Logf("err returned by DeleteUser: %v", err)
	if err == nil {
		t.Fatalf("expected DeleteUser to fail")
	}

	// 校验 DB 恢复成功
	restored, err := userRepo.GetByID(context.Background(), 99)
	if err != nil || restored == nil {
		t.Fatalf("expected user 99 to be restored despite canceled parent ctx, err: %v", err)
	}

	// 校验补偿时的 context 没有处于 canceled 状态（证明使用了 context.WithoutCancel）
	if capturedCompCtx == nil {
		t.Fatalf("expected compensatory AddUser to be executed")
	}
	if capturedCompCtxErr != nil {
		t.Fatalf("expected compensating context to be alive (no error), but got: %v", capturedCompCtxErr)
	}
}

type cancelOnSyncConfigSynchronizer struct {
	faulty *faultyConfigSynchronizer
	cancel context.CancelFunc
}

func (c *cancelOnSyncConfigSynchronizer) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	if c.cancel != nil {
		c.cancel()
	}
	return c.faulty.SyncUserToFile(ctx, authorizedTags, user, isDelete)
}

func (c *cancelOnSyncConfigSynchronizer) SaveConfigQuietly(ctx context.Context, remark string) error {
	return nil
}

type compensatingContextInspectorXray struct {
	mock  *mockXrayManagerForConsistency
	onAdd func(ctx context.Context)
}

func (c *compensatingContextInspectorXray) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	if c.onAdd != nil {
		c.onAdd(ctx)
	}
	return c.mock.AddUser(ctx, inboundTag, user)
}

func (c *compensatingContextInspectorXray) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	return c.mock.RemoveUser(ctx, inboundTag, email)
}

// 4. TestUserDeleteConsistency_PartialNodeFailureRollback
// 多节点移除部分失败时，已移除节点被回滚恢复
func TestUserDeleteConsistency_PartialNodeFailureRollback(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)

	ctx := context.Background()

	user := &domain.User{
		ID:          55,
		UUID:        "uuid-55",
		Email:       "multi_node@example.com",
		InboundTag:  "node-1",
		InboundTags: "node-1,node-2,node-3",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to insert multi-node user: %v", err)
	}

	mockXray := &mockXrayManagerForConsistency{
		failRemoveTag: "node-2", // node-1 成功，node-2 失败
		failRemoveErr: errors.New("simulated node-2 connection refused"),
	}
	userSvc := service.NewUserService(userRepo, inboundRepo, nil, mockXray, nil)

	err := userSvc.DeleteUser(ctx, 55)
	if err == nil {
		t.Fatalf("expected DeleteUser to fail on node-2")
	}

	// 检查数据库记录依然完好保留（未被误删）
	uInDB, err := userRepo.GetByID(ctx, 55)
	if err != nil || uInDB == nil {
		t.Fatalf("expected user 55 to remain in DB after partial node failure")
	}

	// 检查 node-1 是否触发了反向 AddUser 恢复
	calls := mockXray.GetCalls()
	foundNode1Restored := false
	for _, c := range calls {
		if c.op == "add" && c.tag == "node-1" && c.email == "multi_node@example.com" {
			foundNode1Restored = true
		}
	}
	if !foundNode1Restored {
		t.Fatalf("expected node-1 to be rolled back via AddUser, got calls: %+v", calls)
	}
}
