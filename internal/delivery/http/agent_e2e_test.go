package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/adapter/xray"
	"panel/internal/agent"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/delivery/http/middleware"
	"panel/internal/domain"
	"panel/internal/service"
)

type spyResponseWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *spyResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *spyResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

type mockCmdCall struct {
	name string
	args []string
}

type mockAgentCommandRunner struct {
	mu    sync.Mutex
	calls []mockCmdCall
}

func (r *mockAgentCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, mockCmdCall{name: name, args: append([]string(nil), args...)})
	if len(args) > 0 && args[0] == "version" {
		return []byte("Xray 25.1.1 (Xray, Penetrates Everything.)\n"), nil
	}
	return []byte("ok"), nil
}

func (r *mockAgentCommandRunner) reloadCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if c.name == "systemctl" && len(c.args) > 0 && c.args[0] == "reload-or-restart" {
			n++
		}
	}
	return n
}

func (r *mockAgentCommandRunner) testCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.calls {
		if len(c.args) >= 2 && c.args[0] == "run" && c.args[1] == "-test" {
			n++
		}
	}
	return n
}

type mockAgentProber struct {
	mu     sync.Mutex
	probes int
}

func (p *mockAgentProber) Probe(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probes++
	return nil
}

type mockAgentUserController struct {
	mu      sync.Mutex
	added   []agent.RemoteUser
	removed []string
}

func (u *mockAgentUserController) AddUser(_ context.Context, user agent.RemoteUser) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.added = append(u.added, user)
	return nil
}

func (u *mockAgentUserController) RemoveUser(_ context.Context, inboundTag, email string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.removed = append(u.removed, inboundTag+":"+email)
	return nil
}

type mockAgentTrafficCollector struct {
	mu      sync.Mutex
	batches []*agent.TrafficBatch
}

func (c *mockAgentTrafficCollector) push(b *agent.TrafficBatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batches = append(c.batches, b)
}

func (c *mockAgentTrafficCollector) CollectTrafficStats(_ context.Context) (*agent.TrafficBatch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.batches) == 0 {
		return nil, nil
	}
	b := c.batches[0]
	c.batches = c.batches[1:]
	return b, nil
}

func TestMasterAgent_E2E_Integration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "e2e_master.db")
	db, err := repository.InitSQLite(dbPath)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	nodeRepo := repository.NewNodeRepository(db)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	trafficLogRepo := repository.NewGormTrafficLogRepository(db)
	snapshotRepo := repository.NewGormConfigSnapshotRepository(db)
	auditLogRepo := repository.NewGormAuditLogRepository(db)

	compiler := xray.NewXrayCompiler(8080)
	masterConfigMgr := xray.NewConfigManager(filepath.Join(t.TempDir(), "master_xray.json"), "")

	nodeSvc := service.NewNodeService(nodeRepo, nodeRepo, inboundRepo, userRepo, compiler)
	configSvc := service.NewConfigService(masterConfigMgr, nil, inboundRepo, userRepo, snapshotRepo, compiler).
		WithNodeCacheInvalidator(nodeSvc).
		WithNodeRepo(nodeRepo)
	userSvc := service.NewUserService(userRepo, inboundRepo, trafficLogRepo, nil, configSvc).
		WithNodeCacheInvalidator(nodeSvc)
	auditSvc := service.NewAuditLogService(auditLogRepo)

	jwtSecret := "e2e-test-jwt-secret"
	adminJWT, err := middleware.GenerateToken("admin", jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	handlers := &deliveryHTTP.Handlers{
		Sub:       deliveryHTTP.NewSubHandler(nil),
		User:      deliveryHTTP.NewUserHandler(userSvc, nil, auditSvc),
		Inbound:   deliveryHTTP.NewInboundHandler(configSvc, auditSvc),
		Node:      deliveryHTTP.NewNodeHandler(nodeSvc, auditSvc),
		Agent:     deliveryHTTP.NewAgentHandler(nodeSvc),
		AgentAuth: deliveryHTTP.AgentAuth(nodeSvc),
	}
	router := deliveryHTTP.SetupRouter(handlers, jwtSecret, nil)

	var (
		spyMu          sync.Mutex
		lastSyncStatus int
		lastSyncBody   string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/sync" {
			sw := &spyResponseWriter{ResponseWriter: w}
			router.ServeHTTP(sw, r)
			spyMu.Lock()
			lastSyncStatus = sw.status
			lastSyncBody = sw.body.String()
			spyMu.Unlock()
			return
		}
		router.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Helper to call Admin HTTP API over real httptest.Server
	adminCall := func(method, path string, payload interface{}) (int, []byte) {
		t.Helper()
		var body bytes.Buffer
		if payload != nil {
			if err := json.NewEncoder(&body).Encode(payload); err != nil {
				t.Fatalf("encode payload failed: %v", err)
			}
		}
		req, err := http.NewRequest(method, srv.URL+path, &body)
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminJWT)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("admin http %s %s failed: %v", method, path, err)
		}
		defer resp.Body.Close()
		var out bytes.Buffer
		_, _ = out.ReadFrom(resp.Body)
		return resp.StatusCode, out.Bytes()
	}

	// Provision remote node, local inbound (NodeID=0), remote inbound (NodeID=created.ID), and initial user
	code, rawNode := adminCall(http.MethodPost, "/api/admin/nodes", map[string]interface{}{
		"name":   "E2E Tokyo Edge",
		"host":   "127.0.0.1",
		"region": "JP",
	})
	if code != http.StatusCreated {
		t.Fatalf("create node expected 201, got %d: %s", code, string(rawNode))
	}
	var created domain.CreateNodeResponse
	if err := json.Unmarshal(rawNode, &created); err != nil {
		t.Fatalf("unmarshal created node: %v", err)
	}

	code, rawLocalInb := adminCall(http.MethodPost, "/api/inbounds", map[string]interface{}{
		"nodeId":   0,
		"tag":      "local-only-vless",
		"port":     8443,
		"protocol": "vless",
		"enabled":  true,
	})
	if code != http.StatusCreated {
		t.Fatalf("create local inbound expected 201, got %d: %s", code, string(rawLocalInb))
	}

	code, rawRemoteInb := adminCall(http.MethodPost, "/api/inbounds", map[string]interface{}{
		"nodeId":         created.ID,
		"tag":            "edge-vless",
		"port":           443,
		"protocol":       "vless",
		"streamSettings": `{"network":"tcp","security":"reality","realitySettings":{"dest":"apple.com:443","serverNames":["apple.com"],"privateKey":"OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc"}}`,
		"enabled":        true,
	})
	if code != http.StatusCreated {
		t.Fatalf("create remote inbound expected 201, got %d: %s", code, string(rawRemoteInb))
	}

	code, rawUser := adminCall(http.MethodPost, "/api/users", map[string]interface{}{
		"email":       "alice@example.com",
		"inboundTags": []string{"edge-vless"},
	})
	if code != http.StatusCreated {
		t.Fatalf("create user expected 201, got %d: %s", code, string(rawUser))
	}

	// 1. Invalid token -> 401; Disabled node -> 403
	badClient, err := agent.NewClient(srv.URL, "invalid-agent-token", srv.Client())
	if err != nil {
		t.Fatalf("NewClient bad: %v", err)
	}
	if _, err := badClient.Sync(context.Background(), &agent.SyncRequest{}); err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("expected 401 error for invalid token, got: %v", err)
	}

	code, _ = adminCall(http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), map[string]interface{}{
		"enabled": false,
	})
	if code != http.StatusOK {
		t.Fatalf("disable node expected 200, got %d", code)
	}

	agentClient, err := agent.NewClient(srv.URL, created.Token, srv.Client())
	if err != nil {
		t.Fatalf("NewClient valid: %v", err)
	}
	if _, err := agentClient.Sync(context.Background(), &agent.SyncRequest{}); err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("expected 403 error for disabled node, got: %v", err)
	}

	code, _ = adminCall(http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), map[string]interface{}{
		"enabled": true,
	})
	if code != http.StatusOK {
		t.Fatalf("re-enable node expected 200, got %d", code)
	}

	// Wire real Agent Runner components
	agentDir := t.TempDir()
	agentCfgPath := filepath.Join(agentDir, "config.json")
	spoolDir := filepath.Join(agentDir, "spool")

	spool, err := agent.NewSpool(spoolDir)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	cmdRunner := &mockAgentCommandRunner{}
	prober := &mockAgentProber{}
	agentCfgMgr, err := agent.NewConfigManager(agent.ConfigManagerConfig{
		ConfigPath:    agentCfgPath,
		ServiceName:   "xray",
		ProbeTimeout:  500 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, cmdRunner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}
	userCtrl := &mockAgentUserController{}
	userSync, err := agent.NewUserSyncer(userCtrl)
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}
	trafficCol := &mockAgentTrafficCollector{}

	runner, err := agent.NewRunner(
		agent.RunnerConfig{AgentVersion: "0.1.0-e2e", Interval: time.Second},
		agentClient,
		spool,
		agentCfgMgr,
		userSync,
		trafficCol,
		agent.NewMetricsCollector(),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	ctx := context.Background()

	// 2. First sync: pulls compiled configJson + users and hot-reloads
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("first SyncOnce failed: %v", err)
	}
	if cmdRunner.testCount() != 1 || cmdRunner.reloadCount() != 1 {
		t.Fatalf("expected 1 xray -test and 1 reload, got test=%d reload=%d", cmdRunner.testCount(), cmdRunner.reloadCount())
	}
	cfgBytes, err := os.ReadFile(agentCfgPath)
	if err != nil {
		t.Fatalf("read agent config.json failed: %v", err)
	}
	cfgText := string(cfgBytes)
	if !strings.Contains(cfgText, `"edge-vless"`) || !strings.Contains(cfgText, `"freedom"`) || !strings.Contains(cfgText, `"127.0.0.1"`) {
		t.Fatalf("agent config.json missing expected remote inbound/direct/api blocks: %s", cfgText)
	}
	if strings.Contains(cfgText, "local-only-vless") {
		t.Fatalf("agent config.json must not include local NodeID=0 inbound: %s", cfgText)
	}
	if !strings.Contains(cfgText, "alice@example.com") {
		t.Fatalf("agent config.json missing active user alice@example.com: %s", cfgText)
	}

	// Verify Master reflects online state & hashes
	code, rawGetNode := adminCall(http.MethodGet, fmt.Sprintf("/api/admin/nodes/%d", created.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("get node expected 200, got %d", code)
	}
	var nodeAfter1 domain.Node
	if err := json.Unmarshal(rawGetNode, &nodeAfter1); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	if !nodeAfter1.Online || nodeAfter1.AgentVersion != "0.1.0-e2e" || nodeAfter1.XrayVersion != "25.1.1" {
		t.Fatalf("unexpected node state after sync1: %+v", nodeAfter1)
	}

	// 3. Second sync: sends configHash + usersHash -> hits configUnchanged=true, usersUnchanged=true short-circuit
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("second SyncOnce failed: %v", err)
	}
	if cmdRunner.testCount() != 1 || cmdRunner.reloadCount() != 1 {
		t.Fatalf("expected no reload on hash short-circuit, got test=%d reload=%d", cmdRunner.testCount(), cmdRunner.reloadCount())
	}
	spyMu.Lock()
	sync2Body := lastSyncBody
	sync2Status := lastSyncStatus
	spyMu.Unlock()
	if sync2Status != http.StatusOK {
		t.Fatalf("expected 200 on second sync, got %d", sync2Status)
	}
	var wireResp2 domain.AgentSyncResponse
	if err := json.Unmarshal([]byte(sync2Body), &wireResp2); err != nil {
		t.Fatalf("unmarshal wireResp2: %v", err)
	}
	if !wireResp2.ConfigUnchanged || !wireResp2.UsersUnchanged || wireResp2.ConfigJSON != "" || len(wireResp2.Users) != 0 {
		t.Fatalf("expected short-circuit response with empty payloads, got %+v (raw=%s)", wireResp2, sync2Body)
	}
	if strings.Contains(sync2Body, `"configJson"`) || strings.Contains(sync2Body, `"users"`) {
		t.Fatalf("expected omitted configJson/users keys on short-circuit, got raw: %s", sync2Body)
	}

	// Differential user addition: adding bob@example.com triggers gRPC AddUser without config reload
	code, _ = adminCall(http.MethodPost, "/api/users", map[string]interface{}{
		"email":       "bob@example.com",
		"inboundTags": []string{"edge-vless"},
	})
	if code != http.StatusCreated {
		t.Fatalf("create bob expected 201, got %d", code)
	}
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("user diff SyncOnce failed: %v", err)
	}
	if cmdRunner.reloadCount() != 1 {
		t.Fatalf("expected zero config reloads on user diff, got %d", cmdRunner.reloadCount())
	}
	userCtrl.mu.Lock()
	if len(userCtrl.added) != 1 || userCtrl.added[0].Email != "bob@example.com" {
		userCtrl.mu.Unlock()
		t.Fatalf("expected differential AddUser for bob@example.com, got %+v", userCtrl.added)
	}
	userCtrl.mu.Unlock()

	// 4. TrafficBatch reporting + strict idempotency on duplicate batchId
	batch := &agent.TrafficBatch{
		BatchID: "e2e-batch-001",
		Users: []agent.UserTrafficDelta{
			{Email: "alice@example.com", UpBytes: 1234, DownBytes: 5678},
		},
		Inbounds: []agent.InboundTrafficDelta{
			{Tag: "edge-vless", UpBytes: 1234, DownBytes: 5678},
		},
	}
	trafficCol.push(batch)
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("traffic SyncOnce failed: %v", err)
	}
	if spooled, err := spool.Load(); err != nil || spooled != nil {
		t.Fatalf("expected spool cleared after 200 OK, got spooled=%+v err=%v", spooled, err)
	}

	// Simulate lost ACK / retry with exact same batchId -> Master must remain strictly idempotent
	if err := spool.Save(batch); err != nil {
		t.Fatalf("spool.Save duplicate failed: %v", err)
	}
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("duplicate batch SyncOnce failed: %v", err)
	}
	if spooled, err := spool.Load(); err != nil || spooled != nil {
		t.Fatalf("expected duplicate batch spool cleared, got spooled=%+v err=%v", spooled, err)
	}

	// Assert User / Inbound / Node counters incremented exactly once
	code, rawUsers := adminCall(http.MethodGet, "/api/users", nil)
	if code != http.StatusOK {
		t.Fatalf("list users expected 200, got %d", code)
	}
	var listedUsers []domain.User
	_ = json.Unmarshal(rawUsers, &listedUsers)
	var alice *domain.User
	for i := range listedUsers {
		if listedUsers[i].Email == "alice@example.com" {
			alice = &listedUsers[i]
			break
		}
	}
	if alice == nil || alice.UpBytes != 1234 || alice.DownBytes != 5678 {
		t.Fatalf("expected alice traffic 1234/5678 (counted once), got %+v", alice)
	}

	code, rawInbs := adminCall(http.MethodGet, fmt.Sprintf("/api/inbounds?nodeId=%d", created.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("list node inbounds expected 200, got %d", code)
	}
	var nodeInbs []domain.Inbound
	_ = json.Unmarshal(rawInbs, &nodeInbs)
	if len(nodeInbs) != 1 || nodeInbs[0].UpBytes != 1234 || nodeInbs[0].DownBytes != 5678 {
		t.Fatalf("expected edge-vless traffic 1234/5678 (counted once), got %+v", nodeInbs)
	}

	code, rawNodeStats := adminCall(http.MethodGet, fmt.Sprintf("/api/admin/nodes/%d", created.ID), nil)
	if code != http.StatusOK {
		t.Fatalf("get node stats expected 200, got %d", code)
	}
	var nodeStats domain.Node
	_ = json.Unmarshal(rawNodeStats, &nodeStats)
	if nodeStats.XrayUpBytes != 1234 || nodeStats.XrayDownBytes != 5678 {
		t.Fatalf("expected node xray traffic 1234/5678 (counted once), got %+v", nodeStats)
	}

	// 5. Master switches desiredState -> circuit_broken (Agent suspends business inbounds) -> active (restores)
	code, _ = adminCall(http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), map[string]interface{}{
		"desiredState": domain.NodeStateCircuitBroken,
	})
	if code != http.StatusOK {
		t.Fatalf("set circuit_broken expected 200, got %d", code)
	}
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("circuit_broken SyncOnce failed: %v", err)
	}
	if cmdRunner.reloadCount() != 2 {
		t.Fatalf("expected reload #2 for circuit_broken silence config, got %d", cmdRunner.reloadCount())
	}
	silenceBytes, err := os.ReadFile(agentCfgPath)
	if err != nil {
		t.Fatalf("read silence config.json failed: %v", err)
	}
	if strings.Contains(string(silenceBytes), "edge-vless") {
		t.Fatalf("circuit_broken silence config must remove business inbound edge-vless: %s", string(silenceBytes))
	}
	if !strings.Contains(string(silenceBytes), `"api"`) {
		t.Fatalf("circuit_broken silence config must preserve api inbound: %s", string(silenceBytes))
	}

	// Steady-state while circuit_broken must not re-apply
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("steady circuit_broken SyncOnce failed: %v", err)
	}
	if cmdRunner.reloadCount() != 2 {
		t.Fatalf("expected no extra reload while already circuit_broken, got %d", cmdRunner.reloadCount())
	}

	// Recover back to active -> Agent restores full business config
	code, _ = adminCall(http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), map[string]interface{}{
		"desiredState": domain.NodeStateActive,
	})
	if code != http.StatusOK {
		t.Fatalf("set active expected 200, got %d", code)
	}
	if err := runner.SyncOnce(ctx); err != nil {
		t.Fatalf("recovery SyncOnce failed: %v", err)
	}
	if cmdRunner.reloadCount() != 3 {
		t.Fatalf("expected reload #3 when recovering to active, got %d", cmdRunner.reloadCount())
	}
	recoveredBytes, err := os.ReadFile(agentCfgPath)
	if err != nil {
		t.Fatalf("read recovered config.json failed: %v", err)
	}
	if !strings.Contains(string(recoveredBytes), `"edge-vless"`) || !strings.Contains(string(recoveredBytes), "bob@example.com") {
		t.Fatalf("recovered config.json missing edge-vless or bob@example.com: %s", string(recoveredBytes))
	}
}
