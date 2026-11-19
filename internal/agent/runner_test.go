package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
		if req.HostMetrics == nil && req.TrafficBatch == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigUnchanged: true,
				UsersUnchanged:  true,
			})
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

func TestRunner_OnDemandLogCollectionAndImmediateCommandResult(t *testing.T) {
	var mu sync.Mutex
	var syncReqs []SyncRequest
	var cmdResults []CommandResult
	var nextCommands []Command

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/agent/commands/") && strings.HasSuffix(r.URL.Path, "/result") {
			var res CommandResult
			if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			cmdResults = append(cmdResults, res)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		syncReqs = append(syncReqs, req)
		cmds := nextCommands
		nextCommands = nil
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
			Commands:        cmds,
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

	// 1. Normal sync without Commands -> only 1 HTTP sync, no command result POSTs.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #1: %v", err)
	}
	runner.WaitCommands()
	mu.Lock()
	if len(syncReqs) != 1 || len(cmdResults) != 0 {
		t.Fatalf("expected 1 sync and 0 command results, got syncs=%d results=%d", len(syncReqs), len(cmdResults))
	}
	if syncReqs[0].HostMetrics == nil || syncReqs[0].TrafficBatch == nil {
		t.Fatalf("expected HostMetrics and TrafficBatch on normal sync")
	}
	syncReqs = nil
	nextCommands = []Command{{
		ID:      "cmd-log-1",
		Type:    CommandTypeLogTail,
		Service: "xray",
		Lines:   50,
	}}
	mu.Unlock()

	// 2. Sync with log_tail command -> executes asynchronously and POSTs /api/agent/commands/cmd-log-1/result.
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #2: %v", err)
	}
	runner.WaitCommands()

	mu.Lock()
	if len(syncReqs) != 1 || len(cmdResults) != 1 {
		t.Fatalf("expected 1 sync and 1 command result POST, got syncs=%d results=%d", len(syncReqs), len(cmdResults))
	}
	normalReq := syncReqs[0]
	res1 := cmdResults[0]
	syncReqs = nil
	cmdResults = nil
	mu.Unlock()

	if normalReq.TrafficBatch == nil || normalReq.TrafficBatch.BatchID != "b-2" {
		t.Fatalf("expected normal sync to carry b-2, got %+v", normalReq.TrafficBatch)
	}
	if res1.ID != "cmd-log-1" || res1.Type != CommandTypeLogTail || res1.Status != CommandStatusSucceeded || res1.Service != "xray" || res1.Error != "" {
		t.Fatalf("unexpected log command result metadata: %+v", res1)
	}
	if len(res1.Lines) != 2 || res1.CollectedAtUnix <= 0 {
		t.Fatalf("unexpected log command result payload: %+v", res1)
	}

	// 3. Collection error -> reports Status=failed with Error populated.
	collector.mu.Lock()
	collector.err = errors.New("journalctl permission denied")
	collector.mu.Unlock()

	mu.Lock()
	nextCommands = []Command{{
		ID:      "cmd-log-2",
		Type:    CommandTypeLogTail,
		Service: "xray-agent",
		Lines:   100,
	}}
	mu.Unlock()

	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #3: %v", err)
	}
	runner.WaitCommands()

	mu.Lock()
	if len(cmdResults) != 1 {
		t.Fatalf("expected 1 error command result POST, got %d", len(cmdResults))
	}
	errRes := cmdResults[0]
	mu.Unlock()

	if errRes.ID != "cmd-log-2" || errRes.Status != CommandStatusFailed || errRes.Error != "journalctl permission denied" {
		t.Fatalf("expected failed command result with error, got %+v", errRes)
	}
}

type fakeGeoDataUpdater struct {
	mu          sync.Mutex
	calls       int
	geoipSize   int64
	geositeSize int64
	err         error
	startedCh   chan struct{}
	releaseCh   chan struct{}
}

func (f *fakeGeoDataUpdater) Update(ctx context.Context) (int64, int64, error) {
	f.mu.Lock()
	f.calls++
	started := f.startedCh
	release := f.releaseCh
	f.mu.Unlock()

	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return 0, 0, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return f.geoipSize, f.geositeSize, f.err
}

func TestRunner_ImmediateLightweightAckOnConfigAndUsersApply(t *testing.T) {
	var mu sync.Mutex
	var requests []SyncRequest
	var nextResp SyncResponse

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		var resp SyncResponse
		if req.HostMetrics == nil && req.TrafficBatch == nil {
			// Lightweight ack -> Master replies unchanged so no recursive ack is triggered.
			resp = SyncResponse{
				OK:              true,
				DesiredState:    DesiredStateActive,
				SyncIntervalSec: 20,
				ConfigHash:      req.ConfigHash,
				UsersHash:       req.UsersHash,
				ConfigUnchanged: true,
				UsersUnchanged:  true,
			}
		} else {
			resp = nextResp
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	dir := t.TempDir()
	client, _ := NewClient(srv.URL, "test-token", srv.Client())
	spool, _ := NewSpool(filepath.Join(dir, "spool"))
	cfgMgr, _ := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    filepath.Join(dir, "config.json"),
		ProbeTimeout:  200 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, &fakeCommandRunner{}, &fakeHealthProber{})
	userSync, _ := NewUserSyncer(&fakeUserController{})

	runner, err := NewRunner(
		RunnerConfig{AgentVersion: "1.2.3", Interval: 20 * time.Second},
		client,
		spool,
		cfgMgr,
		userSync,
		nil,
		NewMetricsCollector(),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	// 1. ConfigJSON + Users applied -> sends normal sync + 1 immediate lightweight ack.
	mu.Lock()
	nextResp = SyncResponse{
		OK:              true,
		DesiredState:    DesiredStateActive,
		SyncIntervalSec: 20,
		ConfigHash:      "cfg-v1",
		UsersHash:       "usr-v1",
		ConfigUnchanged: false,
		UsersUnchanged:  false,
		ConfigJSON:      `{"inbounds":[{"tag":"vless-1"}]}`,
		Users:           []RemoteUser{{InboundTag: "vless-1", Protocol: "vless", Email: "a@example.com", UUID: "u1"}},
	}
	mu.Unlock()

	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce config apply: %v", err)
	}

	mu.Lock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (sync + lightweight ack), got %d", len(requests))
	}
	cfgAck := requests[1]
	requests = nil
	mu.Unlock()

	if cfgAck.HostMetrics != nil || cfgAck.TrafficBatch != nil {
		t.Fatalf("expected nil HostMetrics and TrafficBatch on config ack, got metrics=%v batch=%v", cfgAck.HostMetrics, cfgAck.TrafficBatch)
	}
	if cfgAck.ConfigHash != "cfg-v1" || cfgAck.UsersHash != "usr-v1" || cfgAck.LastError != "" {
		t.Fatalf("unexpected config ack hashes/error: %+v", cfgAck)
	}

	// 2. Differential Users update only -> sends normal sync + 1 immediate lightweight ack.
	mu.Lock()
	nextResp = SyncResponse{
		OK:              true,
		DesiredState:    DesiredStateActive,
		SyncIntervalSec: 20,
		ConfigHash:      "cfg-v1",
		UsersHash:       "usr-v2",
		ConfigUnchanged: true,
		UsersUnchanged:  false,
		Users: []RemoteUser{
			{InboundTag: "vless-1", Protocol: "vless", Email: "a@example.com", UUID: "u1"},
			{InboundTag: "vless-1", Protocol: "vless", Email: "b@example.com", UUID: "u2"},
		},
	}
	mu.Unlock()

	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce users diff: %v", err)
	}

	mu.Lock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (sync + lightweight ack) on users diff, got %d", len(requests))
	}
	usrAck := requests[1]
	mu.Unlock()

	if usrAck.HostMetrics != nil || usrAck.TrafficBatch != nil {
		t.Fatalf("expected nil HostMetrics and TrafficBatch on users ack, got metrics=%v batch=%v", usrAck.HostMetrics, usrAck.TrafficBatch)
	}
	if usrAck.ConfigHash != "cfg-v1" || usrAck.UsersHash != "usr-v2" || usrAck.LastError != "" {
		t.Fatalf("unexpected users ack hashes/error: %+v", usrAck)
	}
}

func TestRunner_OnDemandGeoDataUpdate_NonBlockingFallbackAndCancel(t *testing.T) {
	var mu sync.Mutex
	var syncReqs []SyncRequest
	var cmdResults []CommandResult
	var nextCommands []Command
	failImmediateReport := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/agent/commands/") && strings.HasSuffix(r.URL.Path, "/result") {
			mu.Lock()
			shouldFail := failImmediateReport
			mu.Unlock()
			if shouldFail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"temporary master outage"}`))
				return
			}
			var res CommandResult
			if err := json.NewDecoder(r.Body).Decode(&res); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			cmdResults = append(cmdResults, res)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		var req SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		syncReqs = append(syncReqs, req)
		cmds := nextCommands
		nextCommands = nil
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
			Commands:        cmds,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	client, _ := NewClient(srv.URL, "test-token", srv.Client())
	spool, _ := NewSpool(filepath.Join(dir, "spool"))
	cfgMgr, _ := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    filepath.Join(dir, "config.json"),
		ProbeTimeout:  200 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, &fakeCommandRunner{}, &fakeHealthProber{})
	userSync, _ := NewUserSyncer(&fakeUserController{})

	startedCh := make(chan struct{}, 2)
	releaseCh := make(chan struct{})
	updater := &fakeGeoDataUpdater{
		geoipSize:   10485760,
		geositeSize: 5242880,
		startedCh:   startedCh,
		releaseCh:   releaseCh,
	}

	runner, err := NewRunner(
		RunnerConfig{AgentVersion: "1.2.3", Interval: 20 * time.Second},
		client,
		spool,
		cfgMgr,
		userSync,
		nil,
		NewMetricsCollector(),
	)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	runner.WithGeoDataUpdater(updater)

	// 1. Slow GeoData update MUST NOT block SyncOnce or subsequent heartbeats!
	mu.Lock()
	nextCommands = []Command{{ID: "cmd-geo-1", Type: CommandTypeGeoDataUpdate}}
	mu.Unlock()

	start := time.Now()
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #1: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("SyncOnce #1 blocked on slow GeoData update (%v)", elapsed)
	}

	// Wait until background GeoData goroutine is actively blocked inside Update()
	select {
	case <-startedCh:
	case <-time.After(time.Second):
		t.Fatal("expected background GeoData updater to start")
	}

	// Execute another SyncOnce heartbeat while GeoData download is still blocked
	hbStart := time.Now()
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #2 during slow GeoData failed: %v", err)
	}
	if elapsed := time.Since(hbStart); elapsed > 500*time.Millisecond {
		t.Fatalf("heartbeat SyncOnce #2 blocked during GeoData update (%v)", elapsed)
	}

	// Release slow GeoData update and wait for immediate POST /api/agent/commands/cmd-geo-1/result
	close(releaseCh)
	runner.WaitCommands()

	mu.Lock()
	if len(cmdResults) != 1 {
		t.Fatalf("expected 1 immediate geo command result POST, got %d", len(cmdResults))
	}
	geoRes := cmdResults[0]
	cmdResults = nil
	syncReqs = nil
	mu.Unlock()

	if geoRes.ID != "cmd-geo-1" || geoRes.Status != CommandStatusSucceeded ||
		geoRes.GeoIPSize != 10485760 || geoRes.GeoSiteSize != 5242880 || geoRes.UpdatedAtUnix <= 0 {
		t.Fatalf("unexpected geo command result: %+v", geoRes)
	}

	// 2. When immediate POST /api/agent/commands/:id/result fails, result falls back to next SyncOnce.CommandResults
	updater.mu.Lock()
	updater.releaseCh = nil
	updater.startedCh = nil
	updater.geoipSize = 0
	updater.geositeSize = 0
	updater.err = errors.New("reload/probe failed; rolled back")
	updater.mu.Unlock()

	mu.Lock()
	failImmediateReport = true
	nextCommands = []Command{{ID: "cmd-geo-2", Type: CommandTypeGeoDataUpdate}}
	mu.Unlock()

	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #3: %v", err)
	}
	runner.WaitCommands()

	// Next SyncOnce must carry cmd-geo-2 in CommandResults
	mu.Lock()
	syncReqs = nil
	mu.Unlock()
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #4 (fallback upload): %v", err)
	}
	mu.Lock()
	if len(syncReqs) != 1 || len(syncReqs[0].CommandResults) != 1 {
		t.Fatalf("expected 1 fallback CommandResult in SyncOnce #4, got %+v", syncReqs)
	}
	fallbackRes := syncReqs[0].CommandResults[0]
	syncReqs = nil
	mu.Unlock()
	if fallbackRes.ID != "cmd-geo-2" || fallbackRes.Status != CommandStatusFailed || fallbackRes.Error != "reload/probe failed; rolled back" {
		t.Fatalf("unexpected fallback CommandResult: %+v", fallbackRes)
	}

	// Subsequent SyncOnce after ack must NOT resend cmd-geo-2
	if err := runner.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce #5: %v", err)
	}
	mu.Lock()
	if len(syncReqs) != 1 || len(syncReqs[0].CommandResults) != 0 {
		t.Fatalf("expected 0 CommandResults after ack, got %+v", syncReqs)
	}
	mu.Unlock()

	// 3. Run(ctx) cancellation cleanly aborts in-flight command goroutine without hanging/leaking
	cancelStarted := make(chan struct{}, 1)
	neverRelease := make(chan struct{})
	updater.mu.Lock()
	updater.startedCh = cancelStarted
	updater.releaseCh = neverRelease
	updater.err = nil
	updater.mu.Unlock()

	mu.Lock()
	failImmediateReport = false
	nextCommands = []Command{{ID: "cmd-geo-cancel", Type: CommandTypeGeoDataUpdate}}
	mu.Unlock()

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runner.Run(runCtx)
	}()

	select {
	case <-cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("expected cmd-geo-cancel to start during Run")
	}

	runCancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned unexpected error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit cleanly after ctx cancel (possible goroutine leak)")
	}
}
