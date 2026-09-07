package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func findXrayBinary() (string, error) {
	candidates := []string{
		"../../bin/xray",
		"../../../bin/xray",
		"./bin/xray",
	}

	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for {
			candidates = append(candidates, filepath.Join(dir, "bin", "xray"))
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && (fi.Mode()&0111 != 0) {
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs, nil
			}
			return p, nil
		}
	}

	if p, err := exec.LookPath("xray"); err == nil {
		return p, nil
	}
	return "", errors.New("xray binary not found")
}

func getFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestXrayClient_RealBinary_Lifecycle(t *testing.T) {
	xrayBin, err := findXrayBinary()
	if err != nil {
		t.Skip("xray binary not found, skipping real binary test")
	}

	grpcPort := getFreePort(t)
	vlessPort := getFreePort(t)
	for vlessPort == grpcPort {
		vlessPort = getFreePort(t)
	}

	// Minimal valid Xray config
	rawConfig := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService"},
		},
		"stats": map[string]interface{}{},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]interface{}{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "api",
				"listen":   "127.0.0.1",
				"port":     grpcPort,
				"protocol": "dokodemo-door",
				"settings": map[string]interface{}{
					"address": "127.0.0.1",
				},
			},
			map[string]interface{}{
				"tag":      "vless-in",
				"listen":   "127.0.0.1",
				"port":     vlessPort,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients":    []interface{}{},
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network": "tcp",
				},
			},
		},
		"outbounds": []interface{}{
			map[string]interface{}{
				"tag":      "direct",
				"protocol": "freedom",
			},
		},
		"routing": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"type":        "field",
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
				},
			},
		},
	}

	cfgBytes, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal xray config: %v", err)
	}

	tmpDir := t.TempDir()
	initConfigFile := filepath.Join(tmpDir, "init-config.json")
	if err := os.WriteFile(initConfigFile, cfgBytes, 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, xrayBin, "run", "-c", initConfigFile)
	var logBuf bytes.Buffer
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start xray binary: %v", err)
	}
	defer func() {
		cmdCancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	grpcAddr := fmt.Sprintf("127.0.0.1:%d", grpcPort)

	// Wait for gRPC ready
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()
	readyConn, err := grpc.DialContext(dialCtx, grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatalf("xray gRPC failed to be ready within timeout: %v\nxray logs:\n%s", err, logBuf.String())
	}
	_ = readyConn.Close()

	// Initialize XrayClient with BoltDB
	dbPath := filepath.Join(tmpDir, "xray.db")
	client, err := NewXrayClient(grpcAddr, WithDBPath(dbPath), WithDialTimeout(3*time.Second))
	if err != nil {
		t.Fatalf("NewXrayClient failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. CheckHealth
	if err := client.CheckHealth(ctx); err != nil {
		t.Fatalf("CheckHealth failed: %v", err)
	}

	// 2. AddInboundUser
	testTag := "vless-in"
	testEmail := "realbinary@test.com"
	testUUID := "a1b2c3d4-e5f6-7890-1234-56789abcdef0"
	testFlow := ""

	if err := client.AddInboundUser(testTag, testEmail, testUUID, testFlow); err != nil {
		t.Fatalf("AddInboundUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// Verify user persisted to BoltDB
	u, err := client.GetInboundUser(testTag, testEmail)
	if err != nil || u == nil {
		t.Fatalf("expected user in BoltDB: %v", err)
	}

	// 3. QueryTraffic
	stats, err := client.QueryTraffic("", false)
	if err != nil {
		t.Fatalf("QueryTraffic failed: %v", err)
	}
	t.Logf("QueryTraffic succeeded, total stats entries: %d", len(stats))

	// 4. SyncToDiskConfig
	syncedConfigFile := filepath.Join(tmpDir, "synced-config.json")
	if err := client.SyncToDiskConfig(initConfigFile, syncedConfigFile); err != nil {
		t.Fatalf("SyncToDiskConfig failed: %v", err)
	}

	// Verify cold boot test with the synced config using 'xray -test -config'
	testCmd := exec.Command(xrayBin, "-test", "-config", syncedConfigFile)
	if testOut, err := testCmd.CombinedOutput(); err != nil {
		t.Fatalf("xray config validation failed on synced config: %v\noutput: %s", err, string(testOut))
	}

	// 5. RemoveInboundUser
	if err := client.RemoveInboundUser(testTag, testEmail); err != nil {
		t.Fatalf("RemoveInboundUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// Verify user removed from BoltDB
	_, err = client.GetInboundUser(testTag, testEmail)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected user to be removed from BoltDB, got err: %v", err)
	}
}
