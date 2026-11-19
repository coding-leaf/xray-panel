package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

func setupGeoDataHandlerTestEnv(t *testing.T) (*GeoDataHandler, *service.GeoDataService, *service.NodeService, uint) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "geodata_handler_test.db")
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
	inboundRepo := repository.NewInboundRepository(db)
	userRepo := repository.NewUserRepository(db)
	compiler := xray.NewXrayCompiler(8080)

	nodeSvc := service.NewNodeService(nodeRepo, nodeRepo, inboundRepo, userRepo, compiler)
	created, err := nodeSvc.CreateNode(context.Background(), domain.CreateNodeDTO{
		Name: "Remote Geo Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	geoDir := t.TempDir()
	geoSvc := service.NewGeoDataService(filepath.Join(geoDir, "xray"), nil, nil)
	handler := NewGeoDataHandler(geoSvc).WithNodeService(nodeSvc)
	return handler, geoSvc, nodeSvc, created.ID
}

func waitGeoUpdateIdle(t *testing.T, geoSvc *service.GeoDataService) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !geoSvc.GetProgress().IsUpdating {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for local GeoData update worker to become idle")
}

func TestGeoDataHandler_UpdateGeoData_LocalZeroRegression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Point HTTP/HTTPS proxy at a closed loopback port so background worker fails immediately without external traffic.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	closedAddr := ln.Addr().String()
	_ = ln.Close()
	t.Setenv("HTTP_PROXY", "http://"+closedAddr)
	t.Setenv("HTTPS_PROXY", "http://"+closedAddr)

	handler, geoSvc, _, _ := setupGeoDataHandlerTestEnv(t)
	t.Cleanup(func() {
		waitGeoUpdateIdle(t, geoSvc)
	})

	// 1. No nodeId -> 202 Accepted
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update", nil)
	handler.UpdateGeoData(c1)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted without nodeId, got %d (%s)", w1.Code, w1.Body.String())
	}
	var msgBody map[string]string
	if err := json.Unmarshal(w1.Body.Bytes(), &msgBody); err != nil || msgBody["message"] == "" {
		t.Fatalf("expected non-empty message in 202 response, got %q (err=%v)", w1.Body.String(), err)
	}

	// Wait for worker to finish, then verify explicit nodeId=0 also triggers 202 Accepted
	waitGeoUpdateIdle(t, geoSvc)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update?nodeId=0", nil)
	handler.UpdateGeoData(c2)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for nodeId=0, got %d (%s)", w2.Code, w2.Body.String())
	}
}

func TestGeoDataHandler_UpdateGeoData_RemoteBranchingAndWakeup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, geoSvc, nodeSvc, nodeID := setupGeoDataHandlerTestEnv(t)

	// 1. Invalid nodeId -> 400 Bad Request
	for _, rawQuery := range []string{"?nodeId=abc", "?nodeId=-1"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update"+rawQuery, nil)
		handler.UpdateGeoData(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for %s, got %d (%s)", rawQuery, w.Code, w.Body.String())
		}
	}

	// 2. Missing NodeService for nodeId > 0 -> 500
	noNodeSvcHandler := NewGeoDataHandler(geoSvc)
	wNoSvc := httptest.NewRecorder()
	cNoSvc, _ := gin.CreateTestContext(wNoSvc)
	cNoSvc.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update?nodeId=1", nil)
	noNodeSvcHandler.UpdateGeoData(cNoSvc)
	if wNoSvc.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when nodeSvc nil, got %d", wNoSvc.Code)
	}

	// 3. Unknown nodeId -> 404 Not Found
	wMissing := httptest.NewRecorder()
	cMissing, _ := gin.CreateTestContext(wMissing)
	cMissing.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update?nodeId=99999", nil)
	handler.UpdateGeoData(cMissing)
	if wMissing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for missing node, got %d (%s)", wMissing.Code, wMissing.Body.String())
	}

	// 4. Timeout on remote nodeId > 0 -> 200 OK with pending=true
	nodeSvc.WithGeoWaitTimeout(60 * time.Millisecond)
	wTimeout := httptest.NewRecorder()
	cTimeout, _ := gin.CreateTestContext(wTimeout)
	cTimeout.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update?nodeId=1", nil)
	handler.UpdateGeoData(cTimeout)
	if wTimeout.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on remote geo timeout, got %d (%s)", wTimeout.Code, wTimeout.Body.String())
	}
	var timeoutPayload service.RemoteGeoUpdateResponse
	if err := json.Unmarshal(wTimeout.Body.Bytes(), &timeoutPayload); err != nil {
		t.Fatalf("unmarshal timeout payload failed: %v", err)
	}
	if !timeoutPayload.Pending || timeoutPayload.NodeID != nodeID {
		t.Fatalf("expected pending=true nodeId=%d, got %+v", nodeID, timeoutPayload)
	}

	// 5. Live 1-RTT wakeup via HandleAgentSync -> 200 OK with sizes and durationMs
	nodeSvc.WithGeoWaitTimeout(2 * time.Second)
	respCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/geodata/update?nodeId=1", nil)
		handler.UpdateGeoData(c)
		respCh <- w
	}()

	var reqID string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		syncResp, err := nodeSvc.HandleAgentSync(context.Background(), nodeID, &domain.AgentSyncRequest{})
		if err != nil {
			t.Fatalf("HandleAgentSync failed: %v", err)
		}
		if syncResp.GeoUpdateRequest != nil {
			reqID = syncResp.GeoUpdateRequest.RequestID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reqID == "" {
		t.Fatalf("expected GeoUpdateRequest delivered to agent")
	}

	if _, err := nodeSvc.HandleAgentSync(context.Background(), nodeID, &domain.AgentSyncRequest{
		GeoUpdateResult: &domain.AgentGeoUpdateResult{
			RequestID:     reqID,
			GeoIPSize:     10485760,
			GeoSiteSize:   5242880,
			DurationMs:    2850,
			UpdatedAtUnix: 1732020100,
		},
	}); err != nil {
		t.Fatalf("agent GeoUpdateResult ack failed: %v", err)
	}

	wLive := <-respCh
	if wLive.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on live remote geo update, got %d (%s)", wLive.Code, wLive.Body.String())
	}
	var livePayload service.RemoteGeoUpdateResponse
	if err := json.Unmarshal(wLive.Body.Bytes(), &livePayload); err != nil {
		t.Fatalf("unmarshal live payload failed: %v", err)
	}
	if livePayload.Pending || livePayload.NodeID != nodeID || livePayload.GeoIPSize != 10485760 || livePayload.GeoSiteSize != 5242880 || livePayload.DurationMs != 2850 || livePayload.UpdatedAt != 1732020100 || livePayload.Error != "" {
		t.Fatalf("unexpected live remote geo payload: %+v", livePayload)
	}
}
