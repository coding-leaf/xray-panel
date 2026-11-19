package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const quotaStateFileName = "quota_state.json"

// Traffic counting modes matching Master's domain.TrafficCountMode* constants.
const (
	TrafficCountModeBoth     = "both"
	TrafficCountModeOutbound = "outbound"
	TrafficCountModeMax      = "max"
)

// LocalQuotaState persists the latest Master quota policy and offline usage accumulator on disk.
type LocalQuotaState struct {
	Policy                QuotaPolicy `json:"policy"`
	LocalPendingUsedBytes int64       `json:"localPendingUsedBytes"`
	LocalCircuitBroken    bool        `json:"localCircuitBroken"`
	UpdatedAt             time.Time   `json:"updatedAt"`
}

// QuotaStore atomically persists LocalQuotaState to spoolDir/quota_state.json
// so the agent can enforce autonomous circuit breaking when Master is unreachable.
type QuotaStore struct {
	mu    sync.Mutex
	dir   string
	path  string
	state LocalQuotaState
	now   func() time.Time
}

// NewQuotaStore creates a QuotaStore backed by quota_state.json inside dir and loads any existing state.
func NewQuotaStore(dir string) (*QuotaStore, error) {
	if dir == "" {
		return nil, errors.New("spool dir is required")
	}
	s := &QuotaStore{
		dir:  dir,
		path: filepath.Join(dir, quotaStateFileName),
		now:  time.Now,
	}
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// CalcTrafficByMode calculates counted bytes from (up, down) according to mode and multiplier.
func CalcTrafficByMode(up, down int64, mode string, multiplier float64) int64 {
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	var base int64
	switch mode {
	case TrafficCountModeOutbound:
		base = up
	case TrafficCountModeMax:
		if up >= down {
			base = up
		} else {
			base = down
		}
	default:
		base = up + down
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return int64(math.Round(float64(base) * multiplier))
}

// State returns a thread-safe snapshot of the current LocalQuotaState.
func (s *QuotaStore) State() LocalQuotaState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// UpdatePolicy refreshes the Master quota baseline, resets offline pending usage, and persists to disk.
func (s *QuotaStore) UpdatePolicy(p QuotaPolicy, masterBroken bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Policy = p
	s.state.LocalPendingUsedBytes = 0
	s.state.LocalCircuitBroken = masterBroken
	s.state.UpdatedAt = s.now().UTC()
	return s.saveLocked()
}

// RecordOfflineUsage accumulates offline traffic delta using the stored policy, persists it,
// and returns the new total cycle+pending bytes and whether the autonomous break threshold is reached.
func (s *QuotaStore) RecordOfflineUsage(up, down int64) (totalUsed int64, shouldBreak bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delta := CalcTrafficByMode(up, down, s.state.Policy.CountMode, s.state.Policy.Multiplier)
	if delta > 0 {
		s.state.LocalPendingUsedBytes += delta
		s.state.UpdatedAt = s.now().UTC()
		if saveErr := s.saveLocked(); saveErr != nil {
			total := s.state.Policy.CycleUsedBytes + s.state.LocalPendingUsedBytes
			return total, s.shouldBreakLocked(), saveErr
		}
	}

	total := s.state.Policy.CycleUsedBytes + s.state.LocalPendingUsedBytes
	return total, s.shouldBreakLocked(), nil
}

// ShouldBreak reports whether the persisted policy + offline usage has reached the hard circuit-breaker threshold.
func (s *QuotaStore) ShouldBreak() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shouldBreakLocked()
}

// MarkBroken records that the agent has autonomously applied the circuit-breaker config.
func (s *QuotaStore) MarkBroken() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.LocalCircuitBroken {
		return nil
	}
	s.state.LocalCircuitBroken = true
	s.state.UpdatedAt = s.now().UTC()
	return s.saveLocked()
}

// ClearBroken clears the local circuit-broken flag when Master confirms a non-broken state.
func (s *QuotaStore) ClearBroken() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.state.LocalCircuitBroken {
		return nil
	}
	s.state.LocalCircuitBroken = false
	s.state.UpdatedAt = s.now().UTC()
	return s.saveLocked()
}

func (s *QuotaStore) shouldBreakLocked() bool {
	p := s.state.Policy
	if p.TrafficLimitBytes <= 0 {
		return false
	}
	pct := p.BreakThresholdPct
	if pct <= 0 {
		pct = 95
	}
	totalUsed := p.CycleUsedBytes + s.state.LocalPendingUsedBytes
	if totalUsed < 0 {
		return false
	}
	if totalUsed <= math.MaxInt64/100 && p.TrafficLimitBytes <= math.MaxInt64/int64(pct) {
		return totalUsed*100 >= p.TrafficLimitBytes*int64(pct)
	}
	return float64(totalUsed)*100 >= float64(p.TrafficLimitBytes)*float64(pct)
}

func (s *QuotaStore) loadLocked() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read quota state file: %w", err)
	}

	var st LocalQuotaState
	if unmarshalErr := json.Unmarshal(data, &st); unmarshalErr != nil {
		corruptPath := s.path + ".corrupt"
		_ = os.Remove(corruptPath)
		if renameErr := os.Rename(s.path, corruptPath); renameErr != nil {
			_ = os.Remove(s.path)
		}
		slog.Warn("Quarantined corrupted quota state file", slog.String("path", s.path), slog.String("error", unmarshalErr.Error()))
		return nil
	}

	s.state = st
	return nil
}

func (s *QuotaStore) saveLocked() error {
	data, err := json.Marshal(s.state)
	if err != nil {
		return fmt.Errorf("marshal quota state: %w", err)
	}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create quota state dir: %w", err)
	}

	tmpPath := s.path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open quota state tmp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write quota state tmp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync quota state tmp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close quota state tmp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename quota state file: %w", err)
	}
	return nil
}
