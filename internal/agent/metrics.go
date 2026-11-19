package agent

import (
	"bufio"
	"math"
	"strconv"
	"strings"
	"sync"
)

type cpuSample struct {
	valid bool
	idle  uint64
	total uint64
}

type nicSample struct {
	valid bool
	rx    uint64
	tx    uint64
}

// MetricsCollector gathers host CPU, memory, and primary NIC byte deltas.
type MetricsCollector struct {
	mu      sync.Mutex
	prevCPU cpuSample
	prevNIC nicSample
}

// NewMetricsCollector creates a new MetricsCollector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

func (m *MetricsCollector) updateFromSamples(cpuIdle, cpuTotal uint64, hasCPU bool, memPercent float64, nicRx, nicTx uint64, hasNIC bool) HostMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out HostMetrics
	out.MemPercent = roundTwoDecimals(memPercent)

	if hasCPU {
		if m.prevCPU.valid && cpuTotal > m.prevCPU.total {
			totalDelta := cpuTotal - m.prevCPU.total
			idleDelta := uint64(0)
			if cpuIdle >= m.prevCPU.idle {
				idleDelta = cpuIdle - m.prevCPU.idle
			}
			if totalDelta > 0 && idleDelta <= totalDelta {
				used := float64(totalDelta-idleDelta) / float64(totalDelta) * 100.0
				out.CPUPercent = roundTwoDecimals(used)
			}
		}
		m.prevCPU = cpuSample{valid: true, idle: cpuIdle, total: cpuTotal}
	}

	if hasNIC {
		if m.prevNIC.valid {
			if nicTx >= m.prevNIC.tx {
				out.NicUpDelta = int64(nicTx - m.prevNIC.tx)
			}
			if nicRx >= m.prevNIC.rx {
				out.NicDownDelta = int64(nicRx - m.prevNIC.rx)
			}
		}
		m.prevNIC = nicSample{valid: true, rx: nicRx, tx: nicTx}
	}

	return out
}

func roundTwoDecimals(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return math.Round(v*100) / 100
}

// parseProcStat parses /proc/stat and returns (idleJiffies, totalJiffies, ok).
func parseProcStat(content string) (uint64, uint64, bool) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0, false
		}
		var total uint64
		var idle uint64
		for i, f := range fields[1:] {
			val, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return 0, 0, false
			}
			total += val
			// fields[4] is idle, fields[5] is iowait (index 3 and 4 in fields[1:])
			if i == 3 || i == 4 {
				idle += val
			}
		}
		return idle, total, true
	}
	return 0, 0, false
}

// parseProcMeminfo parses /proc/meminfo and returns memory usage percentage [0..100].
func parseProcMeminfo(content string) (float64, bool) {
	var memTotal, memAvail, memFree, buffers, cached uint64
	var hasTotal, hasAvail bool

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		rest := strings.Fields(strings.TrimSpace(line[colon+1:]))
		if len(rest) == 0 {
			continue
		}
		val, err := strconv.ParseUint(rest[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "MemTotal":
			memTotal = val
			hasTotal = true
		case "MemAvailable":
			memAvail = val
			hasAvail = true
		case "MemFree":
			memFree = val
		case "Buffers":
			buffers = val
		case "Cached":
			cached = val
		}
	}

	if !hasTotal || memTotal == 0 {
		return 0, false
	}
	avail := memAvail
	if !hasAvail {
		avail = memFree + buffers + cached
	}
	if avail > memTotal {
		avail = memTotal
	}
	return float64(memTotal-avail) / float64(memTotal) * 100.0, true
}

// parseProcNetDev parses /proc/net/dev and sums rx/tx bytes across physical/non-loopback interfaces.
func parseProcNetDev(content string) (rxBytes uint64, txBytes uint64, ok bool) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if shouldSkipInterface(iface) {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line[colon+1:]))
		// /proc/net/dev columns after ':': rx_bytes(0), rx_packets(1)... tx_bytes(8)
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		rxBytes += rx
		txBytes += tx
		ok = true
	}
	return rxBytes, txBytes, ok
}

func shouldSkipInterface(iface string) bool {
	if iface == "" || iface == "lo" {
		return true
	}
	for _, prefix := range []string{"veth", "docker", "br-", "virbr", "tun", "tap", "wg"} {
		if strings.HasPrefix(iface, prefix) {
			return true
		}
	}
	return false
}
