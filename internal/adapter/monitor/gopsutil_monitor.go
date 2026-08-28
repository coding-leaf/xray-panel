package monitor

import (
	"context"
	"sync"
	"time"

	"panel/internal/domain"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

type GopsutilMonitor struct {
	mu           sync.Mutex
	lastNetCheck time.Time
	lastSent     uint64
	lastRecv     uint64
	upSpeed      uint64
	downSpeed    uint64
}

func NewGopsutilMonitor() *GopsutilMonitor {
	m := &GopsutilMonitor{
		lastNetCheck: time.Now(),
	}
	// 预先取一次 baseline
	if ioCounters, err := net.IOCounters(false); err == nil && len(ioCounters) > 0 {
		m.lastSent = ioCounters[0].BytesSent
		m.lastRecv = ioCounters[0].BytesRecv
	}
	return m
}

func (m *GopsutilMonitor) GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error) {
	metrics := &domain.SystemMetrics{}

	// 1. CPU
	cpuPercents, err := cpu.PercentWithContext(ctx, 0, false)
	if err == nil && len(cpuPercents) > 0 {
		metrics.CPUUsagePercent = cpuPercents[0]
	}

	// 2. Memory
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metrics.MemoryTotalBytes = vm.Total
		metrics.MemoryUsedBytes = vm.Used
		metrics.MemoryUsagePct = vm.UsedPercent
	}

	// 3. Disk
	if d, err := disk.UsageWithContext(ctx, "/"); err == nil {
		metrics.DiskTotalBytes = d.Total
		metrics.DiskUsedBytes = d.Used
		metrics.DiskUsagePct = d.UsedPercent
	} else if d, err := disk.UsageWithContext(ctx, "C:"); err == nil { // Windows fallback
		metrics.DiskTotalBytes = d.Total
		metrics.DiskUsedBytes = d.Used
		metrics.DiskUsagePct = d.UsedPercent
	}

	// 4. Uptime
	if h, err := host.InfoWithContext(ctx); err == nil {
		metrics.UptimeSeconds = h.Uptime
	}

	// 5. Network Speed & Totals
	upSpeed, downSpeed, _ := m.GetNetworkSpeed(ctx)
	metrics.NetUpSpeedBps = upSpeed
	metrics.NetDownSpeedBps = downSpeed

	if ioCounters, err := net.IOCountersWithContext(ctx, false); err == nil && len(ioCounters) > 0 {
		metrics.NetTotalSent = ioCounters[0].BytesSent
		metrics.NetTotalRecv = ioCounters[0].BytesRecv
	}

	return metrics, nil
}

func (m *GopsutilMonitor) GetNetworkSpeed(ctx context.Context) (uint64, uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(m.lastNetCheck).Seconds()
	if elapsed < 0.8 {
		return m.upSpeed, m.downSpeed, nil
	}

	ioCounters, err := net.IOCountersWithContext(ctx, false)
	if err != nil || len(ioCounters) == 0 {
		return m.upSpeed, m.downSpeed, err
	}

	currSent := ioCounters[0].BytesSent
	currRecv := ioCounters[0].BytesRecv

	if m.lastSent > 0 && currSent >= m.lastSent {
		m.upSpeed = uint64(float64(currSent-m.lastSent) / elapsed)
	}
	if m.lastRecv > 0 && currRecv >= m.lastRecv {
		m.downSpeed = uint64(float64(currRecv-m.lastRecv) / elapsed)
	}

	m.lastSent = currSent
	m.lastRecv = currRecv
	m.lastNetCheck = now

	return m.upSpeed, m.downSpeed, nil
}
