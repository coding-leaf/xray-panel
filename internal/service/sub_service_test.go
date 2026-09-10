package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockSubUserRepo struct {
	user *domain.User
}

func (m *mockSubUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	return nil
}
func (m *mockSubUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockSubUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	return m.user, nil
}
func (m *mockSubUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	if m.user != nil && m.user.UUID == uuid {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.user != nil && m.user.Email == email {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	if m.user != nil && m.user.SubToken == token {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (m *mockSubUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return nil, nil }
func (m *mockSubUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (m *mockSubUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

func TestSubService_OnlySubTokenAllowed(t *testing.T) {
	user := &domain.User{
		ID:       1,
		Email:    "admin@example.com",
		UUID:     "11111111-2222-3333-4444-555555555555",
		SubToken: "secret_token_123456",
		Enabled:  true,
	}
	repo := &mockSubUserRepo{user: user}
	svc := NewSubService(repo, nil, nil)

	// 1. 通过 Token 查询应成功
	payload, err := svc.GetSubscriptionByToken(context.Background(), "secret_token_123456", "", "")
	if err != nil {
		t.Fatalf("expected success with valid sub token, got: %v", err)
	}
	if payload == nil || payload.UserEmail != "admin@example.com" {
		t.Fatalf("expected user payload, got: %+v", payload)
	}

	// 2. 通过 Email 查询必须被拒绝（防越权）
	_, err = svc.GetSubscriptionByToken(context.Background(), "admin@example.com", "", "")
	if err == nil {
		t.Fatalf("expected error when querying by plain email, but succeeded!")
	}

	// 3. 通过 UUID 查询必须被拒绝
	_, err = svc.GetSubscriptionByToken(context.Background(), "11111111-2222-3333-4444-555555555555", "", "")
	if err == nil {
		t.Fatalf("expected error when querying by raw UUID, but succeeded!")
	}
}

type mockSubInboundRepo struct {
	inbounds []domain.Inbound
}

func (m *mockSubInboundRepo) Create(ctx context.Context, in *domain.Inbound) error { return nil }
func (m *mockSubInboundRepo) Update(ctx context.Context, in *domain.Inbound) error { return nil }
func (m *mockSubInboundRepo) Delete(ctx context.Context, id uint) error            { return nil }
func (m *mockSubInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockSubInboundRepo) ListEnabled(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockSubInboundRepo) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return nil
}

func TestSubService_ExportUserSubscription_Formats(t *testing.T) {
	user := &domain.User{
		ID:          1,
		Email:       "test@example.com",
		UUID:        "11111111-2222-3333-4444-555555555555",
		SubToken:    "valid_sub_token_789",
		InboundTags: "node-vless",
		Enabled:     true,
	}
	userRepo := &mockSubUserRepo{user: user}
	inboundRepo := &mockSubInboundRepo{
		inbounds: []domain.Inbound{
			{
				Tag:            "node-vless",
				Protocol:       "vless",
				Port:           443,
				Listen:         "127.0.0.1",
				ExternalHost:   "node.example.com",
				Enabled:        true,
				StreamSettings: `{"network":"tcp","security":"none"}`,
			},
		},
	}
	svc := NewSubService(userRepo, inboundRepo, nil)

	formats := []string{"base64", "raw", "clash", "sing-box"}
	for _, fmtStr := range formats {
		payload, output, err := svc.ExportUserSubscription(context.Background(), "valid_sub_token_789", "", "", fmtStr)
		if err != nil {
			t.Fatalf("format %s failed: %v", fmtStr, err)
		}
		if payload == nil {
			t.Fatalf("format %s returned nil payload", fmtStr)
		}
		if len(output) == 0 {
			t.Fatalf("format %s returned empty output", fmtStr)
		}
	}
}

func TestSubService_GetUserShareInfo_SubRoutes(t *testing.T) {
	user := &domain.User{
		ID:          1,
		Email:       "test@example.com",
		UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
		SubToken:    "token123",
		InboundTags: "vless-in",
		Enabled:     true,
	}
	inboundRepo := &mockSubInboundRepo{
		inbounds: []domain.Inbound{
			{
				Tag:            "vless-in",
				Protocol:       "vless",
				Port:           443,
				Listen:         "0.0.0.0",
				ExternalHost:   "198.51.100.1",
				Enabled:        true,
				StreamSettings: `{"network":"tcp","security":"none"}`,
				SubRoutesJson:  `[{"id":"1","name":"日本落地","routeId":1,"outboundTag":"direct","enabled":true},{"id":"2","name":"新美国中转","routeId":3,"outboundTag":"out-us","enabled":true}]`,
			},
		},
	}
	userRepo := &mockSubUserRepo{user: user}
	svc := NewSubService(userRepo, inboundRepo, nil)

	resp, err := svc.GetUserShareInfo(context.Background(), 1, "https://panel.example.com")
	if err != nil {
		t.Fatalf("GetUserShareInfo failed: %v", err)
	}

	if len(resp.Nodes) != 2 {
		t.Fatalf("expected 2 nodes for expanded subroutes, got %d", len(resp.Nodes))
	}

	if resp.Nodes[0].Remark != "日本落地" {
		t.Errorf("expected node 1 remark to be '日本落地', got '%s'", resp.Nodes[0].Remark)
	}
	if resp.Nodes[1].Remark != "新美国中转" {
		t.Errorf("expected node 2 remark to be '新美国中转', got '%s'", resp.Nodes[1].Remark)
	}
}
