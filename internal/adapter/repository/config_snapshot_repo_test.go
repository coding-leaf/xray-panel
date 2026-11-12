package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupSnapshotTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_snapshot.db")
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
	if err := db.AutoMigrate(&domain.ConfigSnapshot{}); err != nil {
		t.Fatalf("failed to auto-migrate ConfigSnapshot table: %v", err)
	}
	return db
}

func TestConfigSnapshotRepo_PruneAndAutoPrune(t *testing.T) {
	ctx := context.Background()

	t.Run("Prune keeps newest maxKeep snapshots", func(t *testing.T) {
		db := setupSnapshotTestDB(t)
		repo := NewGormConfigSnapshotRepository(db)

		for i := 1; i <= 15; i++ {
			if err := db.WithContext(ctx).Create(&domain.ConfigSnapshot{
				RawConfig: fmt.Sprintf(`{"n":%d}`, i),
				Remark:    fmt.Sprintf("snap-%d", i),
			}).Error; err != nil {
				t.Fatalf("seed snapshot %d failed: %v", i, err)
			}
		}

		// maxKeep >= total should delete nothing
		if err := repo.Prune(ctx, 20); err != nil {
			t.Fatalf("Prune(20) failed: %v", err)
		}
		var count int64
		db.Model(&domain.ConfigSnapshot{}).Count(&count)
		if count != 15 {
			t.Fatalf("expected 15 snapshots when maxKeep=20, got %d", count)
		}

		// maxKeep = 5 should keep IDs 11..15
		if err := repo.Prune(ctx, 5); err != nil {
			t.Fatalf("Prune(5) failed: %v", err)
		}
		list, err := repo.List(ctx, 20)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(list) != 5 {
			t.Fatalf("expected 5 snapshots after Prune(5), got %d", len(list))
		}
		if list[0].ID != 15 || list[4].ID != 11 {
			t.Fatalf("expected newest IDs 15..11, got first=%d last=%d", list[0].ID, list[4].ID)
		}
	})

	t.Run("Save auto-prunes to <= 100 snapshots", func(t *testing.T) {
		db := setupSnapshotTestDB(t)
		repo := NewGormConfigSnapshotRepository(db)

		const total = 120
		for i := 1; i <= total; i++ {
			if err := repo.Save(ctx, &domain.ConfigSnapshot{
				RawConfig: fmt.Sprintf(`{"n":%d}`, i),
				Remark:    fmt.Sprintf("auto-%d", i),
			}); err != nil {
				t.Fatalf("Save snapshot %d failed: %v", i, err)
			}
		}

		var count int64
		db.Model(&domain.ConfigSnapshot{}).Count(&count)
		if count != snapshotMaxKeep {
			t.Fatalf("expected %d snapshots after auto-prune, got %d", snapshotMaxKeep, count)
		}

		list, err := repo.List(ctx, 200)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(list) != snapshotMaxKeep {
			t.Fatalf("expected %d listed snapshots, got %d", snapshotMaxKeep, len(list))
		}
		if list[0].ID != total || list[len(list)-1].ID != total-snapshotMaxKeep+1 {
			t.Fatalf("expected newest IDs %d..%d, got %d..%d", total, total-snapshotMaxKeep+1, list[0].ID, list[len(list)-1].ID)
		}
	})
}
