package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestUserSyncer_AddUser_HandlerNotFoundNotIgnored(t *testing.T) {
	ctrl := &fakeUserController{
		addErr: errors.New("xray alter inbound add user failed: rpc error: code = Unknown desc = app/proxyman/command: handler not found: vless-1"),
	}
	syncer, err := NewUserSyncer(ctrl)
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}

	target := []RemoteUser{
		{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-1"},
	}

	added, _, err := syncer.Reconcile(context.Background(), target)
	if err == nil {
		t.Fatalf("expected AddUser handler-not-found error not to be ignored")
	}
	if added != 0 {
		t.Fatalf("expected added=0 when AddUser fails with handler not found, got %d", added)
	}

	// Verify user was NOT poisoned into applied baseline: next Reconcile after handler recovers must add alice.
	ctrl.mu.Lock()
	ctrl.addErr = nil
	ctrl.mu.Unlock()

	added, _, err = syncer.Reconcile(context.Background(), target)
	if err != nil {
		t.Fatalf("Reconcile after handler recovery failed: %v", err)
	}
	if added != 1 || len(ctrl.added) != 1 || ctrl.added[0].Email != "alice@example.com" {
		t.Fatalf("expected alice@example.com to be added after recovery, got added=%d ctrl.added=%+v", added, ctrl.added)
	}
}

type fakeRuntimeUserController struct {
	fakeUserController
	runtimeUsers map[string][]RemoteUser
	inspectErr   map[string]error
}

func (f *fakeRuntimeUserController) GetInboundUsers(_ context.Context, inboundTag string) ([]RemoteUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.inspectErr[inboundTag]; err != nil {
		return nil, err
	}
	src := f.runtimeUsers[inboundTag]
	out := make([]RemoteUser, len(src))
	copy(out, src)
	return out, nil
}

func TestUserSyncer_ReconcileWithRuntime_HealsDriftAndProtectsStaticClients(t *testing.T) {
	ctrl := &fakeRuntimeUserController{
		runtimeUsers: map[string][]RemoteUser{
			"vless-1": {
				{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-old", Flow: "xtls-rprx-vision"},
				{InboundTag: "vless-1", Protocol: "vless", Email: "zombie@example.com", UUID: "uuid-z"},
				{InboundTag: "vless-1", Protocol: "vless", Email: "relay-auto:to-us@internal", UUID: "relay-uuid"},
				{InboundTag: "vless-1", Protocol: "vless", Email: "custom-static@internal", UUID: "custom-uuid"},
			},
			"trojan-1": {
				{InboundTag: "trojan-1", Protocol: "trojan", Email: "trojan-1@trojan.local", UUID: "fixed-pwd"},
			},
			"ss2022-1": {
				// Decoded from Shadowsocks2022Account: Protocol="shadowsocks-2022", Method=""
				{InboundTag: "ss2022-1", Protocol: "shadowsocks-2022", Email: "ss2022@example.com", UUID: "key-1=="},
			},
			"ss-1": {
				// Decoded from ShadowsocksAccount CipherType_CHACHA20_POLY1305 -> "chacha20-poly1305"
				{InboundTag: "ss-1", Protocol: "shadowsocks", Email: "ss@example.com", UUID: "ss-pwd", Method: "chacha20-poly1305"},
			},
		},
	}

	syncer, err := NewUserSyncer(ctrl)
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}

	desired := []RemoteUser{
		// Credential changed -> must remove old and re-add new.
		{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "uuid-new", Flow: "xtls-rprx-vision"},
		// Missing in runtime -> must add.
		{InboundTag: "vless-1", Protocol: "vless", Email: "bob@example.com", UUID: "uuid-bob"},
		// SS2022 with Method set by Master BuildRemoteNodeUsers -> must match runtime without drift!
		{InboundTag: "ss2022-1", Protocol: "shadowsocks", Email: "ss2022@example.com", UUID: "key-1==", Method: "2022-blake3-aes-128-gcm"},
		// Classic SS with chacha20-ietf-poly1305 alias -> must match chacha20-poly1305 without drift!
		{InboundTag: "ss-1", Protocol: "shadowsocks", Email: "ss@example.com", UUID: "ss-pwd", Method: "chacha20-ietf-poly1305"},
	}

	staticEmails := map[string]struct{}{
		"custom-static@internal": {},
	}
	added, removed, err := syncer.ReconcileWithRuntime(
		context.Background(),
		desired,
		[]string{"vless-1", "trojan-1", "ss2022-1", "ss-1", "api"},
		staticEmails,
	)
	if err != nil {
		t.Fatalf("ReconcileWithRuntime failed: %v", err)
	}
	if added != 2 || removed != 2 {
		t.Fatalf("expected added=2 (alice+bob) removed=2 (alice-old+zombie), got added=%d removed=%d", added, removed)
	}
	for _, rm := range ctrl.removed {
		if rm == "vless-1:relay-auto:to-us@internal" || rm == "vless-1:custom-static@internal" || rm == "trojan-1:trojan-1@trojan.local" {
			t.Fatalf("static client must NEVER be removed, saw %q", rm)
		}
		if rm == "ss2022-1:ss2022@example.com" || rm == "ss-1:ss@example.com" {
			t.Fatalf("matching ss2022/ss alias user should not be treated as drifted, saw %q", rm)
		}
	}
}

func TestUserSyncer_ReconcileWithRuntime_HandlerMissingAndUnimplementedFallback(t *testing.T) {
	t.Run("handler not found returns ErrInboundHandlerMissing", func(t *testing.T) {
		ctrl := &fakeRuntimeUserController{
			inspectErr: map[string]error{
				"vless-1": errors.New("rpc error: code = Unknown desc = app/proxyman/command: handler not found: vless-1"),
			},
		}
		syncer, _ := NewUserSyncer(ctrl)
		_, _, err := syncer.ReconcileWithRuntime(context.Background(), []RemoteUser{
			{InboundTag: "vless-1", Protocol: "vless", Email: "a@example.com", UUID: "u1"},
		}, []string{"vless-1"}, nil)
		if !errors.Is(err, ErrInboundHandlerMissing) {
			t.Fatalf("expected ErrInboundHandlerMissing, got %v", err)
		}
	})

	t.Run("unimplemented falls back to baseline Reconcile", func(t *testing.T) {
		ctrl := &fakeRuntimeUserController{
			inspectErr: map[string]error{
				"vless-1": status.Error(codes.Unimplemented, "unknown method GetInboundUsers"),
			},
		}
		syncer, _ := NewUserSyncer(ctrl)
		added, removed, err := syncer.ReconcileWithRuntime(context.Background(), []RemoteUser{
			{InboundTag: "vless-1", Protocol: "vless", Email: "a@example.com", UUID: "u1"},
		}, []string{"vless-1"}, nil)
		if err != nil {
			t.Fatalf("expected graceful fallback on Unimplemented, got %v", err)
		}
		if added != 1 || removed != 0 || len(ctrl.added) != 1 {
			t.Fatalf("expected 1 added via fallback, got added=%d removed=%d", added, removed)
		}
	})

	t.Run("not a usermanager inbound skipped cleanly", func(t *testing.T) {
		ctrl := &fakeRuntimeUserController{
			inspectErr: map[string]error{
				"socks-1": errors.New("rpc error: code = Unknown desc = proxy is not a UserManager"),
			},
			runtimeUsers: map[string][]RemoteUser{
				"vless-1": {},
			},
		}
		syncer, _ := NewUserSyncer(ctrl)
		added, _, err := syncer.ReconcileWithRuntime(context.Background(), []RemoteUser{
			{InboundTag: "vless-1", Protocol: "vless", Email: "a@example.com", UUID: "u1"},
		}, []string{"socks-1", "vless-1"}, nil)
		if err != nil {
			t.Fatalf("expected non-usermanager inbound skipped, got %v", err)
		}
		if added != 1 {
			t.Fatalf("expected added=1, got %d", added)
		}
	})
}
