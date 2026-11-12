package xray_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"panel/internal/adapter/xray"
)

func TestConfigManager_GetLogPaths_CacheAndInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	cfgPath2 := filepath.Join(tmpDir, "config2.json")

	initial := []byte(`{
		// jsonc comment
		"log": {
			"access": "/var/log/xray/access-v1.log",
			"error": "/var/log/xray/error-v1.log"
		}
	}`)
	if err := os.WriteFile(cfgPath, initial, 0644); err != nil {
		t.Fatalf("write initial config failed: %v", err)
	}

	mgr := xray.NewConfigManager(cfgPath, "")

	// 1. 首次读取解析并写入缓存
	acc, errLog := mgr.GetLogPaths()
	if acc != "/var/log/xray/access-v1.log" || errLog != "/var/log/xray/error-v1.log" {
		t.Fatalf("unexpected initial log paths: access=%q error=%q", acc, errLog)
	}

	// 2. 绕过 WriteConfig 直接篡改磁盘文件，验证再次 GetLogPaths 命中内存缓存
	bypassDisk := []byte(`{"log":{"access":"/tmp/bypass.log","error":"/tmp/bypass-err.log"}}`)
	if err := os.WriteFile(cfgPath, bypassDisk, 0644); err != nil {
		t.Fatalf("overwrite disk config failed: %v", err)
	}
	accCached, errCached := mgr.GetLogPaths()
	if accCached != "/var/log/xray/access-v1.log" || errCached != "/var/log/xray/error-v1.log" {
		t.Fatalf("expected cached log paths on second call, got access=%q error=%q", accCached, errCached)
	}

	// 3. 通过 WriteConfig 写入新配置后，缓存失效并即时刷新
	updated := []byte(`{"api":{"tag":"api"},"log":{"access":"/var/log/xray/access-v2.log","error":"/var/log/xray/error-v2.log"}}`)
	if err := mgr.WriteConfig(context.Background(), updated); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}
	acc2, err2 := mgr.GetLogPaths()
	if acc2 != "/var/log/xray/access-v2.log" || err2 != "/var/log/xray/error-v2.log" {
		t.Fatalf("expected refreshed log paths after WriteConfig, got access=%q error=%q", acc2, err2)
	}

	// 4. 通过 UpdateConfig 切换配置文件路径后，缓存失效并即时刷新
	cfg2Content := []byte(`{"log":{"access":"/var/log/xray/access-v3.log","error":"/var/log/xray/error-v3.log"}}`)
	if err := os.WriteFile(cfgPath2, cfg2Content, 0644); err != nil {
		t.Fatalf("write config2 failed: %v", err)
	}
	mgr.UpdateConfig(cfgPath2, "")
	acc3, err3 := mgr.GetLogPaths()
	if acc3 != "/var/log/xray/access-v3.log" || err3 != "/var/log/xray/error-v3.log" {
		t.Fatalf("expected refreshed log paths after UpdateConfig, got access=%q error=%q", acc3, err3)
	}
}

func TestConfigManager_GetLogPaths_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"api":{"tag":"api"},"log":{"access":"/var/log/a.log","error":"/var/log/e.log"}}`), 0644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	mgr := xray.NewConfigManager(cfgPath, "")
	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				acc, errLog := mgr.GetLogPaths()
				if acc == "" || errLog == "" {
					t.Errorf("unexpected empty log paths: access=%q error=%q", acc, errLog)
				}
			}
		}()
	}

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := mgr.WriteConfig(ctx, []byte(`{"api":{"tag":"api"},"log":{"access":"/var/log/a.log","error":"/var/log/e.log"}}`)); err != nil {
					t.Errorf("WriteConfig failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}
