package xray

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
)

type SystemdSupervisor struct {
	mu              sync.RWMutex
	serviceName     string
	xrayBinPath     string
	lastReload      time.Time
	cachedVersion   string
	cachedStatus    domain.ServiceStatus
	lastStatusCheck time.Time
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

func (s *SystemdSupervisor) UpdateConfig(serviceName, binPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if serviceName != "" {
		s.serviceName = serviceName
		s.lastStatusCheck = time.Time{}
	}
	if binPath != "" {
		s.xrayBinPath = binPath
		s.cachedVersion = "" // 重置版本缓存，下次读取自动重新检测
	}
}

func (s *SystemdSupervisor) GetStatus(ctx context.Context) (domain.ServiceStatus, error) {
	if runtime.GOOS != "linux" {
		s.mu.RLock()
		lastReload := s.lastReload
		s.mu.RUnlock()
		return domain.ServiceStatus{
			Active:     true,
			SubState:   "running (dev-mode)",
			PID:        1,
			Uptime:     "dev",
			LastReload: lastReload,
		}, nil
	}

	// 3 秒节流缓存，杜绝轮询引发的连续 systemctl 进程 Fork
	s.mu.RLock()
	if time.Since(s.lastStatusCheck) < 3*time.Second && s.cachedStatus.SubState != "" {
		st := s.cachedStatus
		s.mu.RUnlock()
		return st, nil
	}
	serviceName := s.serviceName
	s.mu.RUnlock()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", serviceName)
	out, _ := cmd.Output()
	state := strings.TrimSpace(string(out))
	active := (state == "active")

	s.mu.Lock()
	st := domain.ServiceStatus{
		Active:     active,
		SubState:   state,
		PID:        0,
		Uptime:     "",
		LastReload: s.lastReload,
	}
	s.cachedStatus = st
	s.lastStatusCheck = time.Now()
	s.mu.Unlock()

	return st, nil
}

func (s *SystemdSupervisor) Restart(ctx context.Context) error {
	s.mu.Lock()
	s.lastStatusCheck = time.Time{} // 重置状态缓存，强制立即刷新
	serviceName := s.serviceName
	s.mu.Unlock()

	if runtime.GOOS != "linux" {
		s.mu.Lock()
		s.lastReload = time.Now()
		s.mu.Unlock()
		return nil
	}

	cmd := exec.CommandContext(ctx, "systemctl", "restart", serviceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart failed: %s, %w", string(out), err)
	}

	s.mu.Lock()
	s.lastReload = time.Now()
	s.cachedVersion = ""
	s.lastStatusCheck = time.Time{}
	s.mu.Unlock()

	return nil
}

func (s *SystemdSupervisor) Reload(ctx context.Context) error {
	return s.Restart(ctx)
}

func (s *SystemdSupervisor) GetVersion(ctx context.Context) (string, error) {
	s.mu.RLock()
	if s.cachedVersion != "" {
		v := s.cachedVersion
		s.mu.RUnlock()
		return v, nil
	}
	binPath := s.xrayBinPath
	s.mu.RUnlock()

	if binPath == "" {
		binPath = "xray"
	}
	cmd := exec.CommandContext(ctx, binPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(string(out), "\n")
	ver := "unknown"
	if len(lines) > 0 {
		ver = strings.TrimSpace(lines[0])
	}

	s.mu.Lock()
	s.cachedVersion = ver
	s.mu.Unlock()

	return ver, nil
}
