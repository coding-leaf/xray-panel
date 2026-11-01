package xray

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockInboundRepoForManager struct {
	inbounds map[string]*domain.Inbound
}

func (m *mockInboundRepoForManager) Create(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}

func (m *mockInboundRepoForManager) Update(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}

func (m *mockInboundRepoForManager) Delete(ctx context.Context, id uint) error {
	return nil
}

func (m *mockInboundRepoForManager) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}

func (m *mockInboundRepoForManager) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	if inb, ok := m.inbounds[tag]; ok {
		return inb, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockInboundRepoForManager) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return nil, nil
}

func (m *mockInboundRepoForManager) AddTraffic(ctx context.Context, tag string, up, down int64) error {
	return nil
}

func (m *mockInboundRepoForManager) ResetTraffic(ctx context.Context, id uint) error {
	return nil
}

func (m *mockInboundRepoForManager) UpdateLatency(ctx context.Context, tag string, latencyMs int64, isAlive bool) error {
	return nil
}

func TestManager_DynamicUserShortCircuit(t *testing.T) {
	repo := &mockInboundRepoForManager{
		inbounds: map[string]*domain.Inbound{
			"socks-in": {
				Tag:      "socks-in",
				Protocol: "socks",
			},
			"http-in": {
				Tag:      "http-in",
				Protocol: "http",
			},
			"doko-in": {
				Tag:      "doko-in",
				Protocol: "dokodemo-door",
			},
		},
	}

	// With nil grpcClient, any call that is not short-circuited would panic or dereference nil
	mgr := NewManager(nil, nil, nil, repo)
	ctx := context.Background()
	user := &domain.User{Email: "test@example.com", UUID: "11111111-2222-3333-4444-555555555555"}

	t.Run("AddUser short-circuits for non-dynamic protocols", func(t *testing.T) {
		for _, tag := range []string{"socks-in", "http-in", "doko-in"} {
			if err := mgr.AddUser(ctx, tag, user); err != nil {
				t.Errorf("expected nil error for tag %s, got %v", tag, err)
			}
		}
	})

	t.Run("RemoveUser short-circuits for non-dynamic protocols", func(t *testing.T) {
		for _, tag := range []string{"socks-in", "http-in", "doko-in"} {
			if err := mgr.RemoveUser(ctx, tag, user.Email); err != nil {
				t.Errorf("expected nil error for tag %s, got %v", tag, err)
			}
		}
	})
}
