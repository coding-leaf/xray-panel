package xray

import (
	"context"
	"net"
	"sync"
	"testing"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func (m *mockInboundRepoForManager) FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error) {
	var matched []domain.Inbound
	for _, in := range m.inbounds {
		if in != nil && in.NodeID == nodeID {
			matched = append(matched, *in)
		}
	}
	return matched, nil
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

type fakeLocalHandlerServer struct {
	proto.UnimplementedHandlerServiceServer
	mu         sync.Mutex
	localTags  map[string]bool
	calledTags []string
}

func (s *fakeLocalHandlerServer) AlterInbound(ctx context.Context, req *proto.AlterInboundRequest) (*proto.AlterInboundResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calledTags = append(s.calledTags, req.GetTag())
	if !s.localTags[req.GetTag()] {
		return nil, status.Errorf(codes.Unknown, "app/proxyman/command: failed to get handler: handler not found: %s", req.GetTag())
	}
	return &proto.AlterInboundResponse{}, nil
}

func TestManager_RemoteInboundShortCircuit(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	srv := grpc.NewServer()
	fake := &fakeLocalHandlerServer{
		localTags: map[string]bool{"local-vless": true},
	}
	proto.RegisterHandlerServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	grpcClient := NewGRPCClient(lis.Addr().String())
	defer func() { _ = grpcClient.Close() }()

	repo := &mockInboundRepoForManager{
		inbounds: map[string]*domain.Inbound{
			"local-vless": {
				NodeID:   0,
				Tag:      "local-vless",
				Protocol: "vless",
				Enabled:  true,
			},
			"remote-jp-vless": {
				NodeID:   2,
				Tag:      "remote-jp-vless",
				Protocol: "vless",
				Enabled:  true,
			},
		},
	}

	mgr := NewManager(grpcClient, nil, nil, repo)
	ctx := context.Background()
	user := &domain.User{Email: "alice@example.com", UUID: "11111111-2222-3333-4444-555555555555"}

	if err := mgr.AddUser(ctx, "local-vless", user); err != nil {
		t.Fatalf("AddUser local-vless failed: %v", err)
	}
	if err := mgr.AddUser(ctx, "remote-jp-vless", user); err != nil {
		t.Fatalf("AddUser remote-jp-vless should skip local gRPC and return nil, got: %v", err)
	}
	if err := mgr.RemoveUser(ctx, "remote-jp-vless", user.Email); err != nil {
		t.Fatalf("RemoveUser remote-jp-vless should skip local gRPC and return nil, got: %v", err)
	}
	if err := mgr.RemoveUser(ctx, "local-vless", user.Email); err != nil {
		t.Fatalf("RemoveUser local-vless failed: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.calledTags) != 2 || fake.calledTags[0] != "local-vless" || fake.calledTags[1] != "local-vless" {
		t.Fatalf("local gRPC must ONLY be called for local-vless, got calledTags=%v", fake.calledTags)
	}
}
