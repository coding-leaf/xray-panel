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
	serviceName     string
	xrayBinPath     string
	lastReload      time.Time
	verMu           sync.RWMutex
	cachedVersion   string
	statusMu        sync.RWMutex
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
	s.verMu.Lock()
	s.statusMu.Lock()
	defer s.verMu.Unlock()
	defer s.statusMu.Unlock()

	if serviceName != "" {
		s.serviceName = serviceName
	}
	if binPath != "" {
		s.xrayBinPath = binPath
		s.cachedVersion = "" // 重置版本缓存，下次读取自动重新检测
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

	// 3 秒节流缓存，杜绝轮询引发的连续 systemctl 进程 Fork
	s.statusMu.RLock()
	if time.Since(s.lastStatusCheck) < 3*time.Second && s.cachedStatus.SubState != "" {
		st := s.cachedStatus
		s.statusMu.RUnlock()
		return st, nil
	}
	s.statusMu.RUnlock()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", s.serviceName)
	out, _ := cmd.Output()
	state := strings.TrimSpace(string(out))
	active := (state == "active")

	st := domain.ServiceStatus{
		Active:     active,
		SubState:   state,
		PID:        0,
		Uptime:     "",
		LastReload: s.lastReload,
	}

	s.statusMu.Lock()
	s.cachedStatus = st
	s.lastStatusCheck = time.Now()
	s.statusMu.Unlock()

	return st, nil
}

func (s *SystemdSupervisor) Restart(ctx context.Context) error {
	s.statusMu.Lock()
	s.lastStatusCheck = time.Time{} // 重置状态缓存，强制立即刷新
	s.statusMu.Unlock()

	if runtime.GOOS != "linux" {
		s.lastReload = time.Now()
		return nil
	}

	cmd := exec.CommandContext(ctx, "systemctl", "restart", s.serviceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart failed: %s, %w", string(out), err)
	}
	s.lastReload = time.Now()

	// 重启后刷新版本缓存
	s.verMu.Lock()
	s.cachedVersion = ""
	s.verMu.Unlock()

	return nil
}

func (s *SystemdSupervisor) Reload(ctx context.Context) error {
	return s.Restart(ctx)
}

func (s *SystemdSupervisor) GetVersion(ctx context.Context) (string, error) {
	s.verMu.RLock()
	if s.cachedVersion != "" {
		v := s.cachedVersion
		s.verMu.RUnlock()
		return v, nil
	}
	s.verMu.RUnlock()

	if s.xrayBinPath == "" {
		s.xrayBinPath = "xray"
	}
	cmd := exec.CommandContext(ctx, s.xrayBinPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown", err
	}
	lines := strings.Split(string(out), "\n")
	ver := "unknown"
	if len(lines) > 0 {
		ver = strings.TrimSpace(lines[0])
	}

	s.verMu.Lock()
	s.cachedVersion = ver
	s.verMu.Unlock()

	return ver, nil
}
