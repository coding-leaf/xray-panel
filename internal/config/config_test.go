package config

import (
	"flag"
	"os"
	"testing"
)

func TestConfig_PanelVersion_Default(t *testing.T) {
	os.Unsetenv("PANEL_VERSION")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := LoadWithFlagSet(fs, []string{}, "v2.6.6", "commit-abc", "2026-10-15")

	if cfg.PanelVersion != "v2.6.6" {
		t.Fatalf("expected PanelVersion v2.6.6, got %s", cfg.PanelVersion)
	}
	if cfg.Commit != "commit-abc" {
		t.Fatalf("expected Commit commit-abc, got %s", cfg.Commit)
	}
	if cfg.BuildTime != "2026-10-15" {
		t.Fatalf("expected BuildTime 2026-10-15, got %s", cfg.BuildTime)
	}
}

func TestConfig_PanelVersion_EnvOverride(t *testing.T) {
	os.Setenv("PANEL_VERSION", "v2.7.0-beta")
	defer os.Unsetenv("PANEL_VERSION")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := LoadWithFlagSet(fs, []string{}, "v2.6.6", "dev", "unknown")

	if cfg.PanelVersion != "v2.7.0-beta" {
		t.Fatalf("expected PanelVersion v2.7.0-beta from env, got %s", cfg.PanelVersion)
	}
}

func TestConfig_PanelVersion_FlagOverride(t *testing.T) {
	os.Setenv("PANEL_VERSION", "v2.7.0-beta")
	defer os.Unsetenv("PANEL_VERSION")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := LoadWithFlagSet(fs, []string{"-panel-version", "v2.8.0-custom"}, "v2.6.6", "dev", "unknown")

	if cfg.PanelVersion != "v2.8.0-custom" {
		t.Fatalf("expected PanelVersion v2.8.0-custom from flag override, got %s", cfg.PanelVersion)
	}
}

func TestConfig_Load_CommandLineIntegration(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"panel"}

	// Reset flag.CommandLine to isolate test
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	os.Setenv("PANEL_VERSION", "v2.9.0-env")
	defer os.Unsetenv("PANEL_VERSION")

	cfg := Load("v2.6.6", "test-commit", "test-time")
	if cfg.PanelVersion != "v2.9.0-env" {
		t.Fatalf("expected PanelVersion v2.9.0-env, got %s", cfg.PanelVersion)
	}
	if cfg.Commit != "test-commit" {
		t.Fatalf("expected Commit test-commit, got %s", cfg.Commit)
	}
	if cfg.BuildTime != "test-time" {
		t.Fatalf("expected BuildTime test-time, got %s", cfg.BuildTime)
	}
}

func TestConfig_VersionFlag_Output(t *testing.T) {
	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	var exitedCode int
	exitCalled := false
	exitFunc = func(code int) {
		exitCalled = true
		exitedCode = code
	}

	fs := flag.NewFlagSet("test-v", flag.ContinueOnError)
	cfg := LoadWithFlagSet(fs, []string{"-panel-version", "v2.9.9-override", "-v"}, "v2.6.6", "dev", "unknown")

	if !exitCalled {
		t.Fatalf("expected exitFunc to be called when -v is passed")
	}
	if exitedCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitedCode)
	}
	if cfg.PanelVersion != "v2.9.9-override" {
		t.Fatalf("expected PanelVersion v2.9.9-override, got %s", cfg.PanelVersion)
	}
}
