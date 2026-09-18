package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

func TestIsIgnoredInterface(t *testing.T) {
	tests := []struct {
		name     string
		nicName  string
		expected bool
	}{
		// 空白网卡
		{"empty string", "", true},
		{"whitespace string", "   ", true},

		// 回环网卡
		{"loopback lo", "lo", true},
		{"loopback LO uppercase", "LO", true},
		{"loopback lo with space", "  lo  ", true},
		{"loopback lo0", "lo0", true},
		{"loopback lo1", "lo1", true},
		{"loopback lo10", "lo10", true},
		{"loopback prefix", "loopback", true},
		{"loopback0", "loopback0", true},
		{"Loopback uppercase", "Loopback", true},

		// lo 开头但不全为数字（非回环，如 local / log）
		{"lo non-numeric suffix local", "local", false},
		{"lo non-numeric suffix log0", "log0", false},

		// 容器及虚拟网卡
		{"docker0", "docker0", true},
		{"DOCKER uppercase", "DOCKER0", true},
		{"docker_gwbridge", "docker_gwbridge", true},
		{"veth pair", "veth12345", true},
		{"veth uppercase", "VETHabc", true},
		{"bridge br-", "br-1234567890ab", true},
		{"bridge BR- uppercase", "BR-lan", true},
		{"cni interface", "cni0", true},
		{"cni-pod", "cni-pod1", true},
		{"flannel interface", "flannel.1", true},
		{"virbr interface", "virbr0", true},
		{"virbr-nic", "virbr0-nic", true},

		// 真实物理网卡 (保留)
		{"ethernet eth0", "eth0", false},
		{"ethernet eth1", "eth1", false},
		{"ethernet enp3s0", "enp3s0", false},
		{"ethernet ens33", "ens33", false},
		{"ethernet eno1", "eno1", false},
		{"wifi wlan0", "wlan0", false},
		{"cellular wwan0", "wwan0", false},

		// 真实虚拟隧道网卡 (VPN/Proxy 常用，保留)
		{"wireguard wg0", "wg0", false},
		{"wireguard wg-out", "wg-out", false},
		{"tunnel tun0", "tun0", false},
		{"tap tap0", "tap0", false},
		{"tailscale0", "tailscale0", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := isIgnoredInterface(tc.nicName)
			if actual != tc.expected {
				t.Errorf("isIgnoredInterface(%q) = %v; want %v", tc.nicName, actual, tc.expected)
			}
		})
	}
}

func TestFilterAndSumIOCounters(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		sent, recv := filterAndSumIOCounters(nil)
		if sent != 0 || recv != 0 {
			t.Errorf("expected (0, 0), got (%d, %d)", sent, recv)
		}
	})

	t.Run("all ignored", func(t *testing.T) {
		counters := []net.IOCountersStat{
			{Name: "lo", BytesSent: 1000, BytesRecv: 1000},
			{Name: "docker0", BytesSent: 2000, BytesRecv: 2000},
			{Name: "veth99", BytesSent: 500, BytesRecv: 500},
		}
		sent, recv := filterAndSumIOCounters(counters)
		if sent != 0 || recv != 0 {
			t.Errorf("expected (0, 0), got (%d, %d)", sent, recv)
		}
	})

	t.Run("mixed interfaces", func(t *testing.T) {
		counters := []net.IOCountersStat{
			{Name: "lo", BytesSent: 1000, BytesRecv: 1000},
			{Name: "docker0", BytesSent: 2000, BytesRecv: 2000},
			{Name: "eth0", BytesSent: 5000, BytesRecv: 8000},
			{Name: "wg0", BytesSent: 3000, BytesRecv: 4000},
			{Name: "br-abc", BytesSent: 100, BytesRecv: 100},
		}
		// 有效网卡为 eth0 (5000, 8000) 和 wg0 (3000, 4000)
		// totalSent = 5000 + 3000 = 8000
		// totalRecv = 8000 + 4000 = 12000
		sent, recv := filterAndSumIOCounters(counters)
		if sent != 8000 || recv != 12000 {
			t.Errorf("expected (8000, 12000), got (%d, %d)", sent, recv)
		}
	})
}

func TestGopsutilMonitor_BasicAndConcurrency(t *testing.T) {
	m := NewGopsutilMonitor()

	ctx := context.Background()
	metrics, err := m.GetSystemMetrics(ctx)
	if err != nil {
		t.Fatalf("GetSystemMetrics failed: %v", err)
	}
	if metrics == nil {
		t.Fatal("expected non-nil metrics on initialization")
	}

	// 验证 Start and graceful stop with context cancellation
	startCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Start(startCtx)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not terminate within timeout")
	}
}
