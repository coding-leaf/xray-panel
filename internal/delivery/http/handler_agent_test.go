package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/adapter/xray"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/delivery/http/middleware"
	"panel/internal/domain"
	"panel/internal/service"
)

func setupNodeHTTPTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "http_node_test.db")
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
	configMgr := xray.NewConfigManager(filepath.Join(t.TempDir(), "xray_config.json"), "")

	nodeSvc := service.NewNodeService(nodeRepo, nodeRepo, inboundRepo, userRepo, compiler)
	configSvc := service.NewConfigService(configMgr, nil, inboundRepo, userRepo, snapshotRepo, compiler).
		WithNodeCacheInvalidator(nodeSvc).
		WithNodeRepo(nodeRepo)
	userSvc := service.NewUserService(userRepo, inboundRepo, trafficLogRepo, nil, configSvc).
		WithNodeCacheInvalidator(nodeSvc)
	auditSvc := service.NewAuditLogService(auditLogRepo)

	jwtSecret := "unit-test-jwt-secret-value"
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
	return router, adminJWT
}

func doJSONReq(t *testing.T, router http.Handler, method, path, bearer string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body failed: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestNodeAndAgentHTTP_EndToEnd(t *testing.T) {
	router, adminJWT := setupNodeHTTPTestRouter(t)

	// 1. Admin creates a node
	createRes := doJSONReq(t, router, http.MethodPost, "/api/admin/nodes", adminJWT, map[string]interface{}{
		"name":   "Tokyo Edge",
		"host":   "127.0.0.1",
		"region": "JP",
	})
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", createRes.Code, createRes.Body.String())
	}
	var created domain.CreateNodeResponse
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal CreateNodeResponse failed: %v", err)
	}
	if created.ID == 0 || len(created.Token) != 64 {
		t.Fatalf("expected node ID and 64-hex token, got %+v", created)
	}

	// 2. Agent token CANNOT access /api/admin/nodes (least-privilege isolation)
	forbidAdmin := doJSONReq(t, router, http.MethodGet, "/api/admin/nodes", created.Token, nil)
	if forbidAdmin.Code != http.StatusUnauthorized {
		t.Fatalf("agent token must be rejected on /api/admin/nodes with 401, got %d", forbidAdmin.Code)
	}

	// 3. List nodes hides token & tokenHash
	listRes := doJSONReq(t, router, http.MethodGet, "/api/admin/nodes", adminJWT, nil)
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list nodes, got %d", listRes.Code)
	}
	if strings.Contains(listRes.Body.String(), created.Token) || strings.Contains(listRes.Body.String(), "tokenHash") {
		t.Fatalf("list nodes leaked token or tokenHash: %s", listRes.Body.String())
	}

	// 4. Create inbound bound to nodeId + create user
	inbRes := doJSONReq(t, router, http.MethodPost, "/api/inbounds", adminJWT, map[string]interface{}{
		"nodeId":         created.ID,
		"tag":            "tokyo-vless",
		"port":           443,
		"protocol":       "vless",
		"streamSettings": `{"network":"tcp","security":"reality","realitySettings":{"dest":"apple.com:443","serverNames":["apple.com"],"privateKey":"OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc"}}`,
		"enabled":        true,
	})
	if inbRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for remote inbound, got %d: %s", inbRes.Code, inbRes.Body.String())
	}
	var createdInbound domain.Inbound
	_ = json.Unmarshal(inbRes.Body.Bytes(), &createdInbound)
	if createdInbound.NodeID != created.ID {
		t.Fatalf("expected inbound nodeId=%d, got %d", created.ID, createdInbound.NodeID)
	}

	// Verify GET /api/inbounds?nodeId=... filters properly
	nodeInbList := doJSONReq(t, router, http.MethodGet, fmt.Sprintf("/api/inbounds?nodeId=%d", created.ID), adminJWT, nil)
	if nodeInbList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /api/inbounds?nodeId=..., got %d", nodeInbList.Code)
	}
	var filteredInbs []domain.Inbound
	_ = json.Unmarshal(nodeInbList.Body.Bytes(), &filteredInbs)
	if len(filteredInbs) != 1 || filteredInbs[0].Tag != "tokyo-vless" {
		t.Fatalf("expected 1 filtered inbound tokyo-vless, got %+v", filteredInbs)
	}
	localInbList := doJSONReq(t, router, http.MethodGet, "/api/inbounds?nodeId=0", adminJWT, nil)
	if localInbList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /api/inbounds?nodeId=0, got %d", localInbList.Code)
	}
	var localInbs []domain.Inbound
	_ = json.Unmarshal(localInbList.Body.Bytes(), &localInbs)
	if len(localInbs) != 0 {
		t.Fatalf("expected 0 local inbounds, got %+v", localInbs)
	}

	userRes := doJSONReq(t, router, http.MethodPost, "/api/users", adminJWT, map[string]interface{}{
		"email":       "charlie@example.com",
		"inboundTags": []string{"tokyo-vless"},
	})
	if userRes.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for user, got %d: %s", userRes.Code, userRes.Body.String())
	}

	// 5. Deleting node while inbound is bound must fail with 409 Conflict
	delConflict := doJSONReq(t, router, http.MethodDelete, fmt.Sprintf("/api/admin/nodes/%d", created.ID), adminJWT, nil)
	if delConflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict deleting node with bound inbound, got %d: %s", delConflict.Code, delConflict.Body.String())
	}

	// 6. Agent sync auth checks: missing -> 401, invalid -> 401
	unauth1 := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", "", map[string]interface{}{})
	if unauth1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on missing token, got %d", unauth1.Code)
	}
	unauth2 := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", "invalid-token", map[string]interface{}{})
	if unauth2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on invalid token, got %d", unauth2.Code)
	}

	// 7. Agent sync initial -> full configJson + users
	sync1 := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{
		AgentVersion: "0.1.0",
		XrayVersion:  "25.1.1",
		HostMetrics:  &domain.AgentHostMetrics{CPUPercent: 10.5, MemPercent: 25.0, NicUpDelta: 100, NicDownDelta: 200},
	})
	if sync1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on agent sync, got %d: %s", sync1.Code, sync1.Body.String())
	}
	var resp1 domain.AgentSyncResponse
	if err := json.Unmarshal(sync1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal sync1 failed: %v", err)
	}
	if !resp1.OK || resp1.ConfigUnchanged || resp1.UsersUnchanged || resp1.ConfigJSON == "" || len(resp1.Users) != 1 {
		t.Fatalf("unexpected initial sync response: %+v", resp1)
	}

	// 8. Agent sync second -> hash short-circuit + idempotent batch
	batch := &domain.AgentTrafficBatch{
		BatchID:  "http-batch-1",
		Users:    []domain.AgentUserTrafficDelta{{Email: "charlie@example.com", UpBytes: 300, DownBytes: 700}},
		Inbounds: []domain.AgentInboundTrafficDelta{{Tag: "tokyo-vless", UpBytes: 300, DownBytes: 700}},
	}
	sync2 := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{
		ConfigHash:   resp1.ConfigHash,
		UsersHash:    resp1.UsersHash,
		TrafficBatch: batch,
	})
	if sync2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on second sync, got %d", sync2.Code)
	}
	var resp2 domain.AgentSyncResponse
	_ = json.Unmarshal(sync2.Body.Bytes(), &resp2)
	if !resp2.ConfigUnchanged || !resp2.UsersUnchanged || resp2.ConfigJSON != "" || len(resp2.Users) != 0 {
		t.Fatalf("expected short-circuit response, got %+v", resp2)
	}

	// Repeat same batchId
	sync3 := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{
		ConfigHash:   resp1.ConfigHash,
		UsersHash:    resp1.UsersHash,
		TrafficBatch: batch,
	})
	if sync3.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on duplicate batch sync, got %d", sync3.Code)
	}

	// Verify user traffic incremented exactly once via /api/users
	usersListRes := doJSONReq(t, router, http.MethodGet, "/api/users", adminJWT, nil)
	if usersListRes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on /api/users, got %d", usersListRes.Code)
	}
	var users []domain.User
	_ = json.Unmarshal(usersListRes.Body.Bytes(), &users)
	if len(users) != 1 || users[0].UpBytes != 300 || users[0].DownBytes != 700 {
		t.Fatalf("expected charlie traffic 300/700 (counted once), got %+v", users)
	}

	// 9. Concurrent agent sync requests (race-detector check)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			res := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{
				ConfigHash: resp1.ConfigHash,
				UsersHash:  resp1.UsersHash,
				TrafficBatch: &domain.AgentTrafficBatch{
					BatchID: "concurrent-http-batch",
					Users:   []domain.AgentUserTrafficDelta{{Email: "charlie@example.com", UpBytes: 50, DownBytes: 50}},
				},
			})
			if res.Code != http.StatusOK {
				t.Errorf("concurrent sync %d failed with %d: %s", idx, res.Code, res.Body.String())
			}
		}(i)
	}
	wg.Wait()

	// 10. Disable node -> Agent sync returns 403 Forbidden
	disableRes := doJSONReq(t, router, http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), adminJWT, map[string]interface{}{
		"enabled": false,
	})
	if disableRes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on disable node, got %d", disableRes.Code)
	}
	forbiddenSync := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{})
	if forbiddenSync.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for disabled node, got %d", forbiddenSync.Code)
	}

	// 11. Re-enable & set desiredState=circuit_broken, then rotate token
	_ = doJSONReq(t, router, http.MethodPut, fmt.Sprintf("/api/admin/nodes/%d", created.ID), adminJWT, map[string]interface{}{
		"enabled":      true,
		"desiredState": domain.NodeStateCircuitBroken,
	})
	rotRes := doJSONReq(t, router, http.MethodPost, fmt.Sprintf("/api/admin/nodes/%d/rotate-token", created.ID), adminJWT, nil)
	if rotRes.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on rotate-token, got %d", rotRes.Code)
	}
	var rotated domain.RotateTokenResponse
	_ = json.Unmarshal(rotRes.Body.Bytes(), &rotated)
	if oldSync := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", created.Token, domain.AgentSyncRequest{}); oldSync.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked token, got %d", oldSync.Code)
	}
	newSync := doJSONReq(t, router, http.MethodPost, "/api/agent/sync", rotated.Token, domain.AgentSyncRequest{})
	if newSync.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for rotated token, got %d", newSync.Code)
	}
	var cbResp domain.AgentSyncResponse
	_ = json.Unmarshal(newSync.Body.Bytes(), &cbResp)
	if cbResp.DesiredState != domain.NodeStateCircuitBroken {
		t.Fatalf("expected desiredState=%q, got %q", domain.NodeStateCircuitBroken, cbResp.DesiredState)
	}

	// 12. Install script endpoint
	scriptRes := doJSONReq(t, router, http.MethodGet, fmt.Sprintf("/api/admin/nodes/%d/install-script?masterUrl=https://panel.example.test", created.ID), adminJWT, nil)
	if scriptRes.Code != http.StatusOK || !strings.Contains(scriptRes.Body.String(), "https://panel.example.test") {
		t.Fatalf("unexpected install script response (%d): %s", scriptRes.Code, scriptRes.Body.String())
	}

	// 13. Delete inbound then delete node succeeds
	delIb := doJSONReq(t, router, http.MethodDelete, fmt.Sprintf("/api/inbounds/%d", createdInbound.ID), adminJWT, nil)
	if delIb.Code != http.StatusOK {
		t.Fatalf("expected 200 OK deleting inbound, got %d: %s", delIb.Code, delIb.Body.String())
	}
	delNode := doJSONReq(t, router, http.MethodDelete, fmt.Sprintf("/api/admin/nodes/%d", created.ID), adminJWT, nil)
	if delNode.Code != http.StatusOK {
		t.Fatalf("expected 200 OK deleting node, got %d: %s", delNode.Code, delNode.Body.String())
	}
}
