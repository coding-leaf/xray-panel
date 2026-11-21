package repository

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"panel/internal/domain"
)

func setupNodeTestRepos(t *testing.T) (*GORMNodeRepository, *GORMUserRepository, *GORMInboundRepository, *GormTrafficLogRepository) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "node_repo_test.db")
	db, err := InitSQLite(dbPath)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	return NewNodeRepository(db), NewUserRepository(db), NewInboundRepository(db), NewGormTrafficLogRepository(db)
}

func TestNodeRepo_CRUDAndTokenLookup(t *testing.T) {
	nodeRepo, _, inboundRepo, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node := &domain.Node{
		Name:         "US Direct 1",
		Host:         "127.0.0.1",
		Region:       "US",
		TokenHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TokenPrefix:  "12345678",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
		Remark:       "test node",
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("Create node failed: %v", err)
	}
	if node.ID == 0 {
		t.Fatalf("expected non-zero node ID")
	}

	got, err := nodeRepo.GetByTokenHash(ctx, node.TokenHash)
	if err != nil {
		t.Fatalf("GetByTokenHash failed: %v", err)
	}
	if got.ID != node.ID || got.Name != "US Direct 1" {
		t.Fatalf("unexpected node from GetByTokenHash: %+v", got)
	}

	// Verify FindByNodeID & CountInbounds
	if err := inboundRepo.Create(ctx, &domain.Inbound{
		NodeID:   node.ID,
		Tag:      "us-vless",
		Port:     443,
		Protocol: "vless",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("Create remote inbound failed: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{
		NodeID:   0,
		Tag:      "local-vless",
		Port:     8443,
		Protocol: "vless",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("Create local inbound failed: %v", err)
	}

	remoteInbounds, err := inboundRepo.FindByNodeID(ctx, node.ID)
	if err != nil {
		t.Fatalf("FindByNodeID failed: %v", err)
	}
	if len(remoteInbounds) != 1 || remoteInbounds[0].Tag != "us-vless" {
		t.Fatalf("expected 1 remote inbound us-vless, got %+v", remoteInbounds)
	}

	count, err := nodeRepo.CountInbounds(ctx, node.ID)
	if err != nil || count != 1 {
		t.Fatalf("expected inbound count 1, got %d (err=%v)", count, err)
	}

	// Update metadata must not overwrite heartbeat/traffic counters
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:   "cfg-1",
		UsersHash:    "usr-1",
		AgentVersion: "0.1.0",
		XrayVersion:  "25.1.1",
		HostMetrics: &domain.AgentHostMetrics{
			CPUPercent:   15.5,
			MemPercent:   42.0,
			NicUpDelta:   1000,
			NicDownDelta: 2000,
		},
	}, time.Now()); err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	loaded, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	loaded.Name = "US Direct Updated"
	loaded.NicUpBytes = 0 // stale zero should not overwrite DB counter
	if err := nodeRepo.Update(ctx, loaded); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	afterUpdate, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID after update failed: %v", err)
	}
	if afterUpdate.Name != "US Direct Updated" {
		t.Fatalf("expected updated name, got %q", afterUpdate.Name)
	}
	if afterUpdate.NicUpBytes != 1000 || afterUpdate.NicDownBytes != 2000 {
		t.Fatalf("expected NIC counters preserved (1000/2000), got %d/%d", afterUpdate.NicUpBytes, afterUpdate.NicDownBytes)
	}

	// Missing node returns ErrNotFound
	_, err = nodeRepo.GetByID(ctx, 9999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNodeRepo_ApplyNodeTrafficBatch_IdempotentAndIsolated(t *testing.T) {
	nodeRepo, userRepo, inboundRepo, trafficRepo := setupNodeTestRepos(t)
	ctx := context.Background()

	node1 := &domain.Node{
		Name: "Node 1", Host: "127.0.0.1",
		TokenHash: "1111111111111111111111111111111111111111111111111111111111111111", Enabled: true, DesiredState: domain.NodeStateActive,
	}
	node2 := &domain.Node{
		Name: "Node 2", Host: "127.0.0.1",
		TokenHash: "2222222222222222222222222222222222222222222222222222222222222222", Enabled: true, DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, node1); err != nil {
		t.Fatalf("create node1: %v", err)
	}
	if err := nodeRepo.Create(ctx, node2); err != nil {
		t.Fatalf("create node2: %v", err)
	}

	user := &domain.User{
		UUID: "11111111-1111-1111-1111-111111111111", Email: "alice@example.com", SubToken: "sub-alice-1", InboundTags: "n1-vless", Enabled: true,
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{NodeID: node1.ID, Tag: "n1-vless", Port: 443, Protocol: "vless", Enabled: true}); err != nil {
		t.Fatalf("create inbound n1: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{NodeID: node2.ID, Tag: "n2-vless", Port: 444, Protocol: "vless", Enabled: true}); err != nil {
		t.Fatalf("create inbound n2: %v", err)
	}

	batch := &domain.AgentTrafficBatch{
		BatchID: "batch-001",
		Users: []domain.AgentUserTrafficDelta{
			{Email: "alice@example.com", UpBytes: 1000, DownBytes: 4000},
		},
		Inbounds: []domain.AgentInboundTrafficDelta{
			{Tag: "n1-vless", UpBytes: 1000, DownBytes: 4000},
			// Spoofed inbound belonging to node2 must be ignored for node1
			{Tag: "n2-vless", UpBytes: 99999, DownBytes: 99999},
		},
	}

	date := "2026-11-19"
	applied, affected, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node1.ID, batch, date)
	if err != nil {
		t.Fatalf("first ApplyNodeTrafficBatch failed: %v", err)
	}
	if !applied || len(affected) != 1 {
		t.Fatalf("expected applied=true and 1 affected user, got applied=%v len=%d", applied, len(affected))
	}

	// Retry exact same batchId -> must be idempotent no-op
	applied2, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node1.ID, batch, date)
	if err != nil {
		t.Fatalf("second ApplyNodeTrafficBatch failed: %v", err)
	}
	if applied2 {
		t.Fatalf("expected duplicate batchId to return applied=false")
	}

	// Verify totals incremented exactly once and node2 inbound was untouched
	uAfter, _ := userRepo.GetByEmail(ctx, "alice@example.com")
	if uAfter.UpBytes != 1000 || uAfter.DownBytes != 4000 {
		t.Fatalf("expected user traffic 1000/4000, got %d/%d", uAfter.UpBytes, uAfter.DownBytes)
	}
	ib1After, _ := inboundRepo.GetByTag(ctx, "n1-vless")
	if ib1After.UpBytes != 1000 || ib1After.DownBytes != 4000 {
		t.Fatalf("expected n1-vless traffic 1000/4000, got %d/%d", ib1After.UpBytes, ib1After.DownBytes)
	}
	ib2After, _ := inboundRepo.GetByTag(ctx, "n2-vless")
	if ib2After.UpBytes != 0 || ib2After.DownBytes != 0 {
		t.Fatalf("expected n2-vless traffic 0/0 (cross-node isolation), got %d/%d", ib2After.UpBytes, ib2After.DownBytes)
	}
	n1After, _ := nodeRepo.GetByID(ctx, node1.ID)
	if n1After.XrayUpBytes != 1000 || n1After.XrayDownBytes != 4000 {
		t.Fatalf("expected node1 xray traffic 1000/4000, got %d/%d", n1After.XrayUpBytes, n1After.XrayDownBytes)
	}
	logs, err := trafficRepo.GetHistoryByEmail(ctx, "alice@example.com", 7)
	if err != nil || len(logs) != 1 || logs[0].UpBytes != 1000 || logs[0].DownBytes != 4000 {
		t.Fatalf("expected 1 traffic log 1000/4000, got %+v (err=%v)", logs, err)
	}

	// Same batchId on node2 IS distinct because unique index is (node_id, batch_id)
	batchNode2 := &domain.AgentTrafficBatch{
		BatchID: "batch-001",
		Inbounds: []domain.AgentInboundTrafficDelta{
			{Tag: "n2-vless", UpBytes: 500, DownBytes: 600},
		},
	}
	appliedN2, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node2.ID, batchNode2, date)
	if err != nil || !appliedN2 {
		t.Fatalf("expected same batchId on node2 to apply cleanly, got applied=%v err=%v", appliedN2, err)
	}
}

func TestNodeRepo_ApplyNodeTrafficBatch_ConcurrentSameBatch(t *testing.T) {
	nodeRepo, userRepo, inboundRepo, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node := &domain.Node{
		Name: "Concurrent Node", Host: "127.0.0.1",
		TokenHash: "3333333333333333333333333333333333333333333333333333333333333333", Enabled: true, DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := userRepo.Create(ctx, &domain.User{
		UUID: "22222222-2222-2222-2222-222222222222", Email: "bob@example.com", SubToken: "sub-bob-1", InboundTags: "c-vless", Enabled: true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{NodeID: node.ID, Tag: "c-vless", Port: 443, Protocol: "vless", Enabled: true}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	const workers = 6
	var wg sync.WaitGroup
	appliedCount := 0
	var mu sync.Mutex
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node.ID, &domain.AgentTrafficBatch{
				BatchID:  "concurrent-batch-1",
				Users:    []domain.AgentUserTrafficDelta{{Email: "bob@example.com", UpBytes: 250, DownBytes: 750}},
				Inbounds: []domain.AgentInboundTrafficDelta{{Tag: "c-vless", UpBytes: 250, DownBytes: 750}},
			}, "2026-11-19")
			if err != nil {
				t.Errorf("concurrent ApplyNodeTrafficBatch error: %v", err)
				return
			}
			if applied {
				mu.Lock()
				appliedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if appliedCount != 1 {
		t.Fatalf("expected exactly 1 applied goroutine, got %d", appliedCount)
	}
	uAfter, _ := userRepo.GetByEmail(ctx, "bob@example.com")
	if uAfter.UpBytes != 250 || uAfter.DownBytes != 750 {
		t.Fatalf("expected bob traffic 250/750, got %d/%d", uAfter.UpBytes, uAfter.DownBytes)
	}
}

func TestNodeRepo_QuotaCycleDailyAndReset(t *testing.T) {
	nodeRepo, _, inboundRepo, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node := &domain.Node{
		Name:              "Quota Node",
		Host:              "127.0.0.1",
		TokenHash:         "4444444444444444444444444444444444444444444444444444444444444444",
		Enabled:           true,
		DesiredState:      domain.NodeStateActive,
		TrafficLimitBytes: 10000,
		BillingResetDay:   15,
		TrafficCountMode:  domain.TrafficCountModeBoth,
		TrafficMultiplier: 1.0,
		WarnThresholdPct:  80,
		DrainThresholdPct: 90,
		BreakThresholdPct: 95,
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{
		NodeID: node.ID, Tag: "q-vless", Port: 443, Protocol: "vless", Enabled: true,
	}); err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	day1 := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	// NIC heartbeat deltas accumulate both lifetime and cycle counters and daily logs
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 1000, NicDownDelta: 2000},
	}, day1); err != nil {
		t.Fatalf("UpdateHeartbeat day1 failed: %v", err)
	}
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 500, NicDownDelta: 600},
	}, day2); err != nil {
		t.Fatalf("UpdateHeartbeat day2 failed: %v", err)
	}

	// Xray batch accumulates cycle_xray_* and daily logs (idempotent on repeat)
	batch := &domain.AgentTrafficBatch{
		BatchID:  "quota-batch-1",
		Inbounds: []domain.AgentInboundTrafficDelta{{Tag: "q-vless", UpBytes: 300, DownBytes: 700}},
	}
	if applied, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node.ID, batch, "2026-11-19"); err != nil || !applied {
		t.Fatalf("ApplyNodeTrafficBatch failed: applied=%v err=%v", applied, err)
	}
	if applied2, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node.ID, batch, "2026-11-19"); err != nil || applied2 {
		t.Fatalf("duplicate ApplyNodeTrafficBatch should be no-op: applied=%v err=%v", applied2, err)
	}

	loaded, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if loaded.CycleNicUpBytes != 1500 || loaded.CycleNicDownBytes != 2600 {
		t.Fatalf("expected cycle NIC 1500/2600, got %d/%d", loaded.CycleNicUpBytes, loaded.CycleNicDownBytes)
	}
	if loaded.CycleXrayUpBytes != 300 || loaded.CycleXrayDownBytes != 700 {
		t.Fatalf("expected cycle Xray 300/700, got %d/%d", loaded.CycleXrayUpBytes, loaded.CycleXrayDownBytes)
	}

	// Update metadata must not overwrite cycle counters
	loaded.Name = "Quota Node Renamed"
	loaded.CycleNicUpBytes = 0
	if err := nodeRepo.Update(ctx, loaded); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	afterUpdate, _ := nodeRepo.GetByID(ctx, node.ID)
	if afterUpdate.CycleNicUpBytes != 1500 {
		t.Fatalf("Update overwrote cycle_nic_up_bytes: got %d", afterUpdate.CycleNicUpBytes)
	}

	// Verify daily logs ordered newest first
	dailies, err := nodeRepo.ListRecentDailyTraffic(ctx, node.ID, 2)
	if err != nil || len(dailies) != 2 {
		t.Fatalf("expected 2 daily logs, got %+v (err=%v)", dailies, err)
	}
	if dailies[0].Date != "2026-11-19" || dailies[0].NicUpBytes != 500 || dailies[0].XrayUpBytes != 300 {
		t.Fatalf("unexpected newest daily log: %+v", dailies[0])
	}
	if dailies[1].Date != "2026-11-18" || dailies[1].NicUpBytes != 1000 {
		t.Fatalf("unexpected older daily log: %+v", dailies[1])
	}

	// Auto state transition & reset recovery
	if err := nodeRepo.UpdateAutoState(ctx, node.ID, domain.NodeStateCircuitBroken, domain.AutoReasonBreakQuota); err != nil {
		t.Fatalf("UpdateAutoState failed: %v", err)
	}
	if err := nodeRepo.ResetCycleTraffic(ctx, node.ID, 202611, true); err != nil {
		t.Fatalf("ResetCycleTraffic failed: %v", err)
	}
	afterReset, _ := nodeRepo.GetByID(ctx, node.ID)
	if afterReset.CycleNicUpBytes != 0 || afterReset.CycleXrayDownBytes != 0 {
		t.Fatalf("expected cycle counters zeroed, got %+v", afterReset)
	}
	if afterReset.LastResetMonth != 202611 {
		t.Fatalf("expected LastResetMonth 202611, got %d", afterReset.LastResetMonth)
	}
	if afterReset.DesiredState != domain.NodeStateActive || afterReset.AutoStateReason != "" {
		t.Fatalf("expected auto-degraded state restored to active, got state=%q reason=%q", afterReset.DesiredState, afterReset.AutoStateReason)
	}

	// Manual state must NOT be overridden by ResetCycleTraffic
	if err := nodeRepo.UpdateAutoState(ctx, node.ID, domain.NodeStateDraining, domain.AutoReasonManual); err != nil {
		t.Fatalf("UpdateAutoState manual failed: %v", err)
	}
	if err := nodeRepo.ResetCycleTraffic(ctx, node.ID, 202612, true); err != nil {
		t.Fatalf("ResetCycleTraffic manual failed: %v", err)
	}
	afterManualReset, _ := nodeRepo.GetByID(ctx, node.ID)
	if afterManualReset.DesiredState != domain.NodeStateDraining || afterManualReset.AutoStateReason != domain.AutoReasonManual {
		t.Fatalf("manual state was overwritten on reset: state=%q reason=%q", afterManualReset.DesiredState, afterManualReset.AutoStateReason)
	}
}

func TestNodeRepo_UpdateNodeConfig_WhitelistAndIsolation(t *testing.T) {
	nodeRepo, _, _, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node := &domain.Node{
		Name:         "Custom Config Node",
		Host:         "127.0.0.1",
		TokenHash:    "5555555555555555555555555555555555555555555555555555555555555555",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("create node failed: %v", err)
	}

	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:   "hash-cfg",
		UsersHash:    "hash-usr",
		AgentVersion: "1.0.0",
		XrayVersion:  "25.1.1",
		HostMetrics: &domain.AgentHostMetrics{
			CPUPercent:   33.5,
			MemPercent:   61.2,
			NicUpDelta:   1234,
			NicDownDelta: 5678,
		},
	}, time.Now()); err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	outboundsJSON := `[{"tag":"warp-out","protocol":"wireguard","settingsJson":"{}"}]`
	routingJSON := `{"domainStrategy":"IPIfNonMatch","rules":[{"type":"field","domain":["geosite:openai"],"outboundTag":"warp-out"}]}`
	dnsJSON := `{"servers":["1.1.1.1"],"queryStrategy":"UseIPv4"}`

	if err := nodeRepo.UpdateNodeConfig(ctx, node.ID, outboundsJSON, routingJSON, dnsJSON); err != nil {
		t.Fatalf("UpdateNodeConfig failed: %v", err)
	}

	loaded, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if loaded.OutboundsJSON != outboundsJSON || loaded.RoutingJSON != routingJSON || loaded.DNSJSON != dnsJSON {
		t.Fatalf("per-node config not persisted accurately: %+v", loaded)
	}
	if loaded.NicUpBytes != 1234 || loaded.NicDownBytes != 5678 || loaded.CPUPercent != 33.5 {
		t.Fatalf("UpdateNodeConfig overwrote heartbeat counters: %+v", loaded)
	}

	// Regular metadata Update must not overwrite outbounds/routing/dns
	loaded.Name = "Renamed Config Node"
	loaded.OutboundsJSON = ""
	loaded.RoutingJSON = ""
	loaded.DNSJSON = ""
	if err := nodeRepo.Update(ctx, loaded); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	afterMetaUpdate, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID after Update failed: %v", err)
	}
	if afterMetaUpdate.OutboundsJSON != outboundsJSON || afterMetaUpdate.RoutingJSON != routingJSON || afterMetaUpdate.DNSJSON != dnsJSON {
		t.Fatalf("Update overwrote per-node config blobs")
	}

	// Heartbeat with nil HostMetrics (e.g. lightweight config-apply ack) must preserve existing CPU/NIC metrics
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:   "hash-cfg",
		UsersHash:    "hash-usr",
		AgentVersion: "1.0.0",
		XrayVersion:  "25.1.1",
	}, time.Now()); err != nil {
		t.Fatalf("lightweight UpdateHeartbeat failed: %v", err)
	}
	afterLogAck, _ := nodeRepo.GetByID(ctx, node.ID)
	if afterLogAck.CPUPercent != 33.5 || afterLogAck.MemPercent != 61.2 || afterLogAck.NicUpBytes != 1234 {
		t.Fatalf("lightweight ack overwrote host metrics: %+v", afterLogAck)
	}

	if err := nodeRepo.UpdateNodeConfig(ctx, 999999, outboundsJSON, routingJSON, dnsJSON); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing node, got %v", err)
	}
}

func TestNodeRepo_UpdateHeartbeat_ConfigAckPreservesHostMetrics(t *testing.T) {
	nodeRepo, _, _, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node := &domain.Node{
		Name:         "Ack Isolation Node",
		Host:         "127.0.0.1",
		TokenHash:    "6666666666666666666666666666666666666666666666666666666666666666",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, node); err != nil {
		t.Fatalf("create node failed: %v", err)
	}

	// Seed baseline metrics and a prior sync error
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:   "cfg-old",
		UsersHash:    "usr-old",
		AgentVersion: "1.0.0",
		XrayVersion:  "25.1.1",
		LastError:    "previous reload failed",
		HostMetrics: &domain.AgentHostMetrics{
			CPUPercent:   42.5,
			MemPercent:   68.0,
			NicUpDelta:   2048,
			NicDownDelta: 4096,
		},
	}, time.Now()); err != nil {
		t.Fatalf("seed UpdateHeartbeat failed: %v", err)
	}

	// Lightweight config-apply ack (HostMetrics == nil, TrafficBatch == nil)
	// MUST update last_config_hash/last_users_hash and clear old last_sync_error while preserving CPU/RAM/NIC counters.
	if err := nodeRepo.UpdateHeartbeat(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:   "cfg-new",
		UsersHash:    "usr-new",
		AgentVersion: "1.0.1",
		XrayVersion:  "25.1.2",
		LastError:    "",
	}, time.Now()); err != nil {
		t.Fatalf("config-apply lightweight ack failed: %v", err)
	}
	afterConfigAck, err := nodeRepo.GetByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("GetByID after config ack failed: %v", err)
	}
	if afterConfigAck.LastConfigHash != "cfg-new" || afterConfigAck.LastUsersHash != "usr-new" {
		t.Fatalf("config-apply ack did not update hashes: cfg=%q usr=%q",
			afterConfigAck.LastConfigHash, afterConfigAck.LastUsersHash)
	}
	if afterConfigAck.LastSyncError != "" {
		t.Fatalf("config-apply ack did not clear last_sync_error: got %q", afterConfigAck.LastSyncError)
	}
	if afterConfigAck.CPUPercent != 42.5 || afterConfigAck.MemPercent != 68.0 || afterConfigAck.NicUpBytes != 2048 || afterConfigAck.NicDownBytes != 4096 {
		t.Fatalf("config-apply ack overwrote host metrics: %+v", afterConfigAck)
	}
}

func TestNodeRepo_Delete_CascadesTrafficTablesAndReturnsErrNotFound(t *testing.T) {
	nodeRepo, _, inboundRepo, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	node1 := &domain.Node{
		Name:         "Cascade Node 1",
		Host:         "127.0.0.1",
		TokenHash:    "7777777777777777777777777777777777777777777777777777777777777771",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
	}
	node2 := &domain.Node{
		Name:         "Cascade Node 2",
		Host:         "127.0.0.2",
		TokenHash:    "7777777777777777777777777777777777777777777777777777777777777772",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, node1); err != nil {
		t.Fatalf("create node1 failed: %v", err)
	}
	if err := nodeRepo.Create(ctx, node2); err != nil {
		t.Fatalf("create node2 failed: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{NodeID: node1.ID, Tag: "c1-in", Port: 10001, Protocol: "vless", Enabled: true}); err != nil {
		t.Fatalf("create inbound1 failed: %v", err)
	}
	if err := inboundRepo.Create(ctx, &domain.Inbound{NodeID: node2.ID, Tag: "c2-in", Port: 10002, Protocol: "vless", Enabled: true}); err != nil {
		t.Fatalf("create inbound2 failed: %v", err)
	}

	now := time.Now()
	if _, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node1.ID, &domain.AgentTrafficBatch{
		BatchID:  "cascade-b1",
		Inbounds: []domain.AgentInboundTrafficDelta{{Tag: "c1-in", UpBytes: 100, DownBytes: 200}},
	}, now.Format("2006-01-02")); err != nil {
		t.Fatalf("apply batch node1 failed: %v", err)
	}
	if _, _, err := nodeRepo.ApplyNodeTrafficBatch(ctx, node2.ID, &domain.AgentTrafficBatch{
		BatchID:  "cascade-b2",
		Inbounds: []domain.AgentInboundTrafficDelta{{Tag: "c2-in", UpBytes: 300, DownBytes: 400}},
	}, now.Format("2006-01-02")); err != nil {
		t.Fatalf("apply batch node2 failed: %v", err)
	}
	if err := nodeRepo.UpdateHeartbeat(ctx, node1.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 10, NicDownDelta: 20},
	}, now); err != nil {
		t.Fatalf("update heartbeat node1 failed: %v", err)
	}

	// Delete node1 -> must remove its batches & dailies while leaving node2 intact
	if err := nodeRepo.Delete(ctx, node1.ID); err != nil {
		t.Fatalf("Delete node1 failed: %v", err)
	}

	var n1Batches, n1Dailies, n2Batches, n2Dailies int64
	nodeRepo.db.Model(&domain.NodeTrafficBatch{}).Where("node_id = ?", node1.ID).Count(&n1Batches)
	nodeRepo.db.Model(&domain.NodeTrafficDaily{}).Where("node_id = ?", node1.ID).Count(&n1Dailies)
	nodeRepo.db.Model(&domain.NodeTrafficBatch{}).Where("node_id = ?", node2.ID).Count(&n2Batches)
	nodeRepo.db.Model(&domain.NodeTrafficDaily{}).Where("node_id = ?", node2.ID).Count(&n2Dailies)

	if n1Batches != 0 || n1Dailies != 0 {
		t.Fatalf("expected 0 orphan traffic rows for deleted node1, got batches=%d dailies=%d", n1Batches, n1Dailies)
	}
	if n2Batches != 1 || n2Dailies != 1 {
		t.Fatalf("expected node2 traffic rows untouched (1/1), got batches=%d dailies=%d", n2Batches, n2Dailies)
	}

	// Deleting missing node must return ErrNotFound
	if err := nodeRepo.Delete(ctx, 999999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting missing node, got %v", err)
	}
}

func TestNodeRepo_DirectSubEnabledRoundTrip(t *testing.T) {
	nodeRepo, _, _, _ := setupNodeTestRepos(t)
	ctx := context.Background()

	falseVal := false
	trueVal := true

	// 1. Explicit false must persist as false despite gorm default:true
	relayOnly := &domain.Node{
		Name:             "GCP Relay Exit",
		Host:             "127.0.0.1",
		Region:           "SG",
		TokenHash:        "8888888888888888888888888888888888888888888888888888888888888881",
		Enabled:          true,
		DirectSubEnabled: &falseVal,
		DesiredState:     domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, relayOnly); err != nil {
		t.Fatalf("create relay-only node failed: %v", err)
	}
	got, err := nodeRepo.GetByID(ctx, relayOnly.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.DirectSubEnabled == nil || *got.DirectSubEnabled || got.IsDirectSubEnabled() {
		t.Fatalf("expected DirectSubEnabled=false persisted, got ptr=%v val=%v", got.DirectSubEnabled, got.IsDirectSubEnabled())
	}

	// 2. Update with nil DirectSubEnabled preserves existing false
	got.DirectSubEnabled = nil
	got.Name = "GCP Relay Exit Renamed"
	if err := nodeRepo.Update(ctx, got); err != nil {
		t.Fatalf("Update with nil DirectSubEnabled failed: %v", err)
	}
	afterNilUpdate, err := nodeRepo.GetByID(ctx, relayOnly.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if afterNilUpdate.IsDirectSubEnabled() {
		t.Fatalf("expected DirectSubEnabled to remain false when nil passed to Update")
	}

	// 3. Explicit toggle true -> false
	afterNilUpdate.DirectSubEnabled = &trueVal
	if err := nodeRepo.Update(ctx, afterNilUpdate); err != nil {
		t.Fatalf("Update to true failed: %v", err)
	}
	afterTrue, _ := nodeRepo.GetByID(ctx, relayOnly.ID)
	if !afterTrue.IsDirectSubEnabled() {
		t.Fatalf("expected DirectSubEnabled=true after explicit update")
	}

	afterTrue.DirectSubEnabled = &falseVal
	if err := nodeRepo.Update(ctx, afterTrue); err != nil {
		t.Fatalf("Update back to false failed: %v", err)
	}
	afterFalse, _ := nodeRepo.GetByID(ctx, relayOnly.ID)
	if afterFalse.IsDirectSubEnabled() {
		t.Fatalf("expected DirectSubEnabled=false after explicit update back to false")
	}

	// 4. Omitted DirectSubEnabled defaults to true in DB
	defaultNode := &domain.Node{
		Name:         "Default Direct Node",
		Host:         "127.0.0.1",
		TokenHash:    "8888888888888888888888888888888888888888888888888888888888888882",
		Enabled:      true,
		DesiredState: domain.NodeStateActive,
	}
	if err := nodeRepo.Create(ctx, defaultNode); err != nil {
		t.Fatalf("create default node failed: %v", err)
	}
	gotDef, _ := nodeRepo.GetByID(ctx, defaultNode.ID)
	if !gotDef.IsDirectSubEnabled() {
		t.Fatalf("expected omitted DirectSubEnabled to default to true")
	}
}
