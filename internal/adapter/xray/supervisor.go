package xray

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"panel/internal/domain"
)

type SystemdSupervisor struct {
	serviceName string
	xrayBinPath string
	lastReload  time.Time
}

func NewSystemdSupervisor(serviceName, xrayBinPath string) *SystemdSupervisor {
	if serviceName == "" {
		serviceName = "xray"
	}
	return &SystemdSupervisor{
		serviceName: serviceName,
		xrayBinPath: xrayBinPath,
	}
}

func (s *SystemdSupervisor) GetStatus(ctx context.Context) (domain.ServiceStatus, error) {
	if runtime.GOOS != "linux" {
		return domain.ServiceStatus{
			Active:     true,
			SubState:   "running (dev-mode)",
			PID:        1,
			Uptime:     "dev",
			LastReload: s.lastReload,
		}, nil
	}

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", s.serviceName)
	out, _ := cmd.Output()
	state := strings.TrimSpace(string(out))

	active := (state == "active")

	return domain.ServiceStatus{
		Active:     active,
		SubState:   state,
		PID:        0,
		Uptime:     "",
		LastReload: s.lastReload,
	}, nil
}

func (s *SystemdSupervisor) Restart(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		s.lastReload = time.Now()
		return nil
	}

	cmd := exec.CommandContext(ctx, "systemctl", "restart", s.serviceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart failed: %s, %w", string(out), err)
	}
	s.lastReload = time.Now()
	return nil
}

func (s *SystemdSupervisor) Reload(ctx context.Context) error {
	return s.Restart(ctx)
}

func (s *SystemdSupervisor) GetVersion(ctx context.Context) (string, error) {
	if s.xrayBinPath == "" {
		s.xrayBinPath = "xray"
	}
	cmd := exec.CommandContext(ctx, s.xrayBinPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "unknown", nil
}
