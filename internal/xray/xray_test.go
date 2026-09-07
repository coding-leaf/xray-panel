package xray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	proxymanCommand "github.com/xtls/xray-core/app/proxyman/command"
	statsCommand "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockXrayServer implements both HandlerServiceServer and StatsServiceServer for unit testing.
type mockXrayServer struct {
	proxymanCommand.UnimplementedHandlerServiceServer
	statsCommand.UnimplementedStatsServiceServer

	mu              sync.Mutex
	alterInboundErr error
	queryStatsErr   error
	sysStatsErr     error

	lastAlterReq  *proxymanCommand.AlterInboundRequest
	alterRequests []*proxymanCommand.AlterInboundRequest
	lastQueryReq  *statsCommand.QueryStatsRequest

	statsEntries []*statsCommand.Stat
}

func (m *mockXrayServer) AlterInbound(ctx context.Context, req *proxymanCommand.AlterInboundRequest) (*proxymanCommand.AlterInboundResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAlterReq = req
	m.alterRequests = append(m.alterRequests, req)
	if m.alterInboundErr != nil {
		return nil, m.alterInboundErr
	}
	return &proxymanCommand.AlterInboundResponse{}, nil
}

func (m *mockXrayServer) QueryStats(ctx context.Context, req *statsCommand.QueryStatsRequest) (*statsCommand.QueryStatsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastQueryReq = req
	if m.queryStatsErr != nil {
		return nil, m.queryStatsErr
	}
	return &statsCommand.QueryStatsResponse{
		Stat: m.statsEntries,
	}, nil
}

func (m *mockXrayServer) GetSysStats(ctx context.Context, req *statsCommand.SysStatsRequest) (*statsCommand.SysStatsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sysStatsErr != nil {
		return nil, m.sysStatsErr
	}
	return &statsCommand.SysStatsResponse{
		NumGoroutine: 42,
		Alloc:        1024 * 1024,
		Uptime:       3600,
	}, nil
}

func startMockGRPCServer(t *testing.T, srv *mockXrayServer) (string, func()) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}

	grpcServer := grpc.NewServer()
	proxymanCommand.RegisterHandlerServiceServer(grpcServer, srv)
	statsCommand.RegisterStatsServiceServer(grpcServer, srv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	cleanup := func() {
		grpcServer.Stop()
		_ = lis.Close()
	}

	return lis.Addr().String(), cleanup
}

func setupTestDB(t *testing.T) (*bbolt.DB, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-xray.db")
	db, err := bbolt.Open(dbPath, 0600, &bbolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("failed to open test bbolt db: %v", err)
	}

	if err := InitStorage(db); err != nil {
		_ = db.Close()
		t.Fatalf("failed to init storage: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	return db, cleanup
}

func TestXrayClient_AddInboundUser_Success(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	inboundTag := "vless-in"
	email := "alice@example.com"
	userUUID := "11111111-2222-3333-4444-555555555555"
	flow := "xtls-rprx-vision"

	err = client.AddInboundUser(inboundTag, email, userUUID, flow)
	if err != nil {
		t.Fatalf("AddInboundUser failed: %v", err)
	}

	// 1. Verify mock gRPC call
	mockSrv.mu.Lock()
	req := mockSrv.lastAlterReq
	mockSrv.mu.Unlock()

	if req == nil {
		t.Fatal("expected AlterInboundRequest, got nil")
	}
	if req.Tag != inboundTag {
		t.Errorf("expected tag %q, got %q", inboundTag, req.Tag)
	}

	opInstance, err := req.Operation.GetInstance()
	if err != nil {
		t.Fatalf("failed to get Operation instance: %v", err)
	}
	op, ok := opInstance.(*proxymanCommand.AddUserOperation)
	if !ok {
		t.Fatalf("expected AddUserOperation, got %T", opInstance)
	}
	if op.User.Email != email {
		t.Errorf("expected email %q, got %q", email, op.User.Email)
	}

	accInstance, err := op.User.Account.GetInstance()
	if err != nil {
		t.Fatalf("failed to get Account instance: %v", err)
	}
	acc, ok := accInstance.(*vless.Account)
	if !ok {
		t.Fatalf("expected vless.Account, got %T", accInstance)
	}
	if acc.Id != userUUID || acc.Flow != flow {
		t.Errorf("expected uuid %q flow %q, got uuid %q flow %q", userUUID, flow, acc.Id, acc.Flow)
	}

	// 2. Verify BoltDB state
	user, err := client.GetInboundUser(inboundTag, email)
	if err != nil {
		t.Fatalf("GetInboundUser failed: %v", err)
	}
	if user == nil {
		t.Fatal("expected user in db, got nil")
	}
	if user.Email != email || user.UUID != userUUID || user.Flow != flow || user.InboundTag != inboundTag {
		t.Errorf("user record mismatch in db: %+v", user)
	}
}

func TestXrayClient_AddInboundUser_Polymorphic(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	// 1. VMess
	err = client.AddInboundUserWithProtocol("vmess-in", "vmess@test.com", "uuid-vmess", "", "vmess")
	if err != nil {
		t.Fatalf("AddInboundUserWithProtocol VMess failed: %v", err)
	}
	mockSrv.mu.Lock()
	req := mockSrv.lastAlterReq
	mockSrv.mu.Unlock()
	opInst, _ := req.Operation.GetInstance()
	op := opInst.(*proxymanCommand.AddUserOperation)
	accInst, _ := op.User.Account.GetInstance()
	if _, ok := accInst.(*vmess.Account); !ok {
		t.Fatalf("expected vmess.Account, got %T", accInst)
	}

	// 2. Trojan
	err = client.AddInboundUserWithProtocol("trojan-in", "trojan@test.com", "pass-trojan", "", "trojan")
	if err != nil {
		t.Fatalf("AddInboundUserWithProtocol Trojan failed: %v", err)
	}
	mockSrv.mu.Lock()
	req = mockSrv.lastAlterReq
	mockSrv.mu.Unlock()
	opInst, _ = req.Operation.GetInstance()
	op = opInst.(*proxymanCommand.AddUserOperation)
	accInst, _ = op.User.Account.GetInstance()
	trojanAcc, ok := accInst.(*trojan.Account)
	if !ok || trojanAcc.Password != "pass-trojan" {
		t.Fatalf("expected trojan.Account with pass-trojan, got %+v", accInst)
	}

	// 3. Shadowsocks
	err = client.AddInboundUserWithProtocol("ss-in", "ss@test.com", "pass-ss", "aes-256-gcm", "shadowsocks")
	if err != nil {
		t.Fatalf("AddInboundUserWithProtocol Shadowsocks failed: %v", err)
	}
	mockSrv.mu.Lock()
	req = mockSrv.lastAlterReq
	mockSrv.mu.Unlock()
	opInst, _ = req.Operation.GetInstance()
	op = opInst.(*proxymanCommand.AddUserOperation)
	accInst, _ = op.User.Account.GetInstance()
	ssAcc, ok := accInst.(*shadowsocks.Account)
	if !ok || ssAcc.Password != "pass-ss" || ssAcc.CipherType != shadowsocks.CipherType_AES_256_GCM {
		t.Fatalf("expected shadowsocks.Account with aes-256-gcm, got %+v", accInst)
	}
}

func TestXrayClient_AddInboundUser_GRPCFailure_DoesNotPersistDB(t *testing.T) {
	mockSrv := &mockXrayServer{
		alterInboundErr: status.Error(codes.InvalidArgument, "inbound tag not found"),
	}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	err = client.AddInboundUser("invalid-tag", "bob@example.com", "uuid-bob", "")
	if err == nil {
		t.Fatal("expected error from AddInboundUser, got nil")
	}

	// Verify BoltDB has NO record for bob
	user, err := client.GetInboundUser("invalid-tag", "bob@example.com")
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got err=%v user=%+v", err, user)
	}
}

func TestXrayClient_RemoveInboundUser_Success(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	// Pre-seed user in DB and via AddInboundUser
	inboundTag := "vless-in"
	email := "charlie@example.com"
	_ = client.AddInboundUser(inboundTag, email, "uuid-charlie", "")

	// Remove user
	err = client.RemoveInboundUser(inboundTag, email)
	if err != nil {
		t.Fatalf("RemoveInboundUser failed: %v", err)
	}

	// 1. Verify mock gRPC call
	mockSrv.mu.Lock()
	req := mockSrv.lastAlterReq
	mockSrv.mu.Unlock()

	if req == nil {
		t.Fatal("expected AlterInboundRequest, got nil")
	}
	if req.Tag != inboundTag {
		t.Errorf("expected tag %q, got %q", inboundTag, req.Tag)
	}

	opInstance, err := req.Operation.GetInstance()
	if err != nil {
		t.Fatalf("failed to get Operation instance: %v", err)
	}
	op, ok := opInstance.(*proxymanCommand.RemoveUserOperation)
	if !ok {
		t.Fatalf("expected RemoveUserOperation, got %T", opInstance)
	}
	if op.Email != email {
		t.Errorf("expected email %q, got %q", email, op.Email)
	}

	// 2. Verify BoltDB record is removed
	user, err := client.GetInboundUser(inboundTag, email)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound after deletion, got user=%+v err=%v", user, err)
	}
}

func TestXrayClient_RemoveInboundUser_GRPCFailure_PreservesDB(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	inboundTag := "vless-in"
	email := "david@example.com"
	_ = client.AddInboundUser(inboundTag, email, "uuid-david", "")

	// Make gRPC fail on next AlterInbound
	mockSrv.mu.Lock()
	mockSrv.alterInboundErr = status.Error(codes.Internal, "simulated xray error")
	mockSrv.mu.Unlock()

	err = client.RemoveInboundUser(inboundTag, email)
	if err == nil {
		t.Fatal("expected error from RemoveInboundUser, got nil")
	}

	// Verify DB record is preserved
	user, err := client.GetInboundUser(inboundTag, email)
	if err != nil || user == nil {
		t.Fatalf("expected user to remain in DB after gRPC failure, got err=%v user=%+v", err, user)
	}
	if user.Email != email {
		t.Errorf("expected email %q, got %q", email, user.Email)
	}
}

func TestXrayClient_RemoveInboundUser_EmptyDB(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	// BoltDB without pre-created buckets
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fresh.db")
	freshDB, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("bbolt open failed: %v", err)
	}
	defer freshDB.Close()

	client, err := NewXrayClient(addr, WithStorage(freshDB))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	// Removing non-existent user should succeed without error
	err = client.RemoveInboundUser("nonexistent-tag", "noone@example.com")
	if err != nil {
		t.Fatalf("expected nil error when removing user from empty DB, got: %v", err)
	}
}

func TestXrayClient_StorageKey_NoCollision(t *testing.T) {
	db, stopDB := setupTestDB(t)
	defer stopDB()

	// User 1: tag "inb:reality", email "user1"
	u1 := &InboundUser{
		InboundTag: "inb:reality",
		Email:      "user1",
		UUID:       "uuid-1",
	}
	// User 2: tag "inb", email "reality:user1"
	u2 := &InboundUser{
		InboundTag: "inb",
		Email:      "reality:user1",
		UUID:       "uuid-2",
	}

	if err := SaveUser(db, u1); err != nil {
		t.Fatalf("SaveUser u1 failed: %v", err)
	}
	if err := SaveUser(db, u2); err != nil {
		t.Fatalf("SaveUser u2 failed: %v", err)
	}

	// Verify both users can be retrieved independently without collision
	res1, err := GetUser(db, "inb:reality", "user1")
	if err != nil || res1 == nil || res1.UUID != "uuid-1" {
		t.Fatalf("GetUser u1 failed or collided: %+v, err: %v", res1, err)
	}

	res2, err := GetUser(db, "inb", "reality:user1")
	if err != nil || res2 == nil || res2.UUID != "uuid-2" {
		t.Fatalf("GetUser u2 failed or collided: %+v, err: %v", res2, err)
	}

	// Verify ListUsers with tag "inb" does NOT return u1
	inbUsers, err := ListUsers(db, "inb")
	if err != nil {
		t.Fatalf("ListUsers inb failed: %v", err)
	}
	if len(inbUsers) != 1 || inbUsers[0].UUID != "uuid-2" {
		t.Fatalf("expected only u2 for inb, got %d users: %+v", len(inbUsers), inbUsers)
	}
}

func TestXrayClient_QueryTraffic(t *testing.T) {
	mockSrv := &mockXrayServer{
		statsEntries: []*statsCommand.Stat{
			{Name: "user>>>alice@example.com>>>traffic>>>uplink", Value: 1048576},
			{Name: "user>>>alice@example.com>>>traffic>>>downlink", Value: 5242880},
			{Name: "user>>>bob@example.com>>>traffic>>>downlink", Value: 2048},
			{Name: "inbound>>>vless-in>>>traffic>>>downlink", Value: 999999},
		},
	}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	client, err := NewXrayClient(addr, WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	// 1. Raw QueryTraffic
	statsMap, err := client.QueryTraffic("user>>>", true)
	if err != nil {
		t.Fatalf("QueryTraffic failed: %v", err)
	}

	mockSrv.mu.Lock()
	lastReq := mockSrv.lastQueryReq
	mockSrv.mu.Unlock()

	if lastReq == nil || lastReq.Pattern != "user>>>" || !lastReq.Reset_ {
		t.Errorf("unexpected query req: %+v", lastReq)
	}

	if len(statsMap) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(statsMap))
	}
	if statsMap["user>>>alice@example.com>>>traffic>>>uplink"] != 1048576 {
		t.Errorf("alice uplink mismatch: %d", statsMap["user>>>alice@example.com>>>traffic>>>uplink"])
	}

	// 2. QueryUserTraffic helper
	userTraffic, err := client.QueryUserTraffic(false)
	if err != nil {
		t.Fatalf("QueryUserTraffic failed: %v", err)
	}

	alice := userTraffic["alice@example.com"]
	if alice == nil || alice.Uplink != 1048576 || alice.Downlink != 5242880 {
		t.Errorf("alice aggregated traffic mismatch: %+v", alice)
	}
	bob := userTraffic["bob@example.com"]
	if bob == nil || bob.Uplink != 0 || bob.Downlink != 2048 {
		t.Errorf("bob aggregated traffic mismatch: %+v", bob)
	}
}

func TestXrayClient_HealthCheck_And_Reconnect(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)

	client, err := NewXrayClient(addr, WithDialTimeout(1*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Initial health check should pass
	if err := client.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth failed on running server: %v", err)
	}
	if !client.IsHealthy(ctx) {
		t.Fatal("IsHealthy returned false on running server")
	}

	// 2. Stop server -> health check should fail
	stopSrv()
	time.Sleep(50 * time.Millisecond)

	checkCtx, checkCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer checkCancel()
	if err := client.CheckHealth(checkCtx); err == nil {
		t.Error("expected CheckHealth to fail on stopped server, got nil")
	}

	// 3. Restart server on a new listener and re-point client address
	newMockSrv := &mockXrayServer{}
	newLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	newAddr := newLis.Addr().String()

	newGrpcServer := grpc.NewServer()
	proxymanCommand.RegisterHandlerServiceServer(newGrpcServer, newMockSrv)
	statsCommand.RegisterStatsServiceServer(newGrpcServer, newMockSrv)
	go func() {
		_ = newGrpcServer.Serve(newLis)
	}()
	defer func() {
		newGrpcServer.Stop()
		_ = newLis.Close()
	}()

	client.addr = newAddr
	if err := client.Reconnect(context.Background()); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	if err := client.CheckHealth(context.Background()); err != nil {
		t.Fatalf("CheckHealth failed after reconnect: %v", err)
	}
}

func TestXrayClient_ReconnectStormPrevention(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	client, err := NewXrayClient(addr, WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	initialEpoch := client.getEpoch()
	var wg sync.WaitGroup
	callers := 10

	// 10 concurrent callers attempting to reconnect with the same seenEpoch
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.ReconnectAtEpoch(context.Background(), initialEpoch)
		}()
	}
	wg.Wait()

	finalEpoch := client.getEpoch()
	// Should have reconnected once (epoch incremented by 1), not 10 times!
	if finalEpoch > initialEpoch+2 {
		t.Errorf("reconnect storm detected: epoch grew from %d to %d", initialEpoch, finalEpoch)
	}
}

func TestXrayClient_SyncToDiskConfig(t *testing.T) {
	db, stopDB := setupTestDB(t)
	defer stopDB()

	// Pre-seed users for multiple inbounds
	users := []*InboundUser{
		{InboundTag: "vless-in", Email: "u1@vless.com", UUID: "uuid-vless-1", Flow: "xtls-rprx-vision"},
		{InboundTag: "vless-in", Email: "u2@vless.com", UUID: "uuid-vless-2", Flow: ""},
		{InboundTag: "vmess-in", Email: "u3@vmess.com", UUID: "uuid-vmess-1"},
		{InboundTag: "trojan-in", Email: "u4@trojan.com", UUID: "pass-trojan-1"},
	}
	for _, u := range users {
		if err := SaveUser(db, u); err != nil {
			t.Fatalf("SaveUser failed: %v", err)
		}
	}

	// Create a template config JSON with comments (JSONC)
	templateContent := `{
		// System API inbound (dokodemo-door)
		"inbounds": [
			{
				"tag": "api",
				"port": 10085,
				"protocol": "dokodemo-door",
				"settings": { "address": "127.0.0.1" }
			},
			{
				"tag": "vless-in",
				"port": 443,
				"protocol": "vless",
				"settings": {
					/* stale client */
					"clients": [
						{ "id": "stale-uuid", "email": "stale@vless.com" }
					]
				}
			},
			{
				"tag": "vmess-in",
				"port": 4433,
				"protocol": "vmess",
				"settings": {}
			},
			{
				"tag": "trojan-in",
				"port": 8443,
				"protocol": "trojan"
			},
			{
				"tag": "unmanaged-in",
				"port": 1080,
				"protocol": "socks",
				"settings": {
					"auth": "noauth"
				}
			}
		],
		"outbounds": [
			{ "tag": "direct", "protocol": "freedom" }
		]
	}`

	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.json")
	targetPath := filepath.Join(tmpDir, "target-config.json")

	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}

	// Execute SyncToDiskConfig
	if err := SyncToDiskConfig(db, templatePath, targetPath); err != nil {
		t.Fatalf("SyncToDiskConfig failed: %v", err)
	}

	// Verify target file exists and content is valid JSON
	targetBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read target config: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(targetBytes, &root); err != nil {
		t.Fatalf("target config is not valid JSON: %v", err)
	}

	inbounds, ok := root["inbounds"].([]interface{})
	if !ok {
		t.Fatalf("inbounds not found in target config")
	}

	inboundByTag := make(map[string]map[string]interface{})
	for _, inb := range inbounds {
		m := inb.(map[string]interface{})
		inboundByTag[m["tag"].(string)] = m
	}

	// 1. Verify vless-in (and mandatory decryption: none injection)
	vlessIn := inboundByTag["vless-in"]
	vlessSettings := vlessIn["settings"].(map[string]interface{})
	vlessClients := vlessSettings["clients"].([]interface{})
	if len(vlessClients) != 2 {
		t.Fatalf("expected 2 clients in vless-in, got %d", len(vlessClients))
	}
	c1 := vlessClients[0].(map[string]interface{})
	if c1["email"] != "u1@vless.com" || c1["id"] != "uuid-vless-1" || c1["flow"] != "xtls-rprx-vision" {
		t.Errorf("unexpected client 1 in vless-in: %+v", c1)
	}
	if vlessSettings["decryption"] != "none" {
		t.Errorf("expected decryption: none in vless settings, got %v", vlessSettings["decryption"])
	}

	// 2. Verify vmess-in
	vmessIn := inboundByTag["vmess-in"]
	vmessSettings := vmessIn["settings"].(map[string]interface{})
	vmessClients := vmessSettings["clients"].([]interface{})
	if len(vmessClients) != 1 {
		t.Fatalf("expected 1 client in vmess-in, got %d", len(vmessClients))
	}
	c2 := vmessClients[0].(map[string]interface{})
	if c2["email"] != "u3@vmess.com" || c2["id"] != "uuid-vmess-1" {
		t.Errorf("unexpected client in vmess-in: %+v", c2)
	}

	// 3. Verify trojan-in
	trojanIn := inboundByTag["trojan-in"]
	trojanSettings := trojanIn["settings"].(map[string]interface{})
	trojanClients := trojanSettings["clients"].([]interface{})
	if len(trojanClients) != 1 {
		t.Fatalf("expected 1 client in trojan-in, got %d", len(trojanClients))
	}
	c3 := trojanClients[0].(map[string]interface{})
	if c3["email"] != "u4@trojan.com" || c3["password"] != "pass-trojan-1" {
		t.Errorf("unexpected client in trojan-in: %+v", c3)
	}

	// 4. Verify unmanaged-in preserved its settings
	unmanagedIn := inboundByTag["unmanaged-in"]
	unmanagedSettings := unmanagedIn["settings"].(map[string]interface{})
	if unmanagedSettings["auth"] != "noauth" {
		t.Errorf("unmanaged inbound modified unexpectedly: %+v", unmanagedSettings)
	}

	// 5. Test in-place sync (templatePath == targetPath)
	if err := SyncToDiskConfig(db, targetPath, targetPath); err != nil {
		t.Fatalf("in-place SyncToDiskConfig failed: %v", err)
	}

	// 6. Test removing all users from managed inbound and verifying clients become empty
	_ = RemoveUser(db, "trojan-in", "u4@trojan.com")
	if err := SyncToDiskConfig(db, targetPath, targetPath); err != nil {
		t.Fatalf("SyncToDiskConfig after user removal failed: %v", err)
	}
	updatedBytes, _ := os.ReadFile(targetPath)
	var updatedRoot map[string]interface{}
	_ = json.Unmarshal(updatedBytes, &updatedRoot)
	for _, inb := range updatedRoot["inbounds"].([]interface{}) {
		m := inb.(map[string]interface{})
		if m["tag"] == "trojan-in" {
			settings := m["settings"].(map[string]interface{})
			clients := settings["clients"].([]interface{})
			if len(clients) != 0 {
				t.Errorf("expected 0 clients in trojan-in after deletion, got %d", len(clients))
			}
		}
	}
}

func TestXrayClient_SyncToDiskConfig_ErrorCases(t *testing.T) {
	db, stopDB := setupTestDB(t)
	defer stopDB()

	// 1. Nil DB
	if err := SyncToDiskConfig(nil, "tmp", "tmp"); !errors.Is(err, ErrStorageNotAvailable) {
		t.Errorf("expected ErrStorageNotAvailable, got %v", err)
	}

	// 2. Empty paths
	if err := SyncToDiskConfig(db, "", "tmp"); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got %v", err)
	}
	if err := SyncToDiskConfig(db, "tmp", ""); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter, got %v", err)
	}

	// 3. Non-existent template
	if err := SyncToDiskConfig(db, "/non/existent/path.json", "/tmp/out.json"); err == nil {
		t.Error("expected error for non-existent template, got nil")
	}

	// 4. Invalid JSON template
	tmpDir := t.TempDir()
	badJSON := filepath.Join(tmpDir, "bad.json")
	_ = os.WriteFile(badJSON, []byte("{ not-json"), 0644)
	if err := SyncToDiskConfig(db, badJSON, filepath.Join(tmpDir, "out.json")); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestXrayClient_ParameterValidation(t *testing.T) {
	client, _ := NewXrayClient("127.0.0.1:10085")

	// AddInboundUser validation
	if err := client.AddInboundUser("", "a@b.com", "uuid", ""); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty tag, got %v", err)
	}
	if err := client.AddInboundUser("tag", "", "uuid", ""); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty email, got %v", err)
	}
	if err := client.AddInboundUser("tag", "a@b.com", "", ""); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty uuid, got %v", err)
	}

	// RemoveInboundUser validation
	if err := client.RemoveInboundUser("", "a@b.com"); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty tag, got %v", err)
	}
	if err := client.RemoveInboundUser("tag", ""); !errors.Is(err, ErrInvalidParameter) {
		t.Errorf("expected ErrInvalidParameter for empty email, got %v", err)
	}
}

func TestXrayClient_Concurrency(t *testing.T) {
	mockSrv := &mockXrayServer{
		statsEntries: []*statsCommand.Stat{
			{Name: "user>>>concurrent@test.com>>>traffic>>>uplink", Value: 100},
		},
	}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	workers := 20
	iterations := 10

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				email := fmt.Sprintf("user-%d-%d@concurrency.test", workerID, i)
				uuid := fmt.Sprintf("uuid-%d-%d", workerID, i)

				// Add user
				if err := client.AddInboundUser("vless-in", email, uuid, "xtls-rprx-vision"); err != nil {
					t.Errorf("concurrent AddInboundUser failed: %v", err)
					return
				}

				// Query traffic
				if _, err := client.QueryTraffic("user>>>", false); err != nil {
					t.Errorf("concurrent QueryTraffic failed: %v", err)
					return
				}

				// Read user
				if u, err := client.GetInboundUser("vless-in", email); err != nil || u == nil {
					t.Errorf("concurrent GetInboundUser failed: %v", err)
					return
				}

				// Remove user
				if err := client.RemoveInboundUser("vless-in", email); err != nil {
					t.Errorf("concurrent RemoveInboundUser failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}

func TestXrayClient_WithDBPath_Validation(t *testing.T) {
	// 1. Empty path returns error
	_, err := NewXrayClient("127.0.0.1:10085", WithDBPath("   "))
	if !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected ErrInvalidParameter for empty DB path, got: %v", err)
	}

	// 2. Nested directory creation
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "sub", "dir", "xray.db")
	client, err := NewXrayClient("127.0.0.1:10085", WithDBPath(nestedPath))
	if err != nil {
		t.Fatalf("NewXrayClient failed with nested directory: %v", err)
	}
	defer client.Close()

	if _, err := os.Stat(nestedPath); err != nil {
		t.Fatalf("expected db file to exist at %s, got: %v", nestedPath, err)
	}
}

func TestXrayClient_AddInboundUser_UnsupportedProtocol(t *testing.T) {
	client, err := NewXrayClient("127.0.0.1:10085")
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	err = client.AddInboundUserWithProtocol("tag", "email@test.com", "uuid", "", "wireguard")
	if !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected ErrInvalidParameter for unsupported protocol wireguard, got: %v", err)
	}
}

func TestXrayClient_Concurrent_AddRemove_And_SyncToDisk(t *testing.T) {
	mockSrv := &mockXrayServer{}
	addr, stopSrv := startMockGRPCServer(t, mockSrv)
	defer stopSrv()

	db, stopDB := setupTestDB(t)
	defer stopDB()

	client, err := NewXrayClient(addr, WithStorage(db), WithDialTimeout(2*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.json")
	targetPath := filepath.Join(tmpDir, "target.json")
	templateJSON := `{
		"inbounds": [
			{
				"tag": "vless-in",
				"protocol": "vless",
				"settings": {}
			}
		]
	}`
	if err := os.WriteFile(templatePath, []byte(templateJSON), 0644); err != nil {
		t.Fatalf("write template failed: %v", err)
	}

	var syncWg sync.WaitGroup
	var workersWg sync.WaitGroup
	workers := 10
	iterations := 20
	stopSync := make(chan struct{})

	// Background syncer running SyncToDiskConfig concurrently
	syncWg.Add(1)
	go func() {
		defer syncWg.Done()
		for {
			select {
			case <-stopSync:
				return
			default:
				_ = client.SyncToDiskConfig(templatePath, targetPath)
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	// Multiple workers concurrently adding and removing users
	for w := 0; w < workers; w++ {
		workersWg.Add(1)
		workerID := w
		go func() {
			defer workersWg.Done()
			for i := 0; i < iterations; i++ {
				email := fmt.Sprintf("u-%d-%d@sync.test", workerID, i)
				uuid := fmt.Sprintf("uuid-%d-%d", workerID, i)

				if addErr := client.AddInboundUser("vless-in", email, uuid, "xtls-rprx-vision"); addErr != nil {
					t.Errorf("AddInboundUser failed: %v", addErr)
					return
				}

				if remErr := client.RemoveInboundUser("vless-in", email); remErr != nil {
					t.Errorf("RemoveInboundUser failed: %v", remErr)
					return
				}
			}
		}()
	}

	workersWg.Wait()
	close(stopSync)
	syncWg.Wait()

	// Final sync to disk
	if err := client.SyncToDiskConfig(templatePath, targetPath); err != nil {
		t.Fatalf("final SyncToDiskConfig failed: %v", err)
	}

	// Validate target JSON is valid
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("target file corrupted during concurrent sync: %v", err)
	}
}
