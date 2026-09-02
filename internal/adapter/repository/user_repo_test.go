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
	if err := db.AutoMigrate(&domain.User{}); err != nil {
		t.Fatalf("failed to auto-migrate User table: %v", err)
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

