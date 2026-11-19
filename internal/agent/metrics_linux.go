//go:build linux

package agent

import (
	"context"
	"os"
)

// Collect reads /proc/stat, /proc/meminfo, and /proc/net/dev on Linux to compute HostMetrics.
func (m *MetricsCollector) Collect(_ context.Context) HostMetrics {
	var cpuIdle, cpuTotal uint64
	var hasCPU bool
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		cpuIdle, cpuTotal, hasCPU = parseProcStat(string(data))
	}

	var memPercent float64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		if pct, ok := parseProcMeminfo(string(data)); ok {
			memPercent = pct
		}
	}

	var nicRx, nicTx uint64
	var hasNIC bool
	if data, err := os.ReadFile("/proc/net/dev"); err == nil {
		nicRx, nicTx, hasNIC = parseProcNetDev(string(data))
	}

	return m.updateFromSamples(cpuIdle, cpuTotal, hasCPU, memPercent, nicRx, nicTx, hasNIC)
}
