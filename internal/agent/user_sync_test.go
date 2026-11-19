package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type fakeUserController struct {
	mu      sync.Mutex
	added   []RemoteUser
	removed []string
	addErr  error
	rmErr   error
}

func (f *fakeUserController) AddUser(_ context.Context, user RemoteUser) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, user)
	return nil
}

func (f *fakeUserController) RemoveUser(_ context.Context, inboundTag, email string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rmErr != nil {
		return f.rmErr
	}
	f.removed = append(f.removed, inboundTag+":"+email)
	return nil
}

func TestUserSyncer_DifferentialReconcile(t *testing.T) {
	ctrl := &fakeUserController{}
	syncer, err := NewUserSyncer(ctrl)
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}

	syncer.ResetBaseline([]RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-1", Flow: "xtls-rprx-vision"},
		{InboundTag: "vless-1", Protocol: "vless", Email: "bob@example.com", UUID: "uuid-2"},
	})

	// No-op reconcile: unchanged users must trigger 0 gRPC calls.
	added, removed, err := syncer.Reconcile(context.Background(), []RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-1", Flow: "xtls-rprx-vision"},
		{InboundTag: "vless-1", Protocol: "vless", Email: "bob@example.com", UUID: "uuid-2"},
	})
	if err != nil {
		t.Fatalf("no-op Reconcile error: %v", err)
	}
	if added != 0 || removed != 0 || len(ctrl.added) != 0 || len(ctrl.removed) != 0 {
		t.Fatalf("expected 0 calls on no-op, got added=%d removed=%d", added, removed)
	}

	// Update alice's UUID, remove bob, add charlie.
	added, removed, err = syncer.Reconcile(context.Background(), []RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-1-new", Flow: "xtls-rprx-vision"},
		{InboundTag: "vless-1", Protocol: "vless", Email: "charlie@example.com", UUID: "uuid-3"},
	})
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}
	if added != 2 || removed != 2 {
		t.Fatalf("expected added=2 removed=2, got added=%d removed=%d", added, removed)
	}
}

func TestUserSyncer_IgnorableAndFatalErrors(t *testing.T) {
	ctrl := &fakeUserController{
		rmErr: errors.New("rpc error: user not found"),
	}
	syncer, _ := NewUserSyncer(ctrl)
	syncer.ResetBaseline([]RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "gone@example.com", UUID: "uuid-1"},
	})

	added, removed, err := syncer.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected ignorable not-found error to succeed, got: %v", err)
	}
	if removed != 1 || added != 0 {
		t.Fatalf("expected removed=1 added=0, got added=%d removed=%d", added, removed)
	}

	ctrl.addErr = errors.New("connection refused")
	_, _, err = syncer.Reconcile(context.Background(), []RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "new@example.com", UUID: "uuid-2"},
	})
	if err == nil {
		t.Fatalf("expected fatal error on connection refused")
	}
}
