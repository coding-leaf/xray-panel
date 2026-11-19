package agent

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSpool_SaveLoadAck(t *testing.T) {
	dir := t.TempDir()
	spool, err := NewSpool(dir)
	if err != nil {
		t.Fatalf("NewSpool failed: %v", err)
	}

	loaded, err := spool.Load()
	if err != nil {
		t.Fatalf("Load on empty dir returned error: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil batch on empty dir, got %+v", loaded)
	}

	want := &TrafficBatch{
		BatchID: "20261119T101500Z-abc123",
		Users: []UserTrafficDelta{
			{Email: "alice@example.com", UpBytes: 1024, DownBytes: 2048},
		},
		Inbounds: []InboundTrafficDelta{
			{Tag: "us-vless", UpBytes: 1024, DownBytes: 2048},
		},
	}
	if err := spool.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := spool.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil || got.BatchID != want.BatchID || len(got.Users) != 1 || got.Users[0].DownBytes != 2048 {
		t.Fatalf("unexpected loaded batch: %+v", got)
	}

	// Ack with mismatched batchID must NOT delete the current batch.
	if err := spool.Ack("other-batch-id"); err != nil {
		t.Fatalf("Ack mismatched failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, spoolFileName)); err != nil {
		t.Fatalf("expected spool file to remain after mismatched ack, got: %v", err)
	}

	// Ack with matching batchID must delete the file.
	if err := spool.Ack(want.BatchID); err != nil {
		t.Fatalf("Ack failed: %v", err)
	}
	loaded, err = spool.Load()
	if err != nil {
		t.Fatalf("Load after Ack failed: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil batch after Ack, got %+v", loaded)
	}
}

func TestSpool_CorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	spool, err := NewSpool(dir)
	if err != nil {
		t.Fatalf("NewSpool failed: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, spoolFileName), []byte("{corrupt json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	got, err := spool.Load()
	if err == nil {
		t.Fatalf("expected error for corrupt spool file")
	}
	if got != nil {
		t.Fatalf("expected nil batch for corrupt spool file")
	}

	// Subsequent Load should be clean because corrupt file was quarantined.
	got, err = spool.Load()
	if err != nil {
		t.Fatalf("expected clean Load after quarantine, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil batch after quarantine")
	}
}

func TestSpool_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	spool, err := NewSpool(dir)
	if err != nil {
		t.Fatalf("NewSpool failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			batch := &TrafficBatch{BatchID: NewBatchID()}
			_ = spool.Save(batch)
			_, _ = spool.Load()
			_ = spool.Ack(batch.BatchID)
		}(i)
	}
	wg.Wait()
}
