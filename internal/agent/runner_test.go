package agent

import (
	"context"
	"encoding/json"
	"errors"
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

func TestRunner_AutonomousOfflineCircuitBreakerAndRecovery(t *testing.T) {
	var mu sync.Mutex
	failHTTP := false
	var step int
	businessCfg := `{"inbounds":[{"tag":"vless-1"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failHTTP {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		step++
		w.Header().Set("Content-Type", "application/json")
		var resp SyncResponse
		switch step {
		case 1:
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      "cfg-v1",
				UsersHash:       "usr-v1",
				ConfigJSON:      businessCfg,
				QuotaPolicy: &QuotaPolicy{
					TrafficLimitBytes: 1000,
					BreakThresholdPct: 95,
					CountMode:         TrafficCountModeBoth,
					Multiplier:        1.0,
					CycleUsedBytes:    900,
					CycleMonth:        202611,
				},
			}
		default:
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      "cfg-v1",
				UsersHash:       "usr-v1",
				ConfigJSON:      businessCfg,
				QuotaPolicy: &QuotaPolicy{
					TrafficLimitBytes: 0,
					BreakThresholdPct: 95,
					CountMode:         TrafficCountModeBoth,
					Multiplier:        1.0,
					CycleUsedBytes:    0,
					CycleMonth:        202611,
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
	quotaStore, err := NewQuotaStore(spoolDir)
	if err != nil {
		t.Fatalf("NewQuotaStore: %v", err)
	}
	cfgMgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    cfgPath,
		ProbeTimeout:  200 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, &fakeCommandRunner{}, &fakeHealthProber{})
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}
	userSync, err := NewUserSyncer(&fakeUserController{})
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}
	traffic := &fakeTrafficCollector{
		batches: []*TrafficBatch{
			nil, // Step 1 (online init): no traffic yet.
			{
				BatchID: "batch-offline-1",
				Inbounds: []InboundTrafficDelta{
					{Tag: "vless-1", UpBytes: 20, DownBytes: 20},
				},
			},
		},
	}

	runner, err := NewRunner(
		RunnerConfig{AgentVersion: "0.1.0", Interval: 50 * time.Millisecond},
		client,
		spool,
		cfgMgr,
		userSync,
		traffic,
		NewMetricsCollector(),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.WithQuotaStore(quotaStore)

	// Step 1: Online sync persists quotaPolicy and applies business config.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("step 1 SyncOnce: %v", err)
	}
	st := quotaStore.State()
	if st.Policy.TrafficLimitBytes != 1000 || st.Policy.CycleUsedBytes != 900 || st.LocalPendingUsedBytes != 0 {
		t.Fatalf("unexpected initial quota state: %+v", st)
	}

	// Step 2: Master fails -> fresh Xray batch (40 bytes) recorded (900+40=940 < 950).
	mu.Lock()
	failHTTP = true
	mu.Unlock()
	if err := runner.SyncOnce(context.Background()); err == nil {
		t.Fatalf("expected error on Master 500")
	}
	st = quotaStore.State()
	if st.LocalPendingUsedBytes != 40 || st.LocalCircuitBroken {
		t.Fatalf("expected pending=40 and not broken yet, got %+v", st)
	}

	// Step 3: Master fails again while batch-offline-1 is still spooled -> must NOT double-count pending batch.
	if err := runner.SyncOnce(context.Background()); err == nil {
		t.Fatalf("expected error on second Master 500")
	}
	st = quotaStore.State()
	if st.LocalPendingUsedBytes != 40 || st.LocalCircuitBroken {
		t.Fatalf("expected pending still 40 (no double count), got %+v", st)
	}

	// Step 4: Offline NIC delta (10 up + 5 down = 15) pushes total to 955 >= 950 -> autonomous circuit breaker!
	runner.handleOfflineQuota(context.Background(), HostMetrics{NicUpDelta: 10, NicDownDelta: 5}, nil)
	st = quotaStore.State()
	if st.LocalPendingUsedBytes != 55 || !st.LocalCircuitBroken {
		t.Fatalf("expected pending=55 and LocalCircuitBroken=true, got %+v", st)
	}
	rawCfg, _ := os.ReadFile(cfgPath)
	if string(rawCfg) == businessCfg {
		t.Fatalf("expected silence config applied by autonomous circuit breaker")
	}

	// Step 5: Master recovers and sends active + unlimited quotaPolicy -> clears broken state and restores business config.
	mu.Lock()
	failHTTP = false
	mu.Unlock()
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("recovery SyncOnce: %v", err)
	}
	st = quotaStore.State()
	if st.LocalPendingUsedBytes != 0 || st.LocalCircuitBroken || st.Policy.TrafficLimitBytes != 0 {
		t.Fatalf("expected quota state reset on recovery, got %+v", st)
	}
	rawCfg, _ = os.ReadFile(cfgPath)
	if string(rawCfg) != businessCfg {
		t.Fatalf("expected business config restored after recovery, got %s", string(rawCfg))
	}
}

type fakeLogCollector struct {
	mu    sync.Mutex
	lines []string
	err   error
	calls []struct {
		service string
		lines   int
	}
}

func (f *fakeLogCollector) Collect(_ context.Context, service string, lines int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		service string
		lines   int
	}{service: service, lines: lines})
	if f.err != nil {
		return nil, f.err
	}
	return f.lines, nil
}

func TestRunner_OnDemandLogCollectionAndLightweightAck(t *testing.T) {
	var mu sync.Mutex
	var requests []SyncRequest
	var nextLogReq *LogRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		logReq := nextLogReq
		nextLogReq = nil
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SyncResponse{
			OK:              true,
			DesiredState:    DesiredStateActive,
			SyncIntervalSec: 20,
			ConfigHash:      "cfg-v1",
			UsersHash:       "usr-v1",
			ConfigUnchanged: true,
			UsersUnchanged:  true,
			LogRequest:      logReq,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	client, err := NewClient(srv.URL, "test-token", srv.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	spool, err := NewSpool(filepath.Join(dir, "spool"))
	if err != nil {
		t.Fatalf("NewSpool: %v", err)
	}
	cfgMgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    filepath.Join(dir, "config.json"),
		ProbeTimeout:  200 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, &fakeCommandRunner{}, &fakeHealthProber{})
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}
	userSync, err := NewUserSyncer(&fakeUserController{})
	if err != nil {
		t.Fatalf("NewUserSyncer: %v", err)
	}
	traffic := &fakeTrafficCollector{
		batches: []*TrafficBatch{
			{BatchID: "b-1", Inbounds: []InboundTrafficDelta{{Tag: "vless-1", UpBytes: 10, DownBytes: 20}}},
			{BatchID: "b-2", Inbounds: []InboundTrafficDelta{{Tag: "vless-1", UpBytes: 30, DownBytes: 40}}},
		},
	}

	collector := &fakeLogCollector{
		lines: []string{"Nov 19 20:50:01 xray started", "Nov 19 20:50:02 connection accepted"},
	}

	runner, err := NewRunner(
		RunnerConfig{AgentVersion: "1.2.3", Interval: 20 * time.Second},
		client,
		spool,
		cfgMgr,
		userSync,
		traffic,
		NewMetricsCollector(),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.WithLogCollector(collector)

	// 1. Normal sync without LogRequest -> only 1 HTTP request, LogResult is nil, HostMetrics & TrafficBatch present.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #1: %v", err)
	}
	mu.Lock()
	if len(requests) != 1 {
		t.Fatalf("expected 1 request on normal sync, got %d", len(requests))
	}
	if requests[0].LogResult != nil {
		t.Fatalf("expected nil LogResult on normal sync, got %+v", requests[0].LogResult)
	}
	if requests[0].HostMetrics == nil || requests[0].TrafficBatch == nil {
		t.Fatalf("expected HostMetrics and TrafficBatch on normal sync, got metrics=%v batch=%v", requests[0].HostMetrics, requests[0].TrafficBatch)
	}
	requests = nil
	nextLogReq = &LogRequest{
		RequestID: "log-req-1",
		Service:   "xray",
		Lines:     50,
	}
	mu.Unlock()

	// 2. Sync with LogRequest -> sends normal sync #1 + immediate lightweight ack #2 with LogResult and nil HostMetrics/TrafficBatch.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #2: %v", err)
	}
	mu.Lock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (sync + lightweight ack), got %d", len(requests))
	}
	normalReq := requests[0]
	ackReq := requests[1]
	requests = nil
	mu.Unlock()

	if normalReq.TrafficBatch == nil || normalReq.TrafficBatch.BatchID != "b-2" {
		t.Fatalf("expected normal sync to carry b-2, got %+v", normalReq.TrafficBatch)
	}
	if ackReq.TrafficBatch != nil {
		t.Fatalf("expected lightweight ack TrafficBatch == nil, got %+v", ackReq.TrafficBatch)
	}
	if ackReq.HostMetrics != nil {
		t.Fatalf("expected lightweight ack HostMetrics == nil, got %+v", ackReq.HostMetrics)
	}
	if ackReq.AgentVersion != "1.2.3" {
		t.Fatalf("expected AgentVersion 1.2.3 on ack, got %q", ackReq.AgentVersion)
	}
	if ackReq.LogResult == nil {
		t.Fatal("expected LogResult on lightweight ack")
	}
	if ackReq.LogResult.RequestID != "log-req-1" || ackReq.LogResult.Service != "xray" || ackReq.LogResult.Error != "" {
		t.Fatalf("unexpected LogResult metadata: %+v", ackReq.LogResult)
	}
	if len(ackReq.LogResult.Lines) != 2 || ackReq.LogResult.CollectedAtUnix <= 0 {
		t.Fatalf("unexpected LogResult payload: %+v", ackReq.LogResult)
	}

	// 3. Collection error -> still sends lightweight ack with LogResult.Error populated.
	collector.mu.Lock()
	collector.err = errors.New("journalctl permission denied")
	collector.mu.Unlock()

	mu.Lock()
	nextLogReq = &LogRequest{
		RequestID: "log-req-2",
		Service:   "xray-agent",
		Lines:     100,
	}
	mu.Unlock()

	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #3: %v", err)
	}
	mu.Lock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests on error-ack cycle, got %d", len(requests))
	}
	errAck := requests[1]
	mu.Unlock()

	if errAck.TrafficBatch != nil || errAck.HostMetrics != nil {
		t.Fatalf("expected nil TrafficBatch and HostMetrics on error ack, got batch=%v metrics=%v", errAck.TrafficBatch, errAck.HostMetrics)
	}
	if errAck.LogResult == nil || errAck.LogResult.RequestID != "log-req-2" || errAck.LogResult.Error != "journalctl permission denied" {
		t.Fatalf("expected LogResult.Error to be propagated, got %+v", errAck.LogResult)
	}
}
