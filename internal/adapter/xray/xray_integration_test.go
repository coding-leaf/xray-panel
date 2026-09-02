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

	"panel/internal/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func findXrayBinary() (string, error) {
	// 1. Check common relative paths
	candidates := []string{
		"../../../bin/xray",
		"../../bin/xray",
		"./bin/xray",
	}

	// 2. Search upwards from current directory to repository root
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

	// 3. Check system PATH
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

func TestXrayCore_GRPC_RealBinary(t *testing.T) {
	xrayBin, err := findXrayBinary()
	if err != nil {
		t.Skip("xray binary not found, skipping integration test")
	}

	grpcPort := getFreePort(t)
	vlessPort := getFreePort(t)
	for vlessPort == grpcPort {
		vlessPort = getFreePort(t)
	}

	compiler := NewXrayCompiler(grpcPort)
	inbounds := []domain.Inbound{
		{
			Tag:            "vless-test",
			Listen:         "127.0.0.1",
			Port:           vlessPort,
			Protocol:       "vless",
			Enabled:        true,
			StreamSettings: `{"network":"tcp"}`,
		},
	}
	outbounds := []domain.Outbound{
		{
			Tag:      "direct",
			Protocol: "freedom",
		},
	}

	compiledCfg, err := compiler.Compile(inbounds, outbounds, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to compile xray config: %v", err)
	}

	// Override log config to avoid permission issues writing to /var/log/xray in unprivileged environments
	compiledCfg.Log = &XrayLogConfig{
		LogLevel: "warning",
	}

	cfgJSON, err := json.MarshalIndent(compiledCfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal xray config: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "xray-integration-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpConfigPath := tmpFile.Name()
	defer os.Remove(tmpConfigPath)

	if _, err := tmpFile.Write(cfgJSON); err != nil {
		_ = tmpFile.Close()
		t.Fatalf("failed to write config to temp file: %v", err)
	}
	_ = tmpFile.Close()

	cmdCtx, cmdCancel := context.WithCancel(context.Background())
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, xrayBin, "run", "-c", tmpConfigPath)
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

	// Wait for Xray gRPC to be ready
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	readyConn, err := grpc.DialContext(
		dialCtx,
		grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("xray gRPC failed to be ready within timeout: %v\nxray logs:\n%s", err, logBuf.String())
	}
	_ = readyConn.Close()

	// Initialize GRPCClient
	client := NewGRPCClient(grpcAddr)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testInbound := &domain.Inbound{
		Tag:            "vless-test",
		Protocol:       "vless",
		Port:           vlessPort,
		StreamSettings: `{"network":"tcp"}`,
	}
	testUser := &domain.User{
		Email: "integration@test.com",
		UUID:  "c8402f1a-6df2-4217-a169-23114d640201",
	}

	// 1. AddUser
	if err := client.AddUser(ctx, testInbound, testUser); err != nil {
		t.Fatalf("AddUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// 2. QueryTrafficStats
	stats, err := client.QueryTrafficStats(ctx, false)
	if err != nil {
		t.Fatalf("QueryTrafficStats failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	t.Logf("QueryTrafficStats succeeded, record count: %d", len(stats))

	// 3. RemoveUser
	if err := client.RemoveUser(ctx, "vless-test", "integration@test.com"); err != nil {
		t.Fatalf("RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
}
