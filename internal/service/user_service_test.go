package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
)

type mockUserRepo struct {
	users []domain.User
}

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error {
	if user.ID == 0 {
		var maxID uint = 0
		for _, u := range m.users {
			if u.ID > maxID {
				maxID = u.ID
			}
		}
		user.ID = maxID + 1
	}
	m.users = append(m.users, *user)
	return nil
}
func (m *mockUserRepo) Update(ctx context.Context, user *domain.User) error {
	for i := range m.users {
		if m.users[i].ID == user.ID {
			m.users[i] = *user
			return nil
		}
	}
	return nil
}
func (m *mockUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	for i := range m.users {
		if m.users[i].ID == id {
			if v, ok := values["flow"].(string); ok {
				m.users[i].Flow = v
			}
			if v, ok := values["total_bytes"].(int64); ok {
				m.users[i].TotalBytes = v
			}
			if v, ok := values["expire_time"].(int64); ok {
				m.users[i].ExpireTime = v
			}
			if v, ok := values["reset_day"].(int); ok {
				m.users[i].ResetDay = v
			}
			if v, ok := values["ip_limit"].(int); ok {
				m.users[i].IPLimit = v
			}
			if v, ok := values["enabled"].(bool); ok {
				m.users[i].Enabled = v
			}
			if v, ok := values["inbound_tag"].(string); ok {
				m.users[i].InboundTag = v
			}
			if v, ok := values["inbound_tags"].(string); ok {
				m.users[i].InboundTags = v
			}
			return nil
		}
	}
	return domain.ErrNotFound
}
func (m *mockUserRepo) Delete(ctx context.Context, id uint) error {
	for i := range m.users {
		if m.users[i].ID == id {
			m.users = append(m.users[:i], m.users[i+1:]...)
			return nil
		}
	}
	return nil
}
func (m *mockUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			cp := u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	for _, u := range m.users {
		if u.UUID == uuid {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	for _, u := range m.users {
		if u.SubToken == token {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (m *mockUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return m.users, nil }
func (m *mockUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (m *mockUserRepo) ResetTraffic(ctx context.Context, id uint) error {
	for i := range m.users {
		if m.users[i].ID == id {
			m.users[i].UpBytes = 0
			m.users[i].DownBytes = 0
		}
	}
	return nil
}

func TestCheckAndResetMonthlyTraffic(t *testing.T) {
	now := time.Now()
	today := now.Day()
	nowMonth := now.Year()*100 + int(now.Month())
	lastMonth := 202601
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()

	repo := &mockUserRepo{
		users: []domain.User{
			{ID: 1, Email: "user1@test.com", ResetDay: today, LastResetMonth: lastMonth, UpBytes: 1024, DownBytes: 2048},
			{ID: 2, Email: "user2@test.com", ResetDay: today, LastResetMonth: nowMonth, UpBytes: 5000, DownBytes: 5000}, // 本月已重置过，不能再重置
			{ID: 3, Email: "user3@test.com", ResetDay: 1, LastResetMonth: lastMonth, UpBytes: 3000, DownBytes: 3000},    // 停机补偿：重置日为 1 号但未重置，应补偿重置
		},
	}

	// 如果今天是月末最后一天，添加一个 ResetDay=31 的用户测试短月自适应
	if today == lastDayOfMonth {
		repo.users = append(repo.users, domain.User{
			ID:             4,
			Email:          "user4@test.com",
			ResetDay:       31,
			LastResetMonth: lastMonth,
			UpBytes:        8000,
			DownBytes:      8000,
		})
	}

	svc := NewUserService(repo, nil, nil, nil, nil)
	if err := svc.CheckAndResetMonthlyTraffic(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u1, _ := repo.GetByID(context.Background(), 1)
	if u1.UpBytes != 0 || u1.DownBytes != 0 || u1.LastResetMonth != nowMonth {
		t.Errorf("user1 should be reset and updated with current month, got %+v", u1)
	}

	u2, _ := repo.GetByID(context.Background(), 2)
	if u2.UpBytes != 5000 || u2.DownBytes != 5000 {
		t.Errorf("user2 should NOT be reset again, got %+v", u2)
	}

	u3, _ := repo.GetByID(context.Background(), 3)
	if u3.UpBytes != 0 || u3.DownBytes != 0 || u3.LastResetMonth != nowMonth {
		t.Errorf("user3 (catch-up) should be reset, got %+v", u3)
	}

	if today == lastDayOfMonth {
		u4, _ := repo.GetByID(context.Background(), 4)
		if u4.UpBytes != 0 || u4.DownBytes != 0 || u4.LastResetMonth != nowMonth {
			t.Errorf("user4 (short-month 31st clamped) should be reset on last day of month, got %+v", u4)
		}
	}
}

func TestUserService_UpdateUser_PreservesTrafficCounters(t *testing.T) {
	origTime := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	repo := &mockUserRepo{
		users: []domain.User{
			{
				ID:          1,
				Email:       "user@test.com",
				UUID:        "uuid-orig",
				SubToken:    "token-orig",
				InboundTag:  "vless-in",
				InboundTags: "vless-in",
				Flow:        "xtls-rprx-vision",
				TotalBytes:  10000000,
				UpBytes:     1234567,
				DownBytes:   7654321,
				ExpireTime:  1000,
				ResetDay:    1,
				IPLimit:     2,
				Enabled:     true,
				CreatedAt:   origTime,
			},
		},
	}

	svc := NewUserService(repo, nil, nil, nil, nil)
	enabled := false
	dto := domain.UpdateUserDTO{
		InboundTags: []string{"vless-in", "trojan-in"},
		InboundTag:  "vless-in",
		Flow:        "",
		TotalBytes:  50000000,
		ExpireTime:  2000,
		ResetDay:    15,
		IPLimit:     5,
		Enabled:     &enabled,
	}

	updated, err := svc.UpdateUser(context.Background(), 1, dto)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify editable fields were updated
	if updated.Flow != "" || updated.TotalBytes != 50000000 || updated.ExpireTime != 2000 ||
		updated.ResetDay != 15 || updated.IPLimit != 5 || updated.Enabled != false ||
		updated.InboundTags != "vless-in,trojan-in" {
		t.Errorf("editable fields not updated properly: %+v", updated)
	}

	// Verify non-editable/system fields and traffic counters are strictly preserved
	if updated.UpBytes != 1234567 || updated.DownBytes != 7654321 {
		t.Errorf("traffic counters overwritten! UpBytes: %d, DownBytes: %d", updated.UpBytes, updated.DownBytes)
	}
	if updated.UUID != "uuid-orig" || updated.SubToken != "token-orig" || updated.Email != "user@test.com" {
		t.Errorf("system identity fields overwritten! UUID: %s, SubToken: %s, Email: %s", updated.UUID, updated.SubToken, updated.Email)
	}
	if !updated.CreatedAt.Equal(origTime) {
		t.Errorf("CreatedAt was modified: %v != %v", updated.CreatedAt, origTime)
	}

	// Check DB persistence directly
	inRepo, _ := repo.GetByID(context.Background(), 1)
	if inRepo.UpBytes != 1234567 || inRepo.DownBytes != 7654321 {
		t.Errorf("traffic counters in repo overwritten! UpBytes: %d, DownBytes: %d", inRepo.UpBytes, inRepo.DownBytes)
	}
}

func TestUpdateUserDTO_UnmarshalJSON(t *testing.T) {
	// Case 1: inboundTags as JSON array
	jsonArray := []byte(`{"inboundTags":["tag1","tag2"],"flow":"flow1","enabled":false}`)
	var dto1 domain.UpdateUserDTO
	if err := json.Unmarshal(jsonArray, &dto1); err != nil {
		t.Fatalf("unmarshal array failed: %v", err)
	}
	if len(dto1.InboundTags) != 2 || dto1.InboundTags[0] != "tag1" || dto1.InboundTags[1] != "tag2" {
		t.Errorf("unexpected InboundTags: %v", dto1.InboundTags)
	}
	if dto1.Enabled == nil || *dto1.Enabled != false {
		t.Errorf("expected Enabled=false, got %v", dto1.Enabled)
	}

	// Case 2: inboundTags as comma-separated string (backward compatibility)
	jsonString := []byte(`{"inboundTags":"tag1, tag2 , tag3","enabled":true}`)
	var dto2 domain.UpdateUserDTO
	if err := json.Unmarshal(jsonString, &dto2); err != nil {
		t.Fatalf("unmarshal string failed: %v", err)
	}
	if len(dto2.InboundTags) != 3 || dto2.InboundTags[0] != "tag1" || dto2.InboundTags[1] != "tag2" || dto2.InboundTags[2] != "tag3" {
		t.Errorf("unexpected InboundTags from string: %v", dto2.InboundTags)
	}
	if dto2.Enabled == nil || *dto2.Enabled != true {
		t.Errorf("expected Enabled=true, got %v", dto2.Enabled)
	}

	// Case 3: enabled omitted
	jsonOmitted := []byte(`{"flow":"xtls"}`)
	var dto3 domain.UpdateUserDTO
	if err := json.Unmarshal(jsonOmitted, &dto3); err != nil {
		t.Fatalf("unmarshal omitted failed: %v", err)
	}
	if dto3.Enabled != nil {
		t.Errorf("expected Enabled=nil when omitted, got %v", *dto3.Enabled)
	}
}

func TestUserService_UpdateUser_NotFound(t *testing.T) {
	repo := &mockUserRepo{users: []domain.User{}}
	svc := NewUserService(repo, nil, nil, nil, nil)
	_, err := svc.UpdateUser(context.Background(), 999, domain.UpdateUserDTO{})
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
}

func TestUserService_BatchRenew_PreservesTrafficCounters(t *testing.T) {
	repo := &mockUserRepo{
		users: []domain.User{
			{
				ID:          10,
				Email:       "renew@test.com",
				UpBytes:     888888,
				DownBytes:   999999,
				ExpireTime:  1000,
				Enabled:     true,
				InboundTags: "vless-in",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil, nil)
	if err := svc.BatchRenew(context.Background(), []uint{10}, 30); err != nil {
		t.Fatalf("BatchRenew failed: %v", err)
	}
	u, _ := repo.GetByID(context.Background(), 10)
	if u.UpBytes != 888888 || u.DownBytes != 999999 {
		t.Errorf("BatchRenew overwrote traffic! UpBytes: %d, DownBytes: %d", u.UpBytes, u.DownBytes)
	}
}

func TestUserService_BatchSetStatus_PreservesTrafficCounters(t *testing.T) {
	repo := &mockUserRepo{
		users: []domain.User{
			{
				ID:          20,
				Email:       "status@test.com",
				UpBytes:     111111,
				DownBytes:   222222,
				Enabled:     true,
				InboundTags: "vless-in",
			},
		},
	}
	svc := NewUserService(repo, nil, nil, nil, nil)
	if err := svc.BatchSetStatus(context.Background(), []uint{20}, false); err != nil {
		t.Fatalf("BatchSetStatus failed: %v", err)
	}
	u, _ := repo.GetByID(context.Background(), 20)
	if u.Enabled != false {
		t.Errorf("expected Enabled=false, got %v", u.Enabled)
	}
	if u.UpBytes != 111111 || u.DownBytes != 222222 {
		t.Errorf("BatchSetStatus overwrote traffic! UpBytes: %d, DownBytes: %d", u.UpBytes, u.DownBytes)
	}
}

type mockSupervisor struct {
	reloadCalls  int
	restartCalls int
	mu           sync.Mutex
}

func (m *mockSupervisor) Reload(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCalls++
	return nil
}

func (m *mockSupervisor) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restartCalls++
	return nil
}

type xrayCall struct {
	op    string // "add" or "remove"
	tag   string
	email string
}

type mockXrayManager struct {
	calls             []xrayCall
	failAddUserTag    string
	failAddUserErr    error
	failRemoveUserErr error
	mu                sync.Mutex
}

func (m *mockXrayManager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, xrayCall{op: "add", tag: inboundTag, email: user.Email})
	if m.failAddUserTag == "" && m.failAddUserErr != nil {
		return m.failAddUserErr
	}
	if m.failAddUserTag != "" && m.failAddUserTag == inboundTag {
		if m.failAddUserErr != nil {
			return m.failAddUserErr
		}
		return errors.New("simulated xray add user error")
	}
	return nil
}

func (m *mockXrayManager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, xrayCall{op: "remove", tag: inboundTag, email: email})
	if m.failRemoveUserErr != nil {
		return m.failRemoveUserErr
	}
	return nil
}

type mockConfigSynchronizer struct {
	failSyncUserToFile error
	failSaveConfig     error
	syncCalls          int
	saveCalls          int
	mu                 sync.Mutex
}

func (m *mockConfigSynchronizer) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncCalls++
	if m.failSyncUserToFile != nil {
		return m.failSyncUserToFile
	}
	return nil
}

func (m *mockConfigSynchronizer) SaveConfigQuietly(ctx context.Context, remark string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCalls++
	if m.failSaveConfig != nil {
		return m.failSaveConfig
	}
	return nil
}

func (m *mockXrayManager) GetCalls() []xrayCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]xrayCall, len(m.calls))
	copy(res, m.calls)
	return res
}

type mockInboundRepo struct {
	inbounds []domain.Inbound
}

func (m *mockInboundRepo) Create(ctx context.Context, in *domain.Inbound) error { return nil }
func (m *mockInboundRepo) Update(ctx context.Context, in *domain.Inbound) error { return nil }
func (m *mockInboundRepo) Delete(ctx context.Context, id uint) error            { return nil }
func (m *mockInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	for _, in := range m.inbounds {
		if in.ID == id {
			return &in, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	for _, in := range m.inbounds {
		if in.Tag == tag {
			return &in, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockInboundRepo) ListEnabled(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockInboundRepo) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return nil
}

func TestUserService_ZeroDowntimeAndAutoRestore(t *testing.T) {
	t.Run("ResetTraffic restores over-quota user to Xray and syncs quietly", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          1,
					Email:       "overquota@test.com",
					TotalBytes:  1000,
					UpBytes:     600,
					DownBytes:   500, // 1100 >= 1000 -> IsActive() is false
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in,trojan-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.json")
		_ = os.WriteFile(cfgPath, []byte("{}"), 0644)
		configMgr := xray.NewConfigManager(cfgPath, "")
		mockSup := &mockSupervisor{}
		configSvc := NewConfigService(configMgr, mockSup, nil, repo, nil, xray.NewXrayCompiler())

		svc := NewUserService(repo, nil, nil, mockXray, configSvc)

		// Before reset, user is inactive
		uBefore, _ := repo.GetByID(context.Background(), 1)
		if uBefore.IsActive() {
			t.Fatalf("expected user to be inactive before reset")
		}

		// Reset traffic
		if err := svc.ResetTraffic(context.Background(), 1); err != nil {
			t.Fatalf("ResetTraffic failed: %v", err)
		}

		uAfter, _ := repo.GetByID(context.Background(), 1)
		if !uAfter.IsActive() {
			t.Fatalf("expected user to be active after reset")
		}

		calls := mockXray.GetCalls()
		addedTags := make(map[string]bool)
		for _, c := range calls {
			if c.op == "add" && c.email == "overquota@test.com" {
				addedTags[c.tag] = true
			}
		}

		if !addedTags["vless-in"] || !addedTags["trojan-in"] {
			t.Fatalf("expected user to be added back to vless-in and trojan-in, got calls: %+v", calls)
		}

		if mockSup.reloadCalls != 0 {
			t.Fatalf("expected 0 supervisor reload calls, got %d", mockSup.reloadCalls)
		}
	})

	t.Run("BatchResetTraffic restores active users to Xray", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          1,
					Email:       "u1@test.com",
					TotalBytes:  1000,
					UpBytes:     1000,
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
				},
				{
					ID:          2,
					Email:       "u2@test.com",
					TotalBytes:  2000,
					DownBytes:   2500,
					Enabled:     true,
					InboundTag:  "trojan-in",
					InboundTags: "trojan-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		svc := NewUserService(repo, nil, nil, mockXray, nil)

		if err := svc.BatchResetTraffic(context.Background(), []uint{1, 2}); err != nil {
			t.Fatalf("BatchResetTraffic failed: %v", err)
		}

		calls := mockXray.GetCalls()
		addedUsers := make(map[string]bool)
		for _, c := range calls {
			if c.op == "add" {
				addedUsers[c.email] = true
			}
		}

		if !addedUsers["u1@test.com"] || !addedUsers["u2@test.com"] {
			t.Fatalf("expected u1 and u2 to be added to xray, got calls: %+v", calls)
		}
	})

	t.Run("BatchRenew restores expired users to Xray", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          3,
					Email:       "expired@test.com",
					ExpireTime:  1000, // expired long ago
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		svc := NewUserService(repo, nil, nil, mockXray, nil)

		uBefore, _ := repo.GetByID(context.Background(), 3)
		if uBefore.IsActive() {
			t.Fatalf("expected user to be inactive before renew")
		}

		if err := svc.BatchRenew(context.Background(), []uint{3}, 30); err != nil {
			t.Fatalf("BatchRenew failed: %v", err)
		}

		uAfter, _ := repo.GetByID(context.Background(), 3)
		if !uAfter.IsActive() {
			t.Fatalf("expected user to be active after renew")
		}

		calls := mockXray.GetCalls()
		foundAdd := false
		for _, c := range calls {
			if c.op == "add" && c.email == "expired@test.com" && c.tag == "vless-in" {
				foundAdd = true
				break
			}
		}
		if !foundAdd {
			t.Fatalf("expected expired user to be added to xray after renew, got calls: %+v", calls)
		}
	})

	t.Run("UpdateUser removes existing user before adding on updated inbounds", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          4,
					Email:       "refresh@test.com",
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		svc := NewUserService(repo, nil, nil, mockXray, nil)

		dto := domain.UpdateUserDTO{
			InboundTags: []string{"vless-in", "trojan-in"},
			InboundTag:  "vless-in",
		}

		_, err := svc.UpdateUser(context.Background(), 4, dto)
		if err != nil {
			t.Fatalf("UpdateUser failed: %v", err)
		}

		calls := mockXray.GetCalls()
		// For vless-in, RemoveUser must appear BEFORE AddUser
		removeIdx := -1
		addIdx := -1
		for i, c := range calls {
			if c.email == "refresh@test.com" && c.tag == "vless-in" {
				if c.op == "remove" && removeIdx == -1 {
					removeIdx = i
				} else if c.op == "add" && addIdx == -1 {
					addIdx = i
				}
			}
		}

		if removeIdx == -1 {
			t.Fatalf("expected RemoveUser to be called for existing inbound tag before AddUser, got calls: %+v", calls)
		}
		if addIdx == -1 {
			t.Fatalf("expected AddUser to be called for inbound tag, got calls: %+v", calls)
		}
		if removeIdx >= addIdx {
			t.Fatalf("expected RemoveUser (idx %d) BEFORE AddUser (idx %d) to avoid Xray conflict, got calls: %+v", removeIdx, addIdx, calls)
		}
	})

	t.Run("SyncUserToFile and SaveConfigQuietly do not reload supervisor", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.json")
		_ = os.WriteFile(cfgPath, []byte("{}"), 0644)
		configMgr := xray.NewConfigManager(cfgPath, "")
		mockSup := &mockSupervisor{}
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          5,
					Email:       "quiet@test.com",
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
				},
			},
		}
		inboundRepo := &mockInboundRepo{
			inbounds: []domain.Inbound{
				{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
			},
		}

		configSvc := NewConfigService(configMgr, mockSup, inboundRepo, repo, nil, xray.NewXrayCompiler())

		user, _ := repo.GetByID(context.Background(), 5)
		if err := configSvc.SyncUserToFile(context.Background(), []string{"vless-in"}, user, false); err != nil {
			t.Fatalf("SyncUserToFile failed: %v", err)
		}

		if mockSup.reloadCalls != 0 {
			t.Fatalf("expected 0 supervisor reload calls during SyncUserToFile, got %d", mockSup.reloadCalls)
		}

		// Verify file was written by SaveConfigQuietly and contains valid JSON
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			t.Fatalf("read config failed: %v", err)
		}
		if len(content) <= 2 { // more than just "{}"
			t.Fatalf("expected config to be written quietly to disk, got content: %s", string(content))
		}
	})

	t.Run("CheckAndResetMonthlyTraffic restores active users to Xray and syncs quietly", func(t *testing.T) {
		today := time.Now().Day()
		lastMonth := 202601
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:             6,
					Email:          "monthly@test.com",
					ResetDay:       today,
					LastResetMonth: lastMonth,
					TotalBytes:     1000,
					UpBytes:        1200, // over quota, inactive
					DownBytes:      0,
					Enabled:        true,
					InboundTag:     "vless-in",
					InboundTags:    "vless-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		svc := NewUserService(repo, nil, nil, mockXray, nil)

		if err := svc.CheckAndResetMonthlyTraffic(context.Background()); err != nil {
			t.Fatalf("CheckAndResetMonthlyTraffic failed: %v", err)
		}

		calls := mockXray.GetCalls()
		foundAdd := false
		for _, c := range calls {
			if c.op == "add" && c.email == "monthly@test.com" && c.tag == "vless-in" {
				foundAdd = true
				break
			}
		}
		if !foundAdd {
			t.Fatalf("expected monthly reset user to be added to xray, got calls: %+v", calls)
		}
	})

	t.Run("ResetSubToken rotates UUID and SubToken, kicks old user from Xray and injects new", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          5,
					Email:       "reset_secret@test.com",
					UUID:        "old-uuid-1111",
					SubToken:    "old-token-2222",
					Enabled:     true,
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
				},
			},
		}
		mockXray := &mockXrayManager{}
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "config.json")
		_ = os.WriteFile(cfgPath, []byte("{}"), 0644)
		configMgr := xray.NewConfigManager(cfgPath, "")
		mockSup := &mockSupervisor{}
		configSvc := NewConfigService(configMgr, mockSup, nil, repo, nil, xray.NewXrayCompiler())

		svc := NewUserService(repo, nil, nil, mockXray, configSvc)

		user, err := svc.ResetSubToken(context.Background(), 5)
		if err != nil {
			t.Fatalf("ResetSubToken failed: %v", err)
		}

		if user.UUID == "old-uuid-1111" || user.UUID == "" {
			t.Fatalf("expected new UUID, got: %s", user.UUID)
		}
		if user.SubToken == "old-token-2222" || user.SubToken == "" {
			t.Fatalf("expected new SubToken, got: %s", user.SubToken)
		}

		calls := mockXray.GetCalls()
		foundRemove := false
		foundAdd := false
		for _, c := range calls {
			if c.op == "remove" && c.email == "reset_secret@test.com" && c.tag == "vless-in" {
				foundRemove = true
			}
			if c.op == "add" && c.email == "reset_secret@test.com" && c.tag == "vless-in" {
				foundAdd = true
			}
		}
		if !foundRemove || !foundAdd {
			t.Fatalf("expected remove followed by add on xray manager, got calls: %+v", calls)
		}
	})

	t.Run("CreateUser Saga rollback on Xray AddUser failure cleans DB and reverts inbounds", func(t *testing.T) {
		repo := &mockUserRepo{users: []domain.User{}}
		mockXray := &mockXrayManager{
			failAddUserTag: "trojan-in",
		}
		mockCfg := &mockConfigSynchronizer{}
		svc := NewUserService(repo, nil, nil, mockXray, mockCfg)

		dto := CreateUserDTO{
			Email:       "saga_create_fail@test.com",
			InboundTags: []string{"vless-in", "trojan-in"},
		}

		user, err := svc.CreateUser(context.Background(), dto)
		if err == nil {
			t.Fatalf("expected CreateUser to fail when trojan-in fails")
		}
		if user != nil {
			t.Fatalf("expected returned user to be nil on failure")
		}

		// 断言 SQLite 中创建的用户被干净回滚删除，不留孤儿数据
		existing, _ := repo.GetByEmail(context.Background(), "saga_create_fail@test.com")
		if existing != nil {
			t.Fatalf("expected user to be deleted from repo after Saga rollback, found: %+v", existing)
		}

		// 断言已成功添加的 vless-in 节点被逆向 RemoveUser
		calls := mockXray.GetCalls()
		foundRollbackRemove := false
		for _, c := range calls {
			if c.op == "remove" && c.tag == "vless-in" && c.email == "saga_create_fail@test.com" {
				foundRollbackRemove = true
			}
		}
		if !foundRollbackRemove {
			t.Fatalf("expected vless-in to be removed on Saga rollback, got calls: %+v", calls)
		}
		if mockCfg.syncCalls != 0 {
			t.Fatalf("expected config sync not to be called when Xray fails")
		}
	})

	t.Run("CreateUser Saga rollback on Config sync failure reverts all Xray nodes and cleans DB", func(t *testing.T) {
		repo := &mockUserRepo{users: []domain.User{}}
		mockXray := &mockXrayManager{}
		mockCfg := &mockConfigSynchronizer{
			failSyncUserToFile: errors.New("simulated disk write failure"),
		}
		svc := NewUserService(repo, nil, nil, mockXray, mockCfg)

		dto := CreateUserDTO{
			Email:       "saga_cfg_fail@test.com",
			InboundTags: []string{"vless-in", "trojan-in"},
		}

		user, err := svc.CreateUser(context.Background(), dto)
		if err == nil {
			t.Fatalf("expected CreateUser to fail on config sync failure")
		}
		if user != nil {
			t.Fatalf("expected user to be nil on failure")
		}

		// 断言 DB 中孤儿数据被清除
		existing, _ := repo.GetByEmail(context.Background(), "saga_cfg_fail@test.com")
		if existing != nil {
			t.Fatalf("expected user to be removed from DB after config sync failure")
		}

		// 断言所有 tags 均被 RemoveUser 撤销
		calls := mockXray.GetCalls()
		removedTags := make(map[string]bool)
		for _, c := range calls {
			if c.op == "remove" && c.email == "saga_cfg_fail@test.com" {
				removedTags[c.tag] = true
			}
		}
		if !removedTags["vless-in"] || !removedTags["trojan-in"] {
			t.Fatalf("expected both tags to be rolled back from Xray, got removedTags: %+v", removedTags)
		}
	})

	t.Run("UpdateUser Saga rollback restores DB and Xray to old snapshot on failure", func(t *testing.T) {
		origUser := domain.User{
			ID:          10,
			Email:       "update_rollback@test.com",
			UUID:        "uuid-orig",
			SubToken:    "token-orig",
			Flow:        "xtls-rprx-vision",
			TotalBytes:  1000,
			InboundTag:  "vless-in",
			InboundTags: "vless-in",
			Enabled:     true,
		}
		repo := &mockUserRepo{users: []domain.User{origUser}}
		mockXray := &mockXrayManager{
			failAddUserTag: "new-tag-in",
			failAddUserErr: errors.New("xray add node fail"),
		}
		mockCfg := &mockConfigSynchronizer{}
		svc := NewUserService(repo, nil, nil, mockXray, mockCfg)

		enabledTrue := true
		dto := domain.UpdateUserDTO{
			InboundTags: []string{"vless-in", "new-tag-in"},
			Flow:        "new-flow",
			TotalBytes:  2000,
			Enabled:     &enabledTrue,
		}

		_, err := svc.UpdateUser(context.Background(), 10, dto)
		if err == nil {
			t.Fatalf("expected UpdateUser to fail")
		}

		// 检查数据库记录已回滚恢复为原始数据
		dbUser, _ := repo.GetByID(context.Background(), 10)
		if dbUser.Flow != "xtls-rprx-vision" || dbUser.TotalBytes != 1000 || dbUser.InboundTags != "vless-in" {
			t.Fatalf("expected user in DB to be restored to snapshot, got: %+v", dbUser)
		}

		// 检查 Xray 节点：由于原用户是 active，回滚时应重新向 vless-in 恢复旧配置
		calls := mockXray.GetCalls()
		foundRestoreAdd := false
		for i := len(calls) - 1; i >= 0; i-- {
			c := calls[i]
			if c.op == "add" && c.tag == "vless-in" && c.email == "update_rollback@test.com" {
				foundRestoreAdd = true
				break
			}
		}
		if !foundRestoreAdd {
			t.Fatalf("expected old snapshot to be restored to Xray on rollback, got calls: %+v", calls)
		}
	})

	t.Run("ResetSubToken Saga rollback preserves original UUID and SubToken on failure", func(t *testing.T) {
		origUser := domain.User{
			ID:          20,
			Email:       "reset_rollback@test.com",
			UUID:        "preserved-uuid-999",
			SubToken:    "preserved-token-888",
			Enabled:     true,
			InboundTag:  "vless-in",
			InboundTags: "vless-in",
		}
		repo := &mockUserRepo{users: []domain.User{origUser}}
		mockXray := &mockXrayManager{
			failAddUserTag: "vless-in",
			failAddUserErr: errors.New("simulated xray failure during reset token add"),
		}
		mockCfg := &mockConfigSynchronizer{}
		svc := NewUserService(repo, nil, nil, mockXray, mockCfg)

		_, err := svc.ResetSubToken(context.Background(), 20)
		if err == nil {
			t.Fatalf("expected ResetSubToken to fail")
		}

		// 断言 DB 中的 UUID 与 SubToken 被完整还原，未被破坏
		dbUser, _ := repo.GetByID(context.Background(), 20)
		if dbUser.UUID != "preserved-uuid-999" {
			t.Fatalf("expected UUID to be restored to original, got: %s", dbUser.UUID)
		}
		if dbUser.SubToken != "preserved-token-888" {
			t.Fatalf("expected SubToken to be restored to original, got: %s", dbUser.SubToken)
		}
	})

	t.Run("DeleteUser removes from Xray and config file before deleting from DB", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{
					ID:          30,
					Email:       "delete_test@test.com",
					InboundTag:  "vless-in",
					InboundTags: "vless-in",
					Enabled:     true,
				},
			},
		}
		mockXray := &mockXrayManager{}
		mockCfg := &mockConfigSynchronizer{
			failSyncUserToFile: errors.New("config disk sync failed"),
		}
		svc := NewUserService(repo, nil, nil, mockXray, mockCfg)

		// 写盘失败时，删除操作应当被中断，DB 记录不被删除
		err := svc.DeleteUser(context.Background(), 30)
		if err == nil {
			t.Fatalf("expected DeleteUser to fail when config sync fails")
		}

		dbUser, _ := repo.GetByID(context.Background(), 30)
		if dbUser == nil {
			t.Fatalf("expected user to remain in DB when config file sync fails")
		}
	})

	t.Run("Batch operations collect and join all errors without silent continue", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{ID: 101, Email: "u101@test.com", Enabled: true, InboundTag: "vless-in", InboundTags: "vless-in"},
				// ID 102 does not exist
				{ID: 103, Email: "u103@test.com", Enabled: true, InboundTag: "vless-in", InboundTags: "vless-in"},
			},
		}
		mockXray := &mockXrayManager{}
		svc := NewUserService(repo, nil, nil, mockXray, nil)

		// 102 缺失应返回聚合错误，不静默丢弃
		err := svc.BatchRenew(context.Background(), []uint{101, 102, 103}, 10)
		if err == nil {
			t.Fatalf("expected BatchRenew to return aggregated error for missing id 102")
		}

		err = svc.BatchSetStatus(context.Background(), []uint{101, 102, 103}, false)
		if err == nil {
			t.Fatalf("expected BatchSetStatus to return aggregated error for missing id 102")
		}
	})

	t.Run("ResetTraffic returns error for non-existent user", func(t *testing.T) {
		repo := &mockUserRepo{users: []domain.User{}}
		svc := NewUserService(repo, nil, nil, nil, nil)
		err := svc.ResetTraffic(context.Background(), 999)
		if err == nil || !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrNotFound for non-existent user, got %v", err)
		}
	})

	t.Run("CreateUser returns ErrAlreadyExists when email exists", func(t *testing.T) {
		repo := &mockUserRepo{
			users: []domain.User{
				{ID: 1, Email: "dup@test.com"},
			},
		}
		svc := NewUserService(repo, nil, nil, nil, nil)
		_, err := svc.CreateUser(context.Background(), CreateUserDTO{
			Email:       "dup@test.com",
			InboundTags: []string{"vless-in"},
		})
		if err == nil || !errors.Is(err, domain.ErrAlreadyExists) {
			t.Fatalf("expected ErrAlreadyExists, got %v", err)
		}
	})
}
