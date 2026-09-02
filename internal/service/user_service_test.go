package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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


