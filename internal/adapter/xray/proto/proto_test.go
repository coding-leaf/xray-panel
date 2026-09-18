package proto_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	myproto "panel/internal/adapter/xray/proto"
)

type mockHandlerServer struct {
	myproto.UnimplementedHandlerServiceServer
	lastTag string
	lastOp  *myproto.TypedMessage
}

func (s *mockHandlerServer) AlterInbound(ctx context.Context, req *myproto.AlterInboundRequest) (*myproto.AlterInboundResponse, error) {
	s.lastTag = req.Tag
	s.lastOp = req.Operation
	return &myproto.AlterInboundResponse{}, nil
}

type mockStatsServer struct {
	myproto.UnimplementedStatsServiceServer
	lastPattern string
	lastReset   bool
}

func (s *mockStatsServer) QueryStats(ctx context.Context, req *myproto.QueryStatsRequest) (*myproto.QueryStatsResponse, error) {
	s.lastPattern = req.Pattern
	s.lastReset = req.Reset_
	return &myproto.QueryStatsResponse{
		Stat: []*myproto.Stat{
			{Name: "user>>>test@example.com>>>traffic>>>uplink", Value: 1000},
			{Name: "user>>>test@example.com>>>traffic>>>downlink", Value: 2000},
		},
	}, nil
}

func TestProtoSerializationAndRoundTrip(t *testing.T) {
	t.Run("VLESS Account", func(t *testing.T) {
		acc := &myproto.VLESSAccount{
			Id:         "vless-uuid",
			Flow:       "xtls-rprx-vision",
			Encryption: "none",
		}
		tm, err := myproto.ToTypedMessage(acc)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tm.Type != myproto.TypeVLESSAccount {
			t.Fatalf("expected type %s, got %s", myproto.TypeVLESSAccount, tm.Type)
		}

		raw, err := tm.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		got, ok := raw.(*myproto.VLESSAccount)
		if !ok {
			t.Fatalf("expected *VLESSAccount, got %T", raw)
		}
		if got.Id != acc.Id || got.Flow != acc.Flow || got.Encryption != acc.Encryption {
			t.Fatalf("mismatch: got %+v, want %+v", got, acc)
		}
	})

	t.Run("VMess Account", func(t *testing.T) {
		acc := &myproto.VMessAccount{
			Id: "vmess-uuid",
			SecuritySettings: &myproto.SecurityConfig{
				Type: myproto.SecurityType_AUTO,
			},
		}
		tm, err := myproto.ToTypedMessage(acc)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tm.Type != myproto.TypeVMessAccount {
			t.Fatalf("expected type %s, got %s", myproto.TypeVMessAccount, tm.Type)
		}

		raw, err := tm.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		got, ok := raw.(*myproto.VMessAccount)
		if !ok {
			t.Fatalf("expected *VMessAccount, got %T", raw)
		}
		if got.Id != acc.Id || got.SecuritySettings.Type != myproto.SecurityType_AUTO {
			t.Fatalf("mismatch: got %+v, want %+v", got, acc)
		}
	})

	t.Run("Trojan Account", func(t *testing.T) {
		acc := &myproto.TrojanAccount{Password: "secret123"}
		tm, err := myproto.ToTypedMessage(acc)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tm.Type != myproto.TypeTrojanAccount {
			t.Fatalf("expected type %s, got %s", myproto.TypeTrojanAccount, tm.Type)
		}

		raw, err := tm.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		got, ok := raw.(*myproto.TrojanAccount)
		if !ok {
			t.Fatalf("expected *TrojanAccount, got %T", raw)
		}
		if got.Password != acc.Password {
			t.Fatalf("mismatch: got %+v, want %+v", got, acc)
		}
	})

	t.Run("Shadowsocks Account", func(t *testing.T) {
		acc := &myproto.ShadowsocksAccount{
			Password:   "secret-ss",
			CipherType: myproto.CipherType_AES_256_GCM,
			IvCheck:    true,
		}
		tm, err := myproto.ToTypedMessage(acc)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tm.Type != myproto.TypeShadowsocksAccount {
			t.Fatalf("expected type %s, got %s", myproto.TypeShadowsocksAccount, tm.Type)
		}

		raw, err := tm.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		got, ok := raw.(*myproto.ShadowsocksAccount)
		if !ok {
			t.Fatalf("expected *ShadowsocksAccount, got %T", raw)
		}
		if got.Password != acc.Password || got.CipherType != myproto.CipherType_AES_256_GCM || !got.IvCheck {
			t.Fatalf("mismatch: got %+v, want %+v", got, acc)
		}
	})

	t.Run("AddUserOperation and RemoveUserOperation", func(t *testing.T) {
		addOp := &myproto.AddUserOperation{
			User: &myproto.User{
				Level: 0,
				Email: "user@test.com",
				Account: &myproto.TypedMessage{
					Type:  myproto.TypeTrojanAccount,
					Value: []byte("dummy"),
				},
			},
		}
		tmAdd, err := myproto.ToTypedMessage(addOp)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tmAdd.Type != myproto.TypeAddUserOperation {
			t.Fatalf("expected type %s, got %s", myproto.TypeAddUserOperation, tmAdd.Type)
		}

		rawAdd, err := tmAdd.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		gotAdd, ok := rawAdd.(*myproto.AddUserOperation)
		if !ok || gotAdd.User.Email != "user@test.com" {
			t.Fatalf("mismatch on AddUserOperation: %+v", gotAdd)
		}

		removeOp := &myproto.RemoveUserOperation{Email: "user@test.com"}
		tmRem, err := myproto.ToTypedMessage(removeOp)
		if err != nil {
			t.Fatalf("ToTypedMessage failed: %v", err)
		}
		if tmRem.Type != myproto.TypeRemoveUserOperation {
			t.Fatalf("expected type %s, got %s", myproto.TypeRemoveUserOperation, tmRem.Type)
		}

		rawRem, err := tmRem.GetInstance()
		if err != nil {
			t.Fatalf("GetInstance failed: %v", err)
		}
		gotRem, ok := rawRem.(*myproto.RemoveUserOperation)
		if !ok || gotRem.Email != "user@test.com" {
			t.Fatalf("mismatch on RemoveUserOperation: %+v", gotRem)
		}
	})
}

func TestGRPCClientServer(t *testing.T) {
	buffer := 1024 * 1024
	lis := bufconn.Listen(buffer)

	baseServer := grpc.NewServer()
	handlerSrv := &mockHandlerServer{}
	statsSrv := &mockStatsServer{}

	myproto.RegisterHandlerServiceServer(baseServer, handlerSrv)
	myproto.RegisterStatsServiceServer(baseServer, statsSrv)

	go func() {
		if err := baseServer.Serve(lis); err != nil {
			return
		}
	}()
	defer baseServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("Failed to dial bufnet: %v", err)
	}
	defer conn.Close()

	// 1. Test AlterInbound via HandlerServiceClient
	hClient := myproto.NewHandlerServiceClient(conn)
	vlessAcc := &myproto.VLESSAccount{Id: "id123", Flow: "xtls-rprx-vision"}
	accTM, _ := myproto.ToTypedMessage(vlessAcc)
	addOp := &myproto.AddUserOperation{
		User: &myproto.User{
			Level:   0,
			Email:   "alice@example.com",
			Account: accTM,
		},
	}
	opTM, _ := myproto.ToTypedMessage(addOp)

	_, err = hClient.AlterInbound(ctx, &myproto.AlterInboundRequest{
		Tag:       "inbound-443",
		Operation: opTM,
	})
	if err != nil {
		t.Fatalf("AlterInbound failed: %v", err)
	}

	if handlerSrv.lastTag != "inbound-443" {
		t.Fatalf("expected tag inbound-443, got %s", handlerSrv.lastTag)
	}
	if handlerSrv.lastOp.Type != myproto.TypeAddUserOperation {
		t.Fatalf("expected op type %s, got %s", myproto.TypeAddUserOperation, handlerSrv.lastOp.Type)
	}

	// 2. Test QueryStats via StatsServiceClient
	sClient := myproto.NewStatsServiceClient(conn)
	resp, err := sClient.QueryStats(ctx, &myproto.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  true,
	})
	if err != nil {
		t.Fatalf("QueryStats failed: %v", err)
	}
	if statsSrv.lastPattern != "user>>>" || !statsSrv.lastReset {
		t.Fatalf("expected pattern user>>> and reset=true, got %s, %v", statsSrv.lastPattern, statsSrv.lastReset)
	}
	if len(resp.Stat) != 2 {
		t.Fatalf("expected 2 stats, got %d", len(resp.Stat))
	}
	if resp.Stat[0].Value != 1000 || resp.Stat[1].Value != 2000 {
		t.Fatalf("unexpected stats values: %v, %v", resp.Stat[0].Value, resp.Stat[1].Value)
	}
}
