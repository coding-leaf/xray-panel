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
	calls        atomic.Int64
	mu           sync.Mutex
	inboundUsers map[string][]*proto.User
}

func (m *mockXrayHandlerServer) AlterInbound(ctx context.Context, req *proto.AlterInboundRequest) (*proto.AlterInboundResponse, error) {
	m.calls.Add(1)
	return &proto.AlterInboundResponse{}, nil
}

func (m *mockXrayHandlerServer) GetInboundUsers(ctx context.Context, req *proto.GetInboundUserRequest) (*proto.GetInboundUserResponse, error) {
	m.calls.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inboundUsers == nil {
		return &proto.GetInboundUserResponse{}, nil
	}
	return &proto.GetInboundUserResponse{
		Users: m.inboundUsers[req.GetTag()],
	}, nil
}

type mockXrayStatsServer struct {
	proto.UnimplementedStatsServiceServer
	calls  atomic.Int64
	uptime atomic.Uint32
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

func (m *mockXrayStatsServer) GetSysStats(ctx context.Context, req *proto.SysStatsRequest) (*proto.SysStatsResponse, error) {
	m.calls.Add(1)
	return &proto.SysStatsResponse{
		NumGoroutine: 42,
		Uptime:       m.uptime.Load(),
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

func TestGRPCClient_GetInboundUsersAndSysUptime_AllProtocols(t *testing.T) {
	addr, hSrv, sSrv, cleanup := startMockXrayGRPCServer(t)
	defer cleanup()

	sSrv.uptime.Store(12345)

	buildUser := func(email string, inbound *domain.Inbound, user *domain.User) *proto.User {
		t.Helper()
		msg, err := BuildAccountMessage(inbound, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage(%s): %v", inbound.Protocol, err)
		}
		return &proto.User{
			Level:   0,
			Email:   email,
			Account: msg,
		}
	}

	hSrv.mu.Lock()
	hSrv.inboundUsers = map[string][]*proto.User{
		"vless-in": {
			buildUser("vless@example.com", &domain.Inbound{
				Tag:            "vless-in",
				Protocol:       "vless",
				StreamSettings: `{"network":"tcp","security":"reality"}`,
			}, &domain.User{
				Email: "vless@example.com",
				UUID:  "11111111-2222-3333-4444-555555555555",
				Flow:  "xtls-rprx-vision",
			}),
		},
		"vmess-in": {
			buildUser("vmess@example.com", &domain.Inbound{
				Tag:      "vmess-in",
				Protocol: "vmess",
			}, &domain.User{
				Email: "vmess@example.com",
				UUID:  "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			}),
		},
		"trojan-in": {
			buildUser("trojan@example.com", &domain.Inbound{
				Tag:      "trojan-in",
				Protocol: "trojan",
			}, &domain.User{
				Email: "trojan@example.com",
				UUID:  "trojan-secret",
			}),
		},
		"ss-in": {
			buildUser("ss@example.com", &domain.Inbound{
				Tag:          "ss-in",
				Protocol:     "shadowsocks",
				SettingsJSON: `{"method":"aes-256-gcm"}`,
			}, &domain.User{
				Email: "ss@example.com",
				UUID:  "ss-pass",
			}),
		},
		"ss2022-in": {
			buildUser("ss2022@example.com", &domain.Inbound{
				Tag:          "ss2022-in",
				Protocol:     "shadowsocks-2022",
				SettingsJSON: `{"method":"2022-blake3-aes-128-gcm"}`,
			}, &domain.User{
				Email: "ss2022@example.com",
				UUID:  "ss2022-key==",
			}),
		},
	}
	hSrv.mu.Unlock()

	client := NewGRPCClient(addr)
	defer func() {
		_ = client.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	uptime, err := client.GetSysUptime(ctx)
	if err != nil {
		t.Fatalf("GetSysUptime failed: %v", err)
	}
	if uptime != 12345 {
		t.Fatalf("GetSysUptime = %d, want 12345", uptime)
	}

	cases := []struct {
		tag  string
		want RuntimeInboundUser
	}{
		{
			tag: "vless-in",
			want: RuntimeInboundUser{
				InboundTag: "vless-in",
				Email:      "vless@example.com",
				Protocol:   "vless",
				UUID:       "11111111-2222-3333-4444-555555555555",
				Flow:       "xtls-rprx-vision",
			},
		},
		{
			tag: "vmess-in",
			want: RuntimeInboundUser{
				InboundTag: "vmess-in",
				Email:      "vmess@example.com",
				Protocol:   "vmess",
				UUID:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			},
		},
		{
			tag: "trojan-in",
			want: RuntimeInboundUser{
				InboundTag: "trojan-in",
				Email:      "trojan@example.com",
				Protocol:   "trojan",
				UUID:       "trojan-secret",
			},
		},
		{
			tag: "ss-in",
			want: RuntimeInboundUser{
				InboundTag: "ss-in",
				Email:      "ss@example.com",
				Protocol:   "shadowsocks",
				UUID:       "ss-pass",
				Method:     "aes-256-gcm",
			},
		},
		{
			tag: "ss2022-in",
			want: RuntimeInboundUser{
				InboundTag: "ss2022-in",
				Email:      "ss2022@example.com",
				Protocol:   "shadowsocks-2022",
				UUID:       "ss2022-key==",
			},
		},
	}

	for _, tc := range cases {
		got, err := client.GetInboundUsers(ctx, tc.tag)
		if err != nil {
			t.Fatalf("GetInboundUsers(%s) failed: %v", tc.tag, err)
		}
		if len(got) != 1 {
			t.Fatalf("GetInboundUsers(%s) len = %d, want 1", tc.tag, len(got))
		}
		if got[0] != tc.want {
			t.Fatalf("GetInboundUsers(%s)[0] = %+v, want %+v", tc.tag, got[0], tc.want)
		}
	}
}
