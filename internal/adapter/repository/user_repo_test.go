package repository

import (
	"context"
	"path/filepath"
	"testing"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test_user_repo.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.Inbound{}, &domain.TrafficLog{}); err != nil {
		t.Fatalf("failed to auto-migrate tables: %v", err)
	}
	return db
}

func TestUserRepo_ListByInboundTag_ExactMatch(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u1 := &domain.User{Email: "u1@test.com", UUID: "uuid-1", SubToken: "sub-1", InboundTag: "vless-in", InboundTags: "vless-in,trojan-in"}
	u2 := &domain.User{Email: "u2@test.com", UUID: "uuid-2", SubToken: "sub-2", InboundTag: "vless-in-2", InboundTags: "vless-in-2"}
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("failed to create u1: %v", err)
	}
	if err := repo.Create(ctx, u2); err != nil {
		t.Fatalf("failed to create u2: %v", err)
	}

	list, err := repo.ListByInboundTag(ctx, "vless-in")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 1 || list[0].Email != "u1@test.com" {
		t.Fatalf("expected only u1, got: %v", list)
	}
}

func TestUserRepo_ListByInboundTag_Variations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	// u1: tags with spaces
	u1 := &domain.User{Email: "u1@test.com", UUID: "uuid-1", SubToken: "sub-1", InboundTags: " vless-in , shadowsocks-in "}
	// u2: only fallback InboundTag
	u2 := &domain.User{Email: "u2@test.com", UUID: "uuid-2", SubToken: "sub-2", InboundTag: "vless-in"}
	// u3: prefix match only, should NOT match
	u3 := &domain.User{Email: "u3@test.com", UUID: "uuid-3", SubToken: "sub-3", InboundTags: "vless-in-direct,vless-in2"}
	// u4: non-matching tags
	u4 := &domain.User{Email: "u4@test.com", UUID: "uuid-4", SubToken: "sub-4", InboundTags: "trojan-in"}

	for _, u := range []*domain.User{u1, u2, u3, u4} {
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("failed to create %s: %v", u.Email, err)
		}
	}

	list, err := repo.ListByInboundTag(ctx, "vless-in")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 users (u1 and u2), got %d: %v", len(list), list)
	}

	emails := map[string]bool{}
	for _, u := range list {
		emails[u.Email] = true
	}
	if !emails["u1@test.com"] || !emails["u2@test.com"] {
		t.Fatalf("expected u1 and u2, got %v", emails)
	}
}

func TestUserRepo_UpdateFields(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u := &domain.User{
		Email:      "update@test.com",
		UUID:       "uuid-up",
		SubToken:   "sub-up",
		TotalBytes: 100,
		UpBytes:    500,
		DownBytes:  600,
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	// Update only total_bytes without touching up_bytes or down_bytes
	err := repo.UpdateFields(ctx, u.ID, map[string]interface{}{
		"total_bytes": int64(2000),
	})
	if err != nil {
		t.Fatalf("UpdateFields failed: %v", err)
	}

	fresh, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fresh.TotalBytes != 2000 {
		t.Errorf("expected TotalBytes=2000, got %d", fresh.TotalBytes)
	}
	if fresh.UpBytes != 500 || fresh.DownBytes != 600 {
		t.Errorf("traffic counters modified! UpBytes=%d, DownBytes=%d", fresh.UpBytes, fresh.DownBytes)
	}
}

func TestUserRepo_Update_OmitsTrafficCounters(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u := &domain.User{
		Email:      "user_concurrent@test.com",
		UUID:       "uuid-concurrent",
		SubToken:   "sub-concurrent",
		TotalBytes: 1000,
		UpBytes:    100,
		DownBytes:  200,
	}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	// Fetch user into memory
	loaded, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Concurrent traffic arrives in DB while loaded is in memory
	if err := repo.AddTraffic(ctx, u.Email, 50, 50); err != nil {
		t.Fatalf("AddTraffic failed: %v", err)
	}

	// In memory loaded has old traffic (100, 200). We change another field like TotalBytes
	loaded.TotalBytes = 2000
	if err := repo.Update(ctx, loaded); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	fresh, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fresh.TotalBytes != 2000 {
		t.Errorf("expected TotalBytes=2000, got %d", fresh.TotalBytes)
	}
	// Traffic in DB should be 150, 250, NOT overwritten by loaded's 100, 200!
	if fresh.UpBytes != 150 || fresh.DownBytes != 250 {
		t.Errorf("traffic was overwritten by Update! UpBytes=%d (want 150), DownBytes=%d (want 250)", fresh.UpBytes, fresh.DownBytes)
	}
}

func TestUserRepo_BatchSyncTraffic(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	ctx := context.Background()

	u1 := &domain.User{Email: "batch1@test.com", UUID: "u-batch-1", SubToken: "t-1", UpBytes: 10, DownBytes: 20}
	u2 := &domain.User{Email: "batch2@test.com", UUID: "u-batch-2", SubToken: "t-2", UpBytes: 30, DownBytes: 40}
	if err := repo.Create(ctx, u1); err != nil {
		t.Fatalf("failed to create u1: %v", err)
	}
	if err := repo.Create(ctx, u2); err != nil {
		t.Fatalf("failed to create u2: %v", err)
	}

	inbound := &domain.Inbound{Tag: "vless-in", UpBytes: 100, DownBytes: 200}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	// 第 1 轮批量累加
	userDeltas1 := []domain.UserTrafficDelta{
		{Email: "batch1@test.com", Up: 100, Down: 200},
		{Email: "batch2@test.com", Up: 300, Down: 400},
	}
	inDeltas1 := []domain.InboundTrafficDelta{
		{Tag: "vless-in", Up: 400, Down: 600},
	}

	updated, err := repo.BatchSyncTraffic(ctx, userDeltas1, inDeltas1, "2026-09-18")
	if err != nil {
		t.Fatalf("BatchSyncTraffic round 1 failed: %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("expected 2 updated users, got %d", len(updated))
	}

	// 验证用户在 DB 中的最新值 (10+100=110, 20+200=220)
	freshU1, err := repo.GetByEmail(ctx, "batch1@test.com")
	if err != nil || freshU1.UpBytes != 110 || freshU1.DownBytes != 220 {
		t.Fatalf("unexpected u1 traffic: %+v, err: %v", freshU1, err)
	}

	// 验证 Inbound 在 DB 中的最新值 (100+400=500, 200+600=800)
	var freshInbound domain.Inbound
	if err := db.Where("tag = ?", "vless-in").First(&freshInbound).Error; err != nil || freshInbound.UpBytes != 500 || freshInbound.DownBytes != 800 {
		t.Fatalf("unexpected inbound traffic: %+v, err: %v", freshInbound, err)
	}

	// 验证 TrafficLog 记录
	var log1 domain.TrafficLog
	if err := db.Where("user_email = ? AND date = ?", "batch1@test.com", "2026-09-18").First(&log1).Error; err != nil {
		t.Fatalf("failed to query traffic log for u1: %v", err)
	}
	if log1.UpBytes != 100 || log1.DownBytes != 200 {
		t.Fatalf("unexpected log1 traffic: up=%d down=%d", log1.UpBytes, log1.DownBytes)
	}

	// 第 2 轮批量累加 (同一天累加，测试 TrafficLog upsert)
	userDeltas2 := []domain.UserTrafficDelta{
		{Email: "batch1@test.com", Up: 50, Down: 50},
	}
	inDeltas2 := []domain.InboundTrafficDelta{
		{Tag: "vless-in", Up: 50, Down: 50},
	}
	_, err = repo.BatchSyncTraffic(ctx, userDeltas2, inDeltas2, "2026-09-18")
	if err != nil {
		t.Fatalf("BatchSyncTraffic round 2 failed: %v", err)
	}

	var log1Round2 domain.TrafficLog
	if err := db.Where("user_email = ? AND date = ?", "batch1@test.com", "2026-09-18").First(&log1Round2).Error; err != nil {
		t.Fatalf("failed to query traffic log round 2: %v", err)
	}
	if log1Round2.UpBytes != 150 || log1Round2.DownBytes != 250 {
		t.Fatalf("expected log1 traffic to accumulate to 150/250, got %d/%d", log1Round2.UpBytes, log1Round2.DownBytes)
	}
}


