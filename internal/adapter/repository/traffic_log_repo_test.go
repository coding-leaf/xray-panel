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
