package domain

import "time"

type SystemMetrics struct {
	CPUUsagePercent  float64 `json:"cpuUsagePercent"`
	MemoryTotalBytes uint64  `json:"memoryTotalBytes"`
	MemoryUsedBytes  uint64  `json:"memoryUsedBytes"`
	MemoryUsagePct   float64 `json:"memoryUsagePercent"`
	DiskTotalBytes   uint64  `json:"diskTotalBytes"`
	DiskUsedBytes    uint64  `json:"diskUsedBytes"`
	DiskUsagePct     float64 `json:"diskUsagePercent"`
	NetUpSpeedBps    uint64  `json:"netUpSpeedBps"`   // Bytes per second
	NetDownSpeedBps  uint64  `json:"netDownSpeedBps"` // Bytes per second
	NetTotalSent     uint64  `json:"netTotalSent"`
	NetTotalRecv     uint64  `json:"netTotalRecv"`
	UptimeSeconds    uint64  `json:"uptimeSeconds"`
	XrayRunning      bool    `json:"xrayRunning"`
	XrayVersion      string  `json:"xrayVersion"`
}

type ServiceStatus struct {
	Active     bool      `json:"active"`
	SubState   string    `json:"subState"`
	PID        int       `json:"pid"`
	Uptime     string    `json:"uptime"`
	LastReload time.Time `json:"lastReload"`
}
