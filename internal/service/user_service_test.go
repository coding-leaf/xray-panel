package service

import (
	"context"
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
