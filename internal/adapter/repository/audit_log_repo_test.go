package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupAuditTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.AuditLog{}); err != nil {
		t.Fatalf("failed to automigrate audit_logs: %v", err)
	}
	return db
}

func TestGormAuditLogRepository_CRUDAndPrune(t *testing.T) {
	db := setupAuditTestDB(t)
	repo := NewGormAuditLogRepository(db)
	ctx := context.Background()

	// 1. Create
	log1 := &domain.AuditLog{
		CreatedAt: time.Now(),
		Operator:  "admin",
		IP:        "127.0.0.1",
		Action:    domain.ActionUserCreate,
		Target:    "user1@test.com",
		Details:   "created user1",
		Status:    "SUCCESS",
	}
	if err := repo.Create(ctx, log1); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// 2. List
	logs, total, err := repo.List(ctx, 0, 10, "", "", "")
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("expected 1 log, got total=%d, len=%d, err=%v", total, len(logs), err)
	}

	// 3. Filter by Action and Keyword
	logsAction, totalAction, _ := repo.List(ctx, 0, 10, domain.ActionUserCreate, "", "")
	if totalAction != 1 || len(logsAction) != 1 {
		t.Fatalf("expected 1 log with action %s, got %d", domain.ActionUserCreate, totalAction)
	}

	logsKw, totalKw, _ := repo.List(ctx, 0, 10, "", "", "user1")
	if totalKw != 1 || len(logsKw) != 1 {
		t.Fatalf("expected 1 log with keyword 'user1', got %d", totalKw)
	}

	// 4. Prune
	for i := 2; i <= 15; i++ {
		_ = repo.Create(ctx, &domain.AuditLog{
			CreatedAt: time.Now(),
			Operator:  "admin",
			IP:        "127.0.0.1",
			Action:    domain.ActionUserUpdate,
			Target:    fmt.Sprintf("user%d@test.com", i),
			Details:   "updated",
			Status:    "SUCCESS",
		})
	}
	if err := repo.Prune(ctx, 5); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	logsPruned, totalPruned, _ := repo.List(ctx, 0, 20, "", "", "")
	if totalPruned != 5 || len(logsPruned) != 5 {
		t.Fatalf("expected 5 logs after prune to 5, got total=%d, len=%d", totalPruned, len(logsPruned))
	}

	// 5. Clear
	if err := repo.Clear(ctx); err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	_, totalCleared, _ := repo.List(ctx, 0, 10, "", "", "")
	if totalCleared != 0 {
		t.Fatalf("expected 0 logs after clear, got %d", totalCleared)
	}
}
