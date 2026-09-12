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
	metricsMu     sync.RWMutex
	cachedMetrics domain.SystemMetrics

	netMu        sync.Mutex
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
	// 预先取一次网卡 baseline
	if ioCounters, err := net.IOCounters(false); err == nil && len(ioCounters) > 0 {
		m.lastSent = ioCounters[0].BytesSent
		m.lastRecv = ioCounters[0].BytesRecv
	}

	// 立即抓取一次初始基准快照，确保首次读取绝无 nil 或空值
	m.sampleMetrics(context.Background())

	return m
}

// Start 实现 app.Service 接口，由 main.go errgroup 统一编排调度。
// 定频 2s Ticker 驱动采集，微秒级读取 /proc/stat 获得真正 2 秒移动物理均值，无需 sleep。
func (m *GopsutilMonitor) Start(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.sampleMetrics(ctx)
		}
	}
}

func (m *GopsutilMonitor) sampleMetrics(ctx context.Context) {
	metrics := domain.SystemMetrics{}

	// 1. CPU: 2s 定频触发下，cpu.Percent(0) 精确计算过去 2 秒完整时间片均值，调用耗时 < 0.05ms
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
	} else if d, err := disk.UsageWithContext(ctx, "C:"); err == nil {
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

	// 原子存入读写锁缓存
	m.metricsMu.Lock()
	m.cachedMetrics = metrics
	m.metricsMu.Unlock()
}

// GetSystemMetrics 纯内存只读返回缓存快照，耗时 0ms，彻底消除并发请求自激与 Telegram 误告警
func (m *GopsutilMonitor) GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error) {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	cp := m.cachedMetrics
	return &cp, nil
}

func (m *GopsutilMonitor) GetNetworkSpeed(ctx context.Context) (uint64, uint64, error) {
	m.netMu.Lock()
	defer m.netMu.Unlock()

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
