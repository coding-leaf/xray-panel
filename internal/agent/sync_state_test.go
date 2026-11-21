package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncStateStore_SaveLoadAndCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSyncStateStore(dir)
	if err != nil {
		t.Fatalf("NewSyncStateStore: %v", err)
	}

	if _, ok, err := store.Load(); err != nil || ok {
		t.Fatalf("expected missing state (ok=false, err=nil), got ok=%v err=%v", ok, err)
	}

	want := SyncState{
		ConfigHash:      "cfg-1",
		UsersHash:       "usr-1",
		FullConfigHash:  "full-1",
		HasDesiredUsers: true,
		DesiredUsers: []RemoteUser{
			{InboundTag: "vless-1", Protocol: "vless", Email: "alice@example.com", UUID: "u1", Flow: "xtls-rprx-vision"},
		},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, ok, err := store.Load()
	if err != nil || !ok {
		t.Fatalf("Load failed: ok=%v err=%v", ok, err)
	}
	if got.ConfigHash != want.ConfigHash || got.UsersHash != want.UsersHash || got.FullConfigHash != want.FullConfigHash || !got.HasDesiredUsers {
		t.Fatalf("unexpected loaded state: %+v", got)
	}
	if len(got.DesiredUsers) != 1 || got.DesiredUsers[0] != want.DesiredUsers[0] {
		t.Fatalf("unexpected desired users: %+v", got.DesiredUsers)
	}

	// Corrupt file must be quarantined and treated as uninitialized without error.
	statePath := filepath.Join(dir, syncStateFileName)
	if err := os.WriteFile(statePath, []byte("{corrupt"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	_, ok, err = store.Load()
	if err != nil || ok {
		t.Fatalf("expected corrupt state to reset safely (ok=false, err=nil), got ok=%v err=%v", ok, err)
	}
	if _, statErr := os.Stat(statePath + ".corrupt"); statErr != nil {
		t.Fatalf("expected corrupt state quarantined to %s.corrupt: %v", statePath, statErr)
	}
}

func TestSyncStateStore_InMemoryFallback(t *testing.T) {
	store, err := NewSyncStateStore("")
	if err != nil {
		t.Fatalf("NewSyncStateStore empty: %v", err)
	}
	if _, ok, err := store.Load(); err != nil || ok {
		t.Fatalf("expected empty in-memory store, got ok=%v err=%v", ok, err)
	}
	if err := store.Save(SyncState{ConfigHash: "c1", FullConfigHash: "f1"}); err != nil {
		t.Fatalf("Save in-memory: %v", err)
	}
	got, ok, err := store.Load()
	if err != nil || !ok || got.ConfigHash != "c1" {
		t.Fatalf("Load in-memory unexpected: got=%+v ok=%v err=%v", got, ok, err)
	}
}
