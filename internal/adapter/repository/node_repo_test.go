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
