package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CommandRunner abstracts OS command execution for testability across Windows and Linux.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// HealthProber checks whether the local Xray gRPC endpoint is responsive after a reload.
type HealthProber interface {
	Probe(ctx context.Context) error
}

// OSCommandRunner executes real OS commands via os/exec.
type OSCommandRunner struct{}

// Run executes name with args and returns combined stdout/stderr output.
func (OSCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// ConfigManagerConfig configures ConfigManager behavior.
type ConfigManagerConfig struct {
	XrayBin       string
	ConfigPath    string
	ServiceName   string
	GRPCAddr      string
	ProbeTimeout  time.Duration
	ProbeInterval time.Duration
	ReloadCmd     []string
}

// ConfigManager safely validates, swaps, reloads, probes, and rolls back Xray config files.
type ConfigManager struct {
	mu     sync.Mutex
	cfg    ConfigManagerConfig
	runner CommandRunner
	prober HealthProber
}

// NewConfigManager constructs a ConfigManager with sensible defaults.
func NewConfigManager(cfg ConfigManagerConfig, runner CommandRunner, prober HealthProber) (*ConfigManager, error) {
	if strings.TrimSpace(cfg.ConfigPath) == "" {
		return nil, errors.New("xray config path is required")
	}
	if strings.TrimSpace(cfg.XrayBin) == "" {
		cfg.XrayBin = "xray"
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		cfg.ServiceName = "xray"
	}
	if strings.TrimSpace(cfg.GRPCAddr) == "" {
		cfg.GRPCAddr = "127.0.0.1:8080"
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = 3 * time.Second
	}
	if cfg.ProbeInterval <= 0 {
		cfg.ProbeInterval = 200 * time.Millisecond
	}
	if runner == nil {
		runner = OSCommandRunner{}
	}
	return &ConfigManager{
		cfg:    cfg,
		runner: runner,
		prober: prober,
	}, nil
}

// Apply writes configJSON to a temp file, validates with `xray run -test`, backs up the existing config,
// atomically replaces it, reloads Xray, and probes gRPC within ProbeTimeout—rolling back on any failure.
func (m *ConfigManager) Apply(ctx context.Context, configJSON string) error {
	if strings.TrimSpace(configJSON) == "" {
		return errors.New("config json is empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(m.cfg.ConfigPath), 0o755); err != nil {
		return fmt.Errorf("create xray config dir: %w", err)
	}

	tmpPath := tempConfigPath(m.cfg.ConfigPath)
	if err := writeFileAtomicSync(tmpPath, []byte(configJSON), 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	out, err := m.runner.Run(ctx, m.cfg.XrayBin, "run", "-test", "-c", tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("xray config test failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	bakPath := m.cfg.ConfigPath + ".bak"
	hasBackup := false
	if existing, readErr := os.ReadFile(m.cfg.ConfigPath); readErr == nil {
		if writeErr := writeFileAtomicSync(bakPath, existing, 0o600); writeErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("backup existing config: %w", writeErr)
		}
		hasBackup = true
	} else if !errors.Is(readErr, os.ErrNotExist) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("read existing config for backup: %w", readErr)
	}

	if err := os.Rename(tmpPath, m.cfg.ConfigPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config file: %w", err)
	}

	if err := m.reloadService(ctx); err != nil {
		if hasBackup {
			rbErr := m.rollbackLocked(ctx, bakPath)
			return fmt.Errorf("reload xray failed (%w); rollback: %v", err, rbErr)
		}
		return fmt.Errorf("reload xray failed: %w", err)
	}

	if err := m.waitForProbe(ctx); err != nil {
		if hasBackup {
			rbErr := m.rollbackLocked(ctx, bakPath)
			return fmt.Errorf("xray grpc probe failed (%w); rollback: %v", err, rbErr)
		}
		return fmt.Errorf("xray grpc probe failed: %w", err)
	}

	return nil
}

// ApplyCircuitBreaker replaces the active config with a minimal silence config containing
// only the local gRPC API inbound and freedom outbound (no business inbounds).
func (m *ConfigManager) ApplyCircuitBreaker(ctx context.Context) error {
	silenceCfg, err := buildSilenceConfigJSON(m.cfg.GRPCAddr)
	if err != nil {
		return fmt.Errorf("build circuit-breaker config: %w", err)
	}
	return m.Apply(ctx, silenceCfg)
}

// DetectXrayVersion queries `xray version` and returns the version token (e.g. "25.1.1"), or "" on error.
func (m *ConfigManager) DetectXrayVersion(ctx context.Context) string {
	out, err := m.runner.Run(ctx, m.cfg.XrayBin, "version")
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) >= 2 && strings.EqualFold(fields[0], "Xray") {
		return fields[1]
	}
	return line
}

func (m *ConfigManager) reloadService(ctx context.Context) error {
	name, args := m.reloadCommand()
	out, err := m.runner.Run(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *ConfigManager) reloadCommand() (string, []string) {
	if len(m.cfg.ReloadCmd) > 0 {
		return m.cfg.ReloadCmd[0], m.cfg.ReloadCmd[1:]
	}
	return "systemctl", []string{"reload-or-restart", m.cfg.ServiceName}
}

func (m *ConfigManager) waitForProbe(ctx context.Context) error {
	if m.prober == nil {
		return nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, m.cfg.ProbeTimeout)
	defer cancel()

	var lastErr error
	ticker := time.NewTicker(m.cfg.ProbeInterval)
	defer ticker.Stop()

	for {
		if err := m.prober.Probe(probeCtx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-probeCtx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w (last probe: %v)", probeCtx.Err(), lastErr)
			}
			return probeCtx.Err()
		case <-ticker.C:
		}
	}
}

func (m *ConfigManager) rollbackLocked(ctx context.Context, bakPath string) error {
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		return fmt.Errorf("read backup %s: %w", bakPath, err)
	}
	if err := writeFileAtomicSync(m.cfg.ConfigPath, bakData, 0o644); err != nil {
		return fmt.Errorf("restore backup to %s: %w", m.cfg.ConfigPath, err)
	}
	if err := m.reloadService(rbCtx); err != nil {
		return fmt.Errorf("reload after restore: %w", err)
	}
	return nil
}

// tempConfigPath derives a sibling temporary path that preserves the .json extension
// so `xray run -test -c <path>` recognizes the JSON format reliably.
func tempConfigPath(configPath string) string {
	ext := filepath.Ext(configPath)
	if strings.EqualFold(ext, ".json") {
		return strings.TrimSuffix(configPath, ext) + ".tmp" + ext
	}
	return configPath + ".tmp.json"
}

func writeFileAtomicSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}

func buildSilenceConfigJSON(grpcAddr string) (string, error) {
	host, portStr, err := net.SplitHostPort(grpcAddr)
	if err != nil {
		host = "127.0.0.1"
		portStr = "8080"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		port = 8080
	}

	doc := map[string]any{
		"log": map[string]any{
			"loglevel": "warning",
		},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService"},
		},
		"inbounds": []map[string]any{
			{
				"tag":      "api",
				"listen":   host,
				"port":     port,
				"protocol": "dokodemo-door",
				"settings": map[string]any{
					"address": host,
				},
			},
		},
		"outbounds": []map[string]any{
			{
				"tag":      "direct",
				"protocol": "freedom",
			},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{
					"type":        "field",
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
				},
			},
		},
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
