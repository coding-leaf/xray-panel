package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const spoolFileName = "pending_batch.json"

// Spool persists an unacknowledged TrafficBatch on disk for idempotent retry across restarts or network failures.
type Spool struct {
	mu   sync.Mutex
	dir  string
	path string
}

// NewSpool creates a Spool storing pending_batch.json inside dir.
func NewSpool(dir string) (*Spool, error) {
	if dir == "" {
		return nil, errors.New("spool dir is required")
	}
	return &Spool{
		dir:  dir,
		path: filepath.Join(dir, spoolFileName),
	}, nil
}

// NewBatchID generates a globally unique, time-ordered batch identifier.
func NewBatchID() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", time.Now().UTC().Format("20060102T150405Z"), time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(buf[:]))
}

// Load reads the pending TrafficBatch from disk.
// Returns (nil, nil) when no pending batch exists.
// If the file is corrupted or invalid, it quarantines the file so future ticks are not blocked and returns an error.
func (s *Spool) Load() (*TrafficBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read spool file: %w", err)
	}

	var batch TrafficBatch
	if err := json.Unmarshal(data, &batch); err != nil || batch.BatchID == "" {
		corruptPath := s.path + ".corrupt"
		_ = os.Remove(corruptPath)
		if renameErr := os.Rename(s.path, corruptPath); renameErr != nil {
			_ = os.Remove(s.path)
		}
		if err != nil {
			return nil, fmt.Errorf("corrupted spool file quarantined: %w", err)
		}
		return nil, errors.New("corrupted spool file quarantined: empty batchId")
	}

	return &batch, nil
}

// Save atomically persists batch to pending_batch.json.
func (s *Spool) Save(batch *TrafficBatch) error {
	if batch == nil || batch.BatchID == "" {
		return errors.New("cannot spool nil or empty batchId")
	}

	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal spool batch: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}

	tmpPath := s.path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open spool tmp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write spool tmp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync spool tmp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close spool tmp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename spool file: %w", err)
	}
	return nil
}

// Ack removes the pending batch file once Master has acknowledged batchID.
func (s *Spool) Ack(batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if batchID != "" {
		data, err := os.ReadFile(s.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("read spool before ack: %w", err)
		}
		var existing TrafficBatch
		if err := json.Unmarshal(data, &existing); err == nil && existing.BatchID != "" && existing.BatchID != batchID {
			return nil
		}
	}

	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove spool file: %w", err)
	}
	return nil
}
