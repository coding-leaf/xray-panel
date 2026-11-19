package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCalcTrafficByMode(t *testing.T) {
	cases := []struct {
		name       string
		up         int64
		down       int64
		mode       string
		multiplier float64
		want       int64
	}{
		{"both default", 100, 250, TrafficCountModeBoth, 1.0, 350},
		{"outbound only", 100, 250, TrafficCountModeOutbound, 1.0, 100},
		{"max down", 100, 250, TrafficCountModeMax, 1.0, 250},
		{"max up", 300, 250, TrafficCountModeMax, 1.0, 300},
		{"empty mode defaults to both", 100, 200, "", 1.0, 300},
		{"multiplier 1.05", 1000, 1000, TrafficCountModeBoth, 1.05, 2100},
		{"non-positive multiplier defaults to 1", 100, 200, TrafficCountModeBoth, 0, 300},
		{"negative bytes clamped", -50, 100, TrafficCountModeBoth, 1.0, 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcTrafficByMode(tc.up, tc.down, tc.mode, tc.multiplier)
			if got != tc.want {
				t.Fatalf("CalcTrafficByMode(%d, %d, %q, %v) = %d, want %d", tc.up, tc.down, tc.mode, tc.multiplier, got, tc.want)
			}
		})
	}
}

func TestQuotaStore_AtomicPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	store, err := NewQuotaStore(dir)
	if err != nil {
		t.Fatalf("NewQuotaStore: %v", err)
	}

	policy := QuotaPolicy{
		TrafficLimitBytes: 1000,
		BreakThresholdPct: 95,
		CountMode:         TrafficCountModeBoth,
		Multiplier:        1.0,
		CycleUsedBytes:    900,
		CycleMonth:        202611,
	}
	if err := store.UpdatePolicy(policy, false); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, quotaStateFileName+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected no lingering .tmp file")
	}

	total, shouldBreak, err := store.RecordOfflineUsage(20, 20)
	if err != nil {
		t.Fatalf("RecordOfflineUsage(20, 20): %v", err)
	}
	if total != 940 || shouldBreak {
		t.Fatalf("got total=%d shouldBreak=%v, want 940 false", total, shouldBreak)
	}

	total, shouldBreak, err = store.RecordOfflineUsage(5, 5)
	if err != nil {
		t.Fatalf("RecordOfflineUsage(5, 5): %v", err)
	}
	if total != 950 || !shouldBreak {
		t.Fatalf("got total=%d shouldBreak=%v, want 950 true", total, shouldBreak)
	}

	if err := store.MarkBroken(); err != nil {
		t.Fatalf("MarkBroken: %v", err)
	}

	// Reload from disk into a fresh QuotaStore instance.
	reloaded, err := NewQuotaStore(dir)
	if err != nil {
		t.Fatalf("NewQuotaStore reload: %v", err)
	}
	st := reloaded.State()
	if st.Policy != policy {
		t.Fatalf("reloaded policy = %+v, want %+v", st.Policy, policy)
	}
	if st.LocalPendingUsedBytes != 50 {
		t.Fatalf("reloaded LocalPendingUsedBytes = %d, want 50", st.LocalPendingUsedBytes)
	}
	if !st.LocalCircuitBroken || !reloaded.ShouldBreak() {
		t.Fatalf("expected LocalCircuitBroken=true and ShouldBreak()=true after reload")
	}

	// Master policy refresh clears pending usage and broken state.
	policy.CycleUsedBytes = 960
	if err := reloaded.UpdatePolicy(policy, false); err != nil {
		t.Fatalf("UpdatePolicy refresh: %v", err)
	}
	st = reloaded.State()
	if st.LocalPendingUsedBytes != 0 || st.LocalCircuitBroken {
		t.Fatalf("expected pending=0 and broken=false after UpdatePolicy, got %+v", st)
	}
}

func TestQuotaStore_ZeroLimitNeverBreaks(t *testing.T) {
	store, err := NewQuotaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewQuotaStore: %v", err)
	}

	if err := store.UpdatePolicy(QuotaPolicy{
		TrafficLimitBytes: 0,
		BreakThresholdPct: 95,
		CountMode:         TrafficCountModeBoth,
		Multiplier:        1.0,
		CycleUsedBytes:    999999999,
	}, false); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	_, shouldBreak, err := store.RecordOfflineUsage(1024*1024, 1024*1024)
	if err != nil {
		t.Fatalf("RecordOfflineUsage: %v", err)
	}
	if shouldBreak || store.ShouldBreak() {
		t.Fatalf("expected ShouldBreak=false when TrafficLimitBytes <= 0")
	}
}

func TestQuotaStore_CorruptFileQuarantined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, quotaStateFileName)
	if err := os.WriteFile(path, []byte("{corrupt-json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store, err := NewQuotaStore(dir)
	if err != nil {
		t.Fatalf("NewQuotaStore on corrupt file: %v", err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("expected corrupt file quarantined: %v", err)
	}
	if store.ShouldBreak() {
		t.Fatalf("expected fresh state ShouldBreak=false")
	}
}

func TestQuotaStore_ConcurrentAccess(t *testing.T) {
	store, err := NewQuotaStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewQuotaStore: %v", err)
	}
	if err := store.UpdatePolicy(QuotaPolicy{
		TrafficLimitBytes: 10000,
		BreakThresholdPct: 95,
		CountMode:         TrafficCountModeBoth,
		Multiplier:        1.0,
	}, false); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, _, _ = store.RecordOfflineUsage(10, 10)
				_ = store.ShouldBreak()
				_ = store.State()
			}
		}()
	}
	wg.Wait()
}
