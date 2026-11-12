package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSQLite_PragmasAndSingleConn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panel.db")
	db, err := InitSQLite(dbPath)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if got := sqlDB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("expected MaxOpenConnections=1, got %d", got)
	}

	var journalMode string
	if err := db.Raw("PRAGMA journal_mode;").Scan(&journalMode).Error; err != nil {
		t.Fatalf("query journal_mode failed: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("expected journal_mode=wal, got %q", journalMode)
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout;").Scan(&busyTimeout).Error; err != nil {
		t.Fatalf("query busy_timeout failed: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("expected busy_timeout=5000, got %d", busyTimeout)
	}

	var synchronous int
	if err := db.Raw("PRAGMA synchronous;").Scan(&synchronous).Error; err != nil {
		t.Fatalf("query synchronous failed: %v", err)
	}
	if synchronous != 1 {
		t.Fatalf("expected synchronous=1 (NORMAL), got %d", synchronous)
	}

	var tempStore int
	if err := db.Raw("PRAGMA temp_store;").Scan(&tempStore).Error; err != nil {
		t.Fatalf("query temp_store failed: %v", err)
	}
	if tempStore != 2 {
		t.Fatalf("expected temp_store=2 (MEMORY), got %d", tempStore)
	}

	var cacheSize int
	if err := db.Raw("PRAGMA cache_size;").Scan(&cacheSize).Error; err != nil {
		t.Fatalf("query cache_size failed: %v", err)
	}
	if cacheSize != -8000 {
		t.Fatalf("expected cache_size=-8000, got %d", cacheSize)
	}
}
