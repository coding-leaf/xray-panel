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
func (m *mockSubUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error { return nil }
func (m *mockSubUserRepo) Delete(ctx context.Context, id uint) error            { return nil }
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
