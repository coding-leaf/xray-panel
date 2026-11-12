package repository

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTrafficTestDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test_traffic_log.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	if err := db.AutoMigrate(&domain.TrafficLog{}); err != nil {
		t.Fatalf("failed to auto-migrate TrafficLog table: %v", err)
	}
	return db
}

func TestTrafficLogRepo_CompositeUniqueIndexNoDuplicates(t *testing.T) {
	db := setupTrafficTestDB(t)
	repo := NewGormTrafficLogRepository(db)
	ctx := context.Background()

	email := "testuser@example.com"
	date := "2026-09-03"

	// 1. Initial write
	err := repo.RecordTraffic(ctx, 1, email, 1000, 2000, date)
	if err != nil {
		t.Fatalf("first RecordTraffic failed: %v", err)
	}

	// 2. Incremental write on same date
	err = repo.RecordTraffic(ctx, 1, email, 500, 500, date)
	if err != nil {
		t.Fatalf("second RecordTraffic failed: %v", err)
	}

	// Verify only 1 row exists
	var count int64
	db.Model(&domain.TrafficLog{}).Where("user_email = ? AND date = ?", email, date).Count(&count)
	if count != 1 {
		t.Fatalf("expected exactly 1 row for %s on %s, got %d rows!", email, date, count)
	}

	var log domain.TrafficLog
	db.Where("user_email = ? AND date = ?", email, date).First(&log)
	if log.UpBytes != 1500 || log.DownBytes != 2500 {
		t.Errorf("expected UpBytes=1500, DownBytes=2500, got UpBytes=%d, DownBytes=%d", log.UpBytes, log.DownBytes)
	}

	// 3. Concurrent writes on same date
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.RecordTraffic(ctx, 1, email, 100, 100, date)
		}()
	}
	wg.Wait()

	db.Model(&domain.TrafficLog{}).Where("user_email = ? AND date = ?", email, date).Count(&count)
	if count != 1 {
		t.Fatalf("concurrent writes created duplicate rows! Total rows: %d", count)
	}
}

func TestUserRuntimeSpeed_Lifecycle(t *testing.T) {
	email := "speed_test@example.com"
	domain.SetUserRuntimeSpeed(email, 1024, 2048, 1234567890)

	speeds := domain.GetAllUserRuntimeSpeeds()
	if _, ok := speeds[email]; !ok {
		t.Fatalf("expected user %s to exist in runtime speeds", email)
	}

	// Test removal
	domain.RemoveUserRuntimeSpeed(email)

	speedsAfter := domain.GetAllUserRuntimeSpeeds()
	if _, ok := speedsAfter[email]; ok {
		t.Fatalf("expected user %s to be completely removed from runtime speeds", email)
	}
}

func TestTrafficLogRepo_DeleteByUserAndPruneBefore(t *testing.T) {
	db := setupTrafficTestDB(t)
	repo := NewGormTrafficLogRepository(db)
	ctx := context.Background()

	// Seed logs for two users across multiple dates
	if err := repo.RecordTraffic(ctx, 10, "alice@example.com", 100, 200, "2025-01-01"); err != nil {
		t.Fatalf("RecordTraffic failed: %v", err)
	}
	if err := repo.RecordTraffic(ctx, 99, "alice@example.com", 300, 400, "2026-06-01"); err != nil {
		t.Fatalf("RecordTraffic failed: %v", err)
	}
	if err := repo.RecordTraffic(ctx, 10, "alice-old@example.com", 500, 600, "2026-06-02"); err != nil {
		t.Fatalf("RecordTraffic failed: %v", err)
	}
	if err := repo.RecordTraffic(ctx, 20, "bob@example.com", 700, 800, "2025-12-31"); err != nil {
		t.Fatalf("RecordTraffic failed: %v", err)
	}
	if err := repo.RecordTraffic(ctx, 20, "bob@example.com", 900, 1000, "2026-01-01"); err != nil {
		t.Fatalf("RecordTraffic failed: %v", err)
	}

	// DeleteByUser(10, "alice@example.com") should delete all 3 rows matching user_id=10 OR email="alice@example.com"
	if err := repo.DeleteByUser(ctx, 10, "alice@example.com"); err != nil {
		t.Fatalf("DeleteByUser failed: %v", err)
	}

	aliceHistory, err := repo.GetHistoryByEmail(ctx, "alice@example.com", 30)
	if err != nil {
		t.Fatalf("GetHistoryByEmail failed: %v", err)
	}
	if len(aliceHistory) != 0 {
		t.Fatalf("expected 0 logs for alice@example.com, got %d", len(aliceHistory))
	}

	var aliceOldCount int64
	db.Model(&domain.TrafficLog{}).Where("user_email = ?", "alice-old@example.com").Count(&aliceOldCount)
	if aliceOldCount != 0 {
		t.Fatalf("expected user_id=10 log to be deleted too, got %d", aliceOldCount)
	}

	// Bob's 2 logs must remain intact
	bobHistory, err := repo.GetHistoryByEmail(ctx, "bob@example.com", 30)
	if err != nil {
		t.Fatalf("GetHistoryByEmail bob failed: %v", err)
	}
	if len(bobHistory) != 2 {
		t.Fatalf("expected 2 logs for bob, got %d", len(bobHistory))
	}

	// PruneBefore("2026-01-01") should delete "2025-12-31" and preserve "2026-01-01"
	if err := repo.PruneBefore(ctx, "2026-01-01"); err != nil {
		t.Fatalf("PruneBefore failed: %v", err)
	}
	bobAfterPrune, err := repo.GetHistoryByEmail(ctx, "bob@example.com", 30)
	if err != nil {
		t.Fatalf("GetHistoryByEmail bob after prune failed: %v", err)
	}
	if len(bobAfterPrune) != 1 || bobAfterPrune[0].Date != "2026-01-01" {
		t.Fatalf("expected only 2026-01-01 log to remain for bob, got %+v", bobAfterPrune)
	}
}
