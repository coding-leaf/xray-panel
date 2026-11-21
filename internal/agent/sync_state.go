package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const syncStateFileName = "sync_state.json"

// SyncState mirrors the last converged Master/Xray sync snapshot on disk so an agent restart
// avoids unnecessary Xray reloads when disk config.json and runtime already match Master.
type SyncState struct {
	ConfigHash      string       `json:"configHash"`
	UsersHash       string       `json:"usersHash"`
	FullConfigHash  string       `json:"fullConfigHash"`
	DesiredUsers    []RemoteUser `json:"desiredUsers,omitempty"`
	HasDesiredUsers bool         `json:"hasDesiredUsers,omitempty"`
	CircuitBroken   bool         `json:"circuitBroken"`
	UpdatedAt       int64        `json:"updatedAt"`
}

// SyncStateStore atomically persists SyncState to spoolDir/sync_state.json (or stays in-memory if dir == "").
type SyncStateStore struct {
	mu   sync.Mutex
	dir  string
	path string
	now  func() time.Time
	mem  *SyncState
}

// NewSyncStateStore creates a SyncStateStore inside dir, or an in-memory store when dir is empty.
func NewSyncStateStore(dir string) (*SyncStateStore, error) {
	if dir == "" {
		return &SyncStateStore{now: time.Now}, nil
	}
	return &SyncStateStore{
		dir:  dir,
		path: filepath.Join(dir, syncStateFileName),
		now:  time.Now,
	}, nil
}

// Load reads SyncState from disk (or memory). Missing or corrupted files return (zero, false, nil)
// so the caller safely falls back to pulling authoritative state from Master.
func (s *SyncStateStore) Load() (SyncState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		if s.mem == nil {
			return SyncState{}, false, nil
		}
		return cloneSyncState(*s.mem), true, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SyncState{}, false, nil
		}
		return SyncState{}, false, fmt.Errorf("read sync state file: %w", err)
	}

	var st SyncState
	if unmarshalErr := json.Unmarshal(data, &st); unmarshalErr != nil {
		corruptPath := s.path + ".corrupt"
		_ = os.Remove(corruptPath)
		if renameErr := os.Rename(s.path, corruptPath); renameErr != nil {
			_ = os.Remove(s.path)
		}
		slog.Warn("Quarantined corrupted sync state file", slog.String("path", s.path), slog.String("error", unmarshalErr.Error()))
		return SyncState{}, false, nil
	}
	return cloneSyncState(st), true, nil
}

// Save atomically persists st to disk (or memory when dir == "").
func (s *SyncStateStore) Save(st SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st = cloneSyncState(st)
	if st.UpdatedAt == 0 {
		st.UpdatedAt = s.now().Unix()
	}

	if s.path == "" {
		copied := cloneSyncState(st)
		s.mem = &copied
		return nil
	}

	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal sync state: %w", err)
	}

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create sync state dir: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := writeFileAtomicSync(tmpPath, data, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write sync state tmp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename sync state file: %w", err)
	}
	return nil
}

func cloneSyncState(st SyncState) SyncState {
	if st.DesiredUsers != nil {
		usersCopy := make([]RemoteUser, len(st.DesiredUsers))
		copy(usersCopy, st.DesiredUsers)
		st.DesiredUsers = usersCopy
	}
	return st
}
