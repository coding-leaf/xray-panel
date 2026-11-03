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
	"runtime"
	"strings"
	"testing"
	"time"

	"panel/internal/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// FindXrayBinary locates the Xray binary on the system or temp directory
func FindXrayBinary() (string, error) {
	// 0. Check explicit environment variables
	for _, envKey := range []string{"XRAY_BIN", "XRAY_BIN_PATH"} {
		if val := strings.TrimSpace(os.Getenv(envKey)); val != "" {
			if fi, err := os.Stat(val); err == nil && !fi.IsDir() {
				if abs, err := filepath.Abs(val); err == nil {
					return abs, nil
				}
				return val, nil
			}
		}
	}

	// 1. Check temp directory pre-release cache
	tempCandidates := []string{
		filepath.Join(os.TempDir(), "opencode", "xray_pre", "xray.exe"),
		filepath.Join(os.TempDir(), "opencode", "xray_pre", "xray"),
	}
	for _, p := range tempCandidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}

	// 2. Check common relative paths
	candidates := []string{
		"../../../temp/xray",
		"../../temp/xray",
		"../temp/xray",
		"./temp/xray",
		"../../../bin/xray",
		"../../bin/xray",
		"./bin/xray",
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			"../../../temp/xray.exe",
			"../../temp/xray.exe",
			"../temp/xray.exe",
			"./temp/xray.exe",
			"../../../bin/xray.exe",
			"../../bin/xray.exe",
			"./bin/xray.exe",
		)
	}

	// 3. Search upwards from current directory to repository root
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for {
			candidates = append(candidates,
				filepath.Join(dir, "temp", "xray"),
				filepath.Join(dir, "bin", "xray"),
			)
			if runtime.GOOS == "windows" {
				candidates = append(candidates,
					filepath.Join(dir, "temp", "xray.exe"),
					filepath.Join(dir, "bin", "xray.exe"),
				)
			}
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
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			if runtime.GOOS != "windows" && fi.Mode()&0111 == 0 {
				continue
			}
			abs, err := filepath.Abs(p)
			if err == nil {
				return abs, nil
			}
			return p, nil
		}
	}

	// 4. Check system PATH
	for _, binName := range []string{"xray", "xray.exe"} {
		if p, err := exec.LookPath(binName); err == nil {
			return p, nil
		}
	}

	return "", errors.New("xray binary not found")
}

func getFreePort(t *testing.T) int {
	for i := 0; i < 100; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		port := l.Addr().(*net.TCPAddr).Port
		u, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
		if err == nil {
			_ = u.Close()
			_ = l.Close()
			return port
		}
		_ = l.Close()
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("failed to find a port free for both TCP and UDP")
	return 0
}

func TestXrayCore_GRPC_RealBinary(t *testing.T) {
	xrayBin, err := FindXrayBinary()
	if err != nil {
		t.Skip("xray binary not found, skipping integration test")
	}

	grpcPort := getFreePort(t)
	vlessPort := getFreePort(t)
	trojanPort := getFreePort(t)
	ssPort := getFreePort(t)
	vmessPort := getFreePort(t)

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
		{
			Tag:      "trojan-test",
			Listen:   "127.0.0.1",
			Port:     trojanPort,
			Protocol: "trojan",
			Enabled:  true,
		},
		{
			Tag:          "ss-test",
			Listen:       "127.0.0.1",
			Port:         ssPort,
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"aes-128-gcm","password":"test-password"}`,
			Enabled:      true,
		},
		{
			Tag:      "vmess-test",
			Listen:   "127.0.0.1",
			Port:     vmessPort,
			Protocol: "vmess",
			Enabled:  true,
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
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(xrayBin))
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

	trojanInbound := &domain.Inbound{
		Tag:      "trojan-test",
		Protocol: "trojan",
		Port:     trojanPort,
	}
	trojanUser := &domain.User{
		Email: "trojan@test.com",
		UUID:  "password12345",
	}
	if err := client.AddUser(ctx, trojanInbound, trojanUser); err != nil {
		t.Fatalf("AddUser trojan failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	ssInbound := &domain.Inbound{
		Tag:          "ss-test",
		Protocol:     "shadowsocks",
		Port:         ssPort,
		SettingsJSON: `{"method":"aes-128-gcm","password":"test-password"}`,
	}
	ssUser := &domain.User{
		Email: "ss@test.com",
		UUID:  "user-password-123",
	}
	if err := client.AddUser(ctx, ssInbound, ssUser); err != nil {
		t.Fatalf("AddUser ss failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	vmessInbound := &domain.Inbound{
		Tag:      "vmess-test",
		Protocol: "vmess",
		Port:     vmessPort,
	}
	vmessUser := &domain.User{
		Email: "vmess@test.com",
		UUID:  "c8402f1a-6df2-4217-a169-23114d640202",
	}
	if err := client.AddUser(ctx, vmessInbound, vmessUser); err != nil {
		t.Fatalf("AddUser vmess failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// 2. User Mutation Re-sync:
	// Scenario A: Email update (Remove oldEmail -> Add newEmail with same UUID)
	if err := client.RemoveUser(ctx, "vless-test", "integration@test.com"); err != nil {
		t.Fatalf("Mutation Email RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	updatedEmailUser := &domain.User{
		Email: "integration-updated@test.com",
		UUID:  "c8402f1a-6df2-4217-a169-23114d640201",
	}
	if err := client.AddUser(ctx, testInbound, updatedEmailUser); err != nil {
		t.Fatalf("Mutation Email AddUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// Scenario B: UUID rotation (Remove current -> Add same email with new UUID)
	if err := client.RemoveUser(ctx, "vless-test", "integration-updated@test.com"); err != nil {
		t.Fatalf("Mutation UUID RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	rotatedUUIDUser := &domain.User{
		Email: "integration-updated@test.com",
		UUID:  "e1234567-89ab-4cde-8012-3456789abcde",
	}
	if err := client.AddUser(ctx, testInbound, rotatedUUIDUser); err != nil {
		t.Fatalf("Mutation UUID AddUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// 3. QueryTrafficStats
	stats, err := client.QueryTrafficStats(ctx, false)
	if err != nil {
		t.Fatalf("QueryTrafficStats failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	t.Logf("QueryTrafficStats succeeded, record count: %d", len(stats))

	// 4. Teardown all users across protocols
	if err := client.RemoveUser(ctx, "vless-test", rotatedUUIDUser.Email); err != nil {
		t.Fatalf("Teardown vless RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	if err := client.RemoveUser(ctx, "trojan-test", trojanUser.Email); err != nil {
		t.Fatalf("Teardown trojan RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	if err := client.RemoveUser(ctx, "ss-test", ssUser.Email); err != nil {
		t.Fatalf("Teardown ss RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}
	if err := client.RemoveUser(ctx, "vmess-test", vmessUser.Email); err != nil {
		t.Fatalf("Teardown vmess RemoveUser failed: %v\nxray logs:\n%s", err, logBuf.String())
	}

	// 5. Verify duplicate RemoveUser returns error (expected: user not found)
	dupErr := client.RemoveUser(ctx, "vless-test", rotatedUUIDUser.Email)
	if dupErr == nil {
		t.Fatalf("expected error for duplicate RemoveUser, got nil")
	}
	t.Logf("Duplicate RemoveUser properly rejected: %v", dupErr)
}
