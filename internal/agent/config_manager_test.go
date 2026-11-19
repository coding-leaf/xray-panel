package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordedCall struct {
	name string
	args []string
}

type fakeCommandRunner struct {
	mu    sync.Mutex
	calls []recordedCall
	fn    func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (f *fakeCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, recordedCall{name: name, args: append([]string(nil), args...)})
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(ctx, name, args...)
	}
	return []byte("ok"), nil
}

type fakeHealthProber struct {
	mu    sync.Mutex
	calls int
	fn    func(ctx context.Context, attempt int) error
}

func (p *fakeHealthProber) Probe(ctx context.Context) error {
	p.mu.Lock()
	p.calls++
	attempt := p.calls
	p.mu.Unlock()
	if p.fn != nil {
		return p.fn(ctx, attempt)
	}
	return nil
}

func TestConfigManager_ApplySuccess(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("seed old config: %v", err)
	}

	runner := &fakeCommandRunner{}
	prober := &fakeHealthProber{
		fn: func(ctx context.Context, attempt int) error {
			if attempt < 2 {
				return errors.New("starting up")
			}
			return nil
		},
	}

	mgr, err := NewConfigManager(ConfigManagerConfig{
		XrayBin:       "xray",
		ConfigPath:    cfgPath,
		ServiceName:   "xray",
		ProbeTimeout:  500 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, runner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}

	if err := mgr.Apply(context.Background(), `{"new":true}`); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(got) != `{"new":true}` {
		t.Fatalf("unexpected config content: %s", string(got))
	}

	bak, err := os.ReadFile(cfgPath + ".bak")
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(bak) != `{"old":true}` {
		t.Fatalf("unexpected backup content: %s", string(bak))
	}
}

func TestConfigManager_XrayTestFailureDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("seed old config: %v", err)
	}

	var testedPath string
	runner := &fakeCommandRunner{
		fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if len(args) >= 4 && args[0] == "run" && args[1] == "-test" && args[2] == "-c" {
				testedPath = args[3]
			}
			return []byte("invalid inbound config"), errors.New("exit status 23")
		},
	}
	prober := &fakeHealthProber{}

	mgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath: cfgPath,
	}, runner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}

	err = mgr.Apply(context.Background(), `{"broken":true}`)
	if err == nil {
		t.Fatalf("expected Apply to fail on xray run -test")
	}
	if filepath.Ext(testedPath) != ".json" {
		t.Fatalf("expected temp config path passed to xray -test to end with .json, got %q", testedPath)
	}

	got, _ := os.ReadFile(cfgPath)
	if string(got) != `{"old":true}` {
		t.Fatalf("expected original config preserved, got %s", string(got))
	}
	if _, statErr := os.Stat(testedPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected temp config cleaned up, stat err: %v", statErr)
	}
	if prober.calls != 0 {
		t.Fatalf("expected prober not called on test failure")
	}
}

func TestConfigManager_ProbeFailureRollsBackToBak(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"good":true}`), 0o600); err != nil {
		t.Fatalf("seed good config: %v", err)
	}

	runner := &fakeCommandRunner{}
	prober := &fakeHealthProber{
		fn: func(ctx context.Context, attempt int) error {
			return errors.New("connection refused")
		},
	}

	mgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath:    cfgPath,
		ProbeTimeout:  80 * time.Millisecond,
		ProbeInterval: 10 * time.Millisecond,
	}, runner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}

	err = mgr.Apply(context.Background(), `{"bad_runtime":true}`)
	if err == nil || !strings.Contains(err.Error(), "probe failed") {
		t.Fatalf("expected probe failure error, got: %v", err)
	}

	got, _ := os.ReadFile(cfgPath)
	if string(got) != `{"good":true}` {
		t.Fatalf("expected rollback to good config, got %s", string(got))
	}

	// Verify reload was called twice: once for apply, once for rollback.
	reloadCount := 0
	for _, c := range runner.calls {
		if c.name == "systemctl" {
			reloadCount++
		}
	}
	if reloadCount != 2 {
		t.Fatalf("expected 2 systemctl reload calls (apply + rollback), got %d", reloadCount)
	}
}

func TestConfigManager_ApplyCircuitBreaker(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	runner := &fakeCommandRunner{}
	prober := &fakeHealthProber{}
	mgr, err := NewConfigManager(ConfigManagerConfig{
		ConfigPath: cfgPath,
		GRPCAddr:   "127.0.0.1:8080",
	}, runner, prober)
	if err != nil {
		t.Fatalf("NewConfigManager: %v", err)
	}

	if err := mgr.ApplyCircuitBreaker(context.Background()); err != nil {
		t.Fatalf("ApplyCircuitBreaker failed: %v", err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read silence config: %v", err)
	}
	var parsed struct {
		Inbounds []struct {
			Tag string `json:"tag"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal silence config: %v", err)
	}
	if len(parsed.Inbounds) != 1 || parsed.Inbounds[0].Tag != "api" {
		t.Fatalf("expected only api inbound in circuit-breaker config, got %+v", parsed.Inbounds)
	}
}

func TestConfigManager_ReloadAndProbe(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	t.Run("success", func(t *testing.T) {
		runner := &fakeCommandRunner{}
		prober := &fakeHealthProber{}
		mgr, err := NewConfigManager(ConfigManagerConfig{
			ConfigPath:    cfgPath,
			ProbeTimeout:  200 * time.Millisecond,
			ProbeInterval: 10 * time.Millisecond,
		}, runner, prober)
		if err != nil {
			t.Fatalf("NewConfigManager: %v", err)
		}

		if err := mgr.ReloadAndProbe(context.Background()); err != nil {
			t.Fatalf("ReloadAndProbe failed: %v", err)
		}
		if len(runner.calls) != 1 || runner.calls[0].name != "systemctl" {
			t.Fatalf("expected 1 systemctl reload call, got %+v", runner.calls)
		}
		if prober.calls != 1 {
			t.Fatalf("expected 1 probe call, got %d", prober.calls)
		}
	})

	t.Run("reload failure skips probe", func(t *testing.T) {
		runner := &fakeCommandRunner{
			fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				return []byte("systemd error"), errors.New("exit status 1")
			},
		}
		prober := &fakeHealthProber{}
		mgr, err := NewConfigManager(ConfigManagerConfig{
			ConfigPath:    cfgPath,
			ProbeTimeout:  100 * time.Millisecond,
			ProbeInterval: 10 * time.Millisecond,
		}, runner, prober)
		if err != nil {
			t.Fatalf("NewConfigManager: %v", err)
		}

		err = mgr.ReloadAndProbe(context.Background())
		if err == nil || !strings.Contains(err.Error(), "reload xray failed") {
			t.Fatalf("expected reload failure, got: %v", err)
		}
		if prober.calls != 0 {
			t.Fatalf("expected 0 probe calls on reload failure, got %d", prober.calls)
		}
	})

	t.Run("probe failure returns error", func(t *testing.T) {
		runner := &fakeCommandRunner{}
		prober := &fakeHealthProber{
			fn: func(ctx context.Context, attempt int) error {
				return errors.New("grpc unavailable")
			},
		}
		mgr, err := NewConfigManager(ConfigManagerConfig{
			ConfigPath:    cfgPath,
			ProbeTimeout:  60 * time.Millisecond,
			ProbeInterval: 10 * time.Millisecond,
		}, runner, prober)
		if err != nil {
			t.Fatalf("NewConfigManager: %v", err)
		}

		err = mgr.ReloadAndProbe(context.Background())
		if err == nil || !strings.Contains(err.Error(), "probe failed") {
			t.Fatalf("expected probe failure, got: %v", err)
		}
	})
}
