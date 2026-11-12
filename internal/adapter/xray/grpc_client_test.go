package xray

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"

	"google.golang.org/grpc"
)

type mockXrayHandlerServer struct {
	proto.UnimplementedHandlerServiceServer
	calls atomic.Int64
}

func (m *mockXrayHandlerServer) AlterInbound(ctx context.Context, req *proto.AlterInboundRequest) (*proto.AlterInboundResponse, error) {
	m.calls.Add(1)
	return &proto.AlterInboundResponse{}, nil
}

type mockXrayStatsServer struct {
	proto.UnimplementedStatsServiceServer
	calls atomic.Int64
}

func (m *mockXrayStatsServer) QueryStats(ctx context.Context, req *proto.QueryStatsRequest) (*proto.QueryStatsResponse, error) {
	m.calls.Add(1)
	return &proto.QueryStatsResponse{
		Stat: []*proto.Stat{
			{Name: "user>>>alice@example.com>>>traffic>>>uplink", Value: 1024},
			{Name: "inbound>>>vless-in>>>traffic>>>downlink", Value: 2048},
			{Name: "outbound>>>direct>>>traffic>>>uplink", Value: 512},
			{Name: "user>>>zero@example.com>>>traffic>>>uplink", Value: 0},
		},
	}, nil
}

func startMockXrayGRPCServer(t *testing.T) (string, *mockXrayHandlerServer, *mockXrayStatsServer, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp failed: %v", err)
	}
	srv := grpc.NewServer()
	hSrv := &mockXrayHandlerServer{}
	sSrv := &mockXrayStatsServer{}
	proto.RegisterHandlerServiceServer(srv, hSrv)
	proto.RegisterStatsServiceServer(srv, sSrv)

	go func() {
		_ = srv.Serve(lis)
	}()

	cleanup := func() {
		srv.Stop()
		_ = lis.Close()
	}
	return lis.Addr().String(), hSrv, sSrv, cleanup
}

func TestGRPCClient_OperationsSuccess(t *testing.T) {
	addr, hSrv, sSrv, cleanup := startMockXrayGRPCServer(t)
	defer cleanup()

	client := NewGRPCClient(addr)
	defer func() {
		_ = client.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	inbound := &domain.Inbound{
		Tag:      "vless-in",
		Protocol: "vless",
	}
	user := &domain.User{
		Email: "alice@example.com",
		UUID:  "11111111-2222-3333-4444-555555555555",
	}

	if err := client.AddUser(ctx, inbound, user); err != nil {
		t.Fatalf("AddUser failed: %v", err)
	}
	if err := client.RemoveUser(ctx, "vless-in", user.Email); err != nil {
		t.Fatalf("RemoveUser failed: %v", err)
	}
	stats, err := client.QueryTrafficStats(ctx, true)
	if err != nil {
		t.Fatalf("QueryTrafficStats failed: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 non-zero stats, got %d: %+v", len(stats), stats)
	}
	if hSrv.calls.Load() != 2 {
		t.Errorf("expected 2 handler calls, got %d", hSrv.calls.Load())
	}
	if sSrv.calls.Load() != 1 {
		t.Errorf("expected 1 stats call, got %d", sSrv.calls.Load())
	}

	// Close and verify subsequent call can reconnect cleanly.
	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := client.AddUser(ctx, inbound, user); err != nil {
		t.Fatalf("AddUser after Close failed: %v", err)
	}
}

func TestGRPCClient_ConcurrentRequestsAndClose(t *testing.T) {
	addr, _, _, cleanup := startMockXrayGRPCServer(t)
	defer cleanup()

	client := NewGRPCClient(addr)
	defer func() {
		_ = client.Close()
	}()

	inbound := &domain.Inbound{
		Tag:      "vless-in",
		Protocol: "vless",
	}
	user := &domain.User{
		Email: "race@example.com",
		UUID:  "11111111-2222-3333-4444-555555555555",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Warm up once before hammering concurrent RPCs and Close().
	if _, err := client.QueryTrafficStats(ctx, false); err != nil {
		t.Fatalf("initial QueryTrafficStats failed: %v", err)
	}

	const workers = 8
	const iters = 200
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				switch (id + i) % 3 {
				case 0:
					_ = client.AddUser(ctx, inbound, user)
				case 1:
					_ = client.RemoveUser(ctx, inbound.Tag, user.Email)
				default:
					_, _ = client.QueryTrafficStats(ctx, false)
				}
			}
		}(w)
	}

	for c := 0; c < 2; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = client.Close()
			}
		}()
	}

	wg.Wait()
}
