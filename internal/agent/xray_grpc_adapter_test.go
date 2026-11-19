package agent

import (
	"context"
	"net"
	"sync"
	"testing"

	"panel/internal/adapter/xray"
	"panel/internal/adapter/xray/proto"

	"google.golang.org/grpc"
	protobuf "google.golang.org/protobuf/proto"
)

type captureHandlerServer struct {
	proto.UnimplementedHandlerServiceServer
	mu   sync.Mutex
	reqs []*proto.AlterInboundRequest
}

func (s *captureHandlerServer) AlterInbound(ctx context.Context, req *proto.AlterInboundRequest) (*proto.AlterInboundResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	return &proto.AlterInboundResponse{}, nil
}

func TestXrayGRPCAdapter_AddUser_ShadowsocksMethods(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	srv := grpc.NewServer()
	capture := &captureHandlerServer{}
	proto.RegisterHandlerServiceServer(srv, capture)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	grpcClient := xray.NewGRPCClient(lis.Addr().String())
	defer func() { _ = grpcClient.Close() }()

	adapter, err := NewXrayGRPCAdapter(grpcClient)
	if err != nil {
		t.Fatalf("NewXrayGRPCAdapter failed: %v", err)
	}

	ctx := context.Background()
	if err := adapter.AddUser(ctx, RemoteUser{
		InboundTag: "jp-ss2022",
		Protocol:   "shadowsocks",
		Email:      "alice@example.com",
		UUID:       "ss2022-user-key==",
		Method:     "2022-blake3-aes-128-gcm",
	}); err != nil {
		t.Fatalf("AddUser ss2022 failed: %v", err)
	}

	if err := adapter.AddUser(ctx, RemoteUser{
		InboundTag: "jp-ss-chacha",
		Protocol:   "shadowsocks",
		Email:      "bob@example.com",
		UUID:       "chacha-password",
		Method:     "chacha20-ietf-poly1305",
	}); err != nil {
		t.Fatalf("AddUser ss-chacha failed: %v", err)
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.reqs) != 2 {
		t.Fatalf("expected 2 AlterInbound requests, got %d", len(capture.reqs))
	}

	// 1. SS-2022 must produce xray.proxy.shadowsocks_2022.Account
	var op1 proto.AddUserOperation
	if err := protobuf.Unmarshal(capture.reqs[0].GetOperation().GetValue(), &op1); err != nil {
		t.Fatalf("unmarshal op1 failed: %v", err)
	}
	if gotType := op1.GetUser().GetAccount().GetType(); gotType != "xray.proxy.shadowsocks_2022.Account" {
		t.Fatalf("ss2022 account type = %q, want xray.proxy.shadowsocks_2022.Account", gotType)
	}
	var acc1 proto.Shadowsocks2022Account
	if err := protobuf.Unmarshal(op1.GetUser().GetAccount().GetValue(), &acc1); err != nil {
		t.Fatalf("unmarshal ss2022 account failed: %v", err)
	}
	if acc1.GetKey() != "ss2022-user-key==" {
		t.Fatalf("ss2022 account key = %q, want ss2022-user-key==", acc1.GetKey())
	}

	// 2. Classic SS with chacha20-ietf-poly1305 must produce CipherType_CHACHA20_POLY1305 (not AES_128_GCM)
	var op2 proto.AddUserOperation
	if err := protobuf.Unmarshal(capture.reqs[1].GetOperation().GetValue(), &op2); err != nil {
		t.Fatalf("unmarshal op2 failed: %v", err)
	}
	if gotType := op2.GetUser().GetAccount().GetType(); gotType != "xray.proxy.shadowsocks.Account" {
		t.Fatalf("ss-chacha account type = %q, want xray.proxy.shadowsocks.Account", gotType)
	}
	var acc2 proto.ShadowsocksAccount
	if err := protobuf.Unmarshal(op2.GetUser().GetAccount().GetValue(), &acc2); err != nil {
		t.Fatalf("unmarshal ss account failed: %v", err)
	}
	if acc2.GetCipherType() != proto.CipherType_CHACHA20_POLY1305 {
		t.Fatalf("ss-chacha cipherType = %v, want CHACHA20_POLY1305", acc2.GetCipherType())
	}
}
