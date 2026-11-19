package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeTrafficCollector struct {
	mu      sync.Mutex
	batches []*TrafficBatch
	idx     int
}

func (f *fakeTrafficCollector) CollectTrafficStats(_ context.Context) (*TrafficBatch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.idx >= len(f.batches) {
		return nil, nil
	}
	b := f.batches[f.idx]
	f.idx++
	return b, nil
}

func TestRunner_EndToEndSpoolRetryDiffSyncAndCircuitBreaker(t *testing.T) {
	var mu sync.Mutex
	var step int
	var receivedRequests []SyncRequest
	failNextHTTP := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		defer mu.Unlock()
		receivedRequests = append(receivedRequests, req)
		if failNextHTTP {
			failNextHTTP = false
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary error"))
			return
		}

		step++
		w.Header().Set("Content-Type", "application/json")
		var resp SyncResponse
		switch step {
		case 1:
			// Initial full config + initial user.
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      "cfg-v1",
				UsersHash:       "usr-v1",
				ConfigUnchanged: false,
				UsersUnchanged:  false,
				ConfigJSON:      `{"inbounds":[{"tag":"vless-1"}]}`,
				Users: []RemoteUser{
					{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "u1"},
				},
			}
		case 2:
			// Users changed only -> differential gRPC update without config reload.
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      "cfg-v1",
				UsersHash:       "usr-v2",
				ConfigUnchanged: true,
				UsersUnchanged:  false,
				Users: []RemoteUser{
					{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "u1"},
					{InboundTag: "vless-1", Protocol: "vless", Email: "bob@example.com", UUID: "u2"},
				},
			}
		case 3:
			// Circuit breaker triggered.
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateCircuitBroken,
				SyncIntervalSec: 20,
				ConfigUnchanged: true,
				UsersUnchanged:  true,
			}
		default:
			// Recovered back to active.
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      "cfg-v1",
				UsersHash:       "usr-v2",
				ConfigUnchanged: false,
				UsersUnchanged:  false,
				ConfigJSON:      `{"inbounds":[{"tag":"vless-1"}]}`,
				Users: []RemoteUser{
					{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "u1"},
					{InboundTag: "vless-1", Protocol: "vless", Email: "bob@example.com", UUID: "u2"},
				},
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	spoolDir := filepath.Join(dir, "spool")

	client, err := NewClient(srv.URL, "test-token", srv.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	spool, err := NewSpool(spoolDir)
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	cmdRunner := &fakeCommandRunner{}
	prober := &fakeHealthProber{}
	cfgMgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    cfgPath,
		ProbeTimeout:  200 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, cmdRunner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}
	userCtrl := &fakeUserController{}
	userSync, err := NewUserSyncer(userCtrl)
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}
	traffic := &fakeTrafficCollector{
		batches: []*TrafficBatch{
			{
				BatchID: "batch-1",
				Users:   []UserTrafficDelta{{Email: "alice@example.com", UpBytes: 100, DownBytes: 200}},
			},
		},
	}

	runner, err := NewRunner(RunnerConfig{AgentVersion: "0.1.0", Interval: 50 * time.Millisecond}, client, spool, cfgMgr, userSync, traffic, NewMetricsCollector())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	// Simulate HTTP 500 on first attempt: batch-1 must remain in spool.
	mu.Lock()
	failNextHTTP = true
	mu.Unlock()
	if err := runner.SyncOnce(context.Background()); err == nil {
		t.Fatalf("expected error on HTTP 500")
	}
	spooled, err := spool.Load()
	if err != nil || spooled == nil || spooled.BatchID != "batch-1" {
		t.Fatalf("expected batch-1 retained in spool, got %+v err=%v", spooled, err)
	}

	// Retry succeeds (step 1): batch-1 must be retransmitted and then acked/deleted; config-v1 applied.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("step 1 SyncOnce failed: %v", err)
	}
	spooled, _ = spool.Load()
	if spooled != nil {
		t.Fatalf("expected spool cleared after 200 OK")
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config.json written: %v", err)
	}

	// Step 2: differential user add (bob@example.com) without config reload.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("step 2 SyncOnce failed: %v", err)
	}
	if len(userCtrl.added) != 1 || userCtrl.added[0].Email != "bob@example.com" {
		t.Fatalf("expected 1 differential AddUser for bob, got %+v", userCtrl.added)
	}

	// Step 3: circuit_broken -> silence config applied.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("step 3 SyncOnce failed: %v", err)
	}
	rawCfg, _ := os.ReadFile(cfgPath)
	if string(rawCfg) == `{"inbounds":[{"tag":"vless-1"}]}` {
		t.Fatalf("expected silence config to replace business inbounds")
	}

	// Step 4: active again -> business config restored because circuit_broken cleared configHash.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("step 4 SyncOnce failed: %v", err)
	}
	rawCfg, _ = os.ReadFile(cfgPath)
	if string(rawCfg) != `{"inbounds":[{"tag":"vless-1"}]}` {
		t.Fatalf("expected business config restored, got %s", string(rawCfg))
	}

	// Verify batch-1 was sent in both the failed attempt and the successful retry.
	mu.Lock()
	defer mu.Unlock()
	if len(receivedRequests) < 2 || receivedRequests[1].TrafficBatch == nil || receivedRequests[1].TrafficBatch.BatchID != "batch-1" {
		t.Fatalf("expected batch-1 retransmitted on retry")
	}
}

func TestRunner_GracefulShutdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SyncResponse{
			OK:              true,
			DesiredState:    DesiredStateActive,
			SyncIntervalSec: 1,
			ConfigUnchanged: true,
			UsersUnchanged:  true,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	client, _ := NewClient(srv.URL, "tok", srv.Client())
	spool, _ := NewSpool(filepath.Join(dir, "spool"))
	cfgMgr, _ := NewConfigManager(ConfigManagerConfig{ConfigPath: filepath.Join(dir, "config.json")}, &fakeCommandRunner{}, &fakeHealthProber{})
	userSync, _ := NewUserSyncer(&fakeUserController{})
	runner, _ := NewRunner(RunnerConfig{Interval: 20 * time.Millisecond}, client, spool, cfgMgr, userSync, nil, NewMetricsCollector())

	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean exit, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("runner did not exit on ctx cancel")
	}
}
