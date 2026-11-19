package agent

import (
	"context"
	"testing"
)

func TestParseProcNetDevAndDelta(t *testing.T) {
	sample1 := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000000    1000    0    0    0     0          0         0  1000000    1000    0    0    0     0       0          0
  eth0: 5000000    4000    0    0    0     0          0         0  2000000    2500    0    0    0     0       0          0
docker0: 999999     500    0    0    0     0          0         0   888888     400    0    0    0     0       0          0
`
	rx1, tx1, ok1 := parseProcNetDev(sample1)
	if !ok1 || rx1 != 5000000 || tx1 != 2000000 {
		t.Fatalf("unexpected sample1 parse: ok=%v rx=%d tx=%d", ok1, rx1, tx1)
	}

	sample2 := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1200000    1100    0    0    0     0          0         0  1200000    1100    0    0    0     0       0          0
  eth0: 5500000    4200    0    0    0     0          0         0  2250000    2600    0    0    0     0       0          0
`
	rx2, tx2, ok2 := parseProcNetDev(sample2)
	if !ok2 {
		t.Fatalf("expected sample2 ok")
	}

	mc := NewMetricsCollector()
	m1 := mc.updateFromSamples(100, 200, true, 40.0, rx1, tx1, true)
	if m1.NicUpDelta != 0 || m1.NicDownDelta != 0 || m1.CPUPercent != 0 {
		t.Fatalf("expected 0 deltas on initial sample, got %+v", m1)
	}

	m2 := mc.updateFromSamples(150, 300, true, 45.25, rx2, tx2, true)
	if m2.NicDownDelta != 500000 || m2.NicUpDelta != 250000 {
		t.Fatalf("unexpected NIC deltas: %+v", m2)
	}
	if m2.CPUPercent != 50.0 || m2.MemPercent != 45.25 {
		t.Fatalf("unexpected CPU/Mem metrics: %+v", m2)
	}
}

func TestParseProcStatAndMeminfo(t *testing.T) {
	stat := "cpu  1000 100 200 7000 200 0 0 0 0 0\ncpu0 500 50 100 3500 100 0 0 0 0 0\n"
	idle, total, ok := parseProcStat(stat)
	if !ok || idle != 7200 || total != 8500 {
		t.Fatalf("unexpected parseProcStat: ok=%v idle=%d total=%d", ok, idle, total)
	}

	meminfo := "MemTotal:       1000000 kB\nMemFree:         200000 kB\nMemAvailable:    600000 kB\n"
	pct, ok := parseProcMeminfo(meminfo)
	if !ok || pct < 39.99 || pct > 40.01 {
		t.Fatalf("unexpected parseProcMeminfo: ok=%v pct=%f", ok, pct)
	}
}

func TestMetricsCollector_CollectCompilesAndRuns(t *testing.T) {
	mc := NewMetricsCollector()
	_ = mc.Collect(context.Background())
}
