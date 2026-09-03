package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"panel/internal/adapter/xray"
	"panel/internal/service"
)

func TestConfigService_SaveConfigQuietly_GuardsAgainstSilentWipe(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "xray_cfg_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	corruptedConfigPath := filepath.Join(tmpDir, "corrupted_config.json")
	corruptedContent := []byte(`{ "outbounds": [ { "tag": "proxy", "protocol": invalid_json }`)
	if err := os.WriteFile(corruptedConfigPath, corruptedContent, 0644); err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}

	configMgr := xray.NewConfigManager(corruptedConfigPath, "")
	compiler := xray.NewXrayCompiler(8080)
	svc := service.NewConfigService(configMgr, nil, nil, nil, nil, compiler)

	err = svc.SaveConfigQuietly(context.Background(), "test guard")
	if err == nil {
		t.Fatalf("expected SaveConfigQuietly to fail on corrupted config to protect file, but returned nil!")
	}

	// Verify the original file content was preserved and NOT wiped
	preservedContent, err := os.ReadFile(corruptedConfigPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(preservedContent) != string(corruptedContent) {
		t.Fatalf("config file was overwritten despite read error! Got:\n%s", string(preservedContent))
	}
}
