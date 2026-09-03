package xray_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"panel/internal/adapter/xray"
)

func TestConfigManager_ConcurrentAtomicWriteAndRead(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config_atomic_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "config.json")
	initialJSON := []byte(`{"api":{"tag":"api"},"log":{"loglevel":"warning"}}`)
	if err := os.WriteFile(cfgPath, initialJSON, 0644); err != nil {
		t.Fatalf("write initial config failed: %v", err)
	}

	mgr := xray.NewConfigManager(cfgPath, "")

	var wg sync.WaitGroup
	ctx := context.Background()

	// 10 concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			payload := map[string]interface{}{
				"api": map[string]interface{}{"tag": "api"},
				"log": map[string]interface{}{"loglevel": "warning"},
				"meta": id,
			}
			b, _ := json.Marshal(payload)
			if err := mgr.WriteConfig(ctx, b); err != nil {
				t.Errorf("writer %d failed: %v", id, err)
			}
		}(i)
	}

	// 10 concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data, err := mgr.ReadRawConfig()
			if err != nil {
				t.Errorf("reader %d failed: %v", id, err)
				return
			}
			if len(data) == 0 {
				t.Errorf("reader %d read 0-byte truncated file! Atomic write violated!", id)
				return
			}
			var js map[string]interface{}
			if err := json.Unmarshal(data, &js); err != nil {
				t.Errorf("reader %d read corrupted/partial JSON: %v. Raw: %s", id, err, string(data))
			}
		}(i)
	}

	wg.Wait()

	// Verify backup file exists
	backupPath := cfgPath + ".bak"
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("expected backup file %s to exist, got error: %v", backupPath, err)
	}
}
