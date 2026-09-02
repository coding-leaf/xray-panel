package service

import (
	"context"
	"encoding/json"
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

func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
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
func (m *mockUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return &u, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (m *mockUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) { return nil, nil }
func (m *mockUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) { return nil, nil }
func (m *mockUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return m.users, nil }
func (m *mockUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error { return nil }
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
	today := time.Now().Day()
	nowMonth := time.Now().Year()*100 + int(time.Now().Month())
	lastMonth := 202601

	repo := &mockUserRepo{
		users: []domain.User{
			{ID: 1, Email: "user1@test.com", ResetDay: today, LastResetMonth: lastMonth, UpBytes: 1024, DownBytes: 2048},
			{ID: 2, Email: "user2@test.com", ResetDay: today, LastResetMonth: nowMonth, UpBytes: 5000, DownBytes: 5000}, // 本月已重置过，不能再重置
		},
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
	calls []xrayCall
	mu    sync.Mutex
}

func (m *mockXrayManager) AddUser(ctx context.Context, inboundTag string, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, xrayCall{op: "add", tag: inboundTag, email: user.Email})
	return nil
}

func (m *mockXrayManager) RemoveUser(ctx context.Context, inboundTag string, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, xrayCall{op: "remove", tag: inboundTag, email: email})
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
		configSvc := NewConfigService(configMgr, mockSup, nil, repo, nil)

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

		configSvc := NewConfigService(configMgr, mockSup, inboundRepo, repo, nil)

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
}


