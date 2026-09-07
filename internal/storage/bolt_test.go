package storage_test

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"panel/internal/domain"
	"panel/internal/storage"

	"go.etcd.io/bbolt"
)

func newTestStorage(t *testing.T, opts ...storage.Option) (*storage.BoltStorage, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_traffic.db")
	s, err := storage.Open(dbPath, opts...)
	if err != nil {
		t.Fatalf("failed to open test storage: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	return s, dbPath
}

func TestOpenClose(t *testing.T) {
	t.Run("auto create parent directories", func(t *testing.T) {
		nestedPath := filepath.Join(t.TempDir(), "nested", "dir", "traffic.db")
		s, err := storage.Open(nestedPath)
		if err != nil {
			t.Fatalf("expected Open to create parent dirs, got error: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("failed to close storage: %v", err)
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		_, err := storage.Open("")
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
	})

	t.Run("idempotent close", func(t *testing.T) {
		s, _ := newTestStorage(t)
		if err := s.Close(); err != nil {
			t.Fatalf("first close failed: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("second close failed: %v", err)
		}
	})

	t.Run("operations after close return error", func(t *testing.T) {
		s, _ := newTestStorage(t)
		_ = s.Close()

		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"u1@test.com": {Uplink: 10, Downlink: 20},
		}); !errors.Is(err, storage.ErrDatabaseClosed) {
			t.Fatalf("expected ErrDatabaseClosed, got: %v", err)
		}

		if _, err := s.GetUserTraffic("u1@test.com"); !errors.Is(err, storage.ErrDatabaseClosed) {
			t.Fatalf("expected ErrDatabaseClosed, got: %v", err)
		}

		if _, err := s.GetAllUsersTraffic(); !errors.Is(err, storage.ErrDatabaseClosed) {
			t.Fatalf("expected ErrDatabaseClosed, got: %v", err)
		}

		if _, err := s.GetTrafficHistory("u1@test.com", 0, 100); !errors.Is(err, storage.ErrDatabaseClosed) {
			t.Fatalf("expected ErrDatabaseClosed, got: %v", err)
		}
	})
}

func TestBucketsInitialized(t *testing.T) {
	s, _ := newTestStorage(t)

	err := s.DB().View(func(tx *bbolt.Tx) error {
		if tx.Bucket([]byte(storage.BucketUsersTraffic)) == nil {
			return fmt.Errorf("bucket %s missing", storage.BucketUsersTraffic)
		}
		if tx.Bucket([]byte(storage.BucketTrafficHistory)) == nil {
			return fmt.Errorf("bucket %s missing", storage.BucketTrafficHistory)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("bucket verification failed: %v", err)
	}
}

func TestSaveTrafficBatchAndGetUserTraffic(t *testing.T) {
	s, _ := newTestStorage(t)

	t.Run("empty batch returns nil", func(t *testing.T) {
		if err := s.SaveTrafficBatch(nil); err != nil {
			t.Fatalf("unexpected error for nil batch: %v", err)
		}
		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{}); err != nil {
			t.Fatalf("unexpected error for empty batch: %v", err)
		}
	})

	t.Run("invalid inputs", func(t *testing.T) {
		// Empty email
		err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"": {Uplink: 10, Downlink: 10},
		})
		if !errors.Is(err, storage.ErrEmptyEmail) {
			t.Fatalf("expected ErrEmptyEmail, got %v", err)
		}

		// Email with null byte
		err = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"user\x00@test.com": {Uplink: 10, Downlink: 10},
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for null byte in email, got %v", err)
		}

		// Negative uplink
		err = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"user@test.com": {Uplink: -1, Downlink: 10},
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for negative uplink, got %v", err)
		}

		// Negative downlink
		err = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"user@test.com": {Uplink: 10, Downlink: -5},
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for negative downlink, got %v", err)
		}

		// Negative timestamp
		err = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			"user@test.com": {Uplink: 10, Downlink: 10, Timestamp: -100},
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for negative timestamp, got %v", err)
		}
	})

	t.Run("single user accumulation and lastSeen", func(t *testing.T) {
		email := "alice@test.com"

		// Not found initially
		_, err := s.GetUserTraffic(email)
		if !errors.Is(err, storage.ErrUserNotFound) || !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("expected ErrUserNotFound/ErrNotFound, got %v", err)
		}

		// First batch
		batch1 := map[string]storage.TrafficDelta{
			email: {Uplink: 1000, Downlink: 2000, Timestamp: 100},
		}
		if err := s.SaveTrafficBatch(batch1); err != nil {
			t.Fatalf("SaveTrafficBatch batch1 failed: %v", err)
		}

		traffic, err := s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.Email != email || traffic.Uplink != 1000 || traffic.Downlink != 2000 || traffic.LastSeen != 100 {
			t.Fatalf("unexpected traffic state: %+v", traffic)
		}

		// Second batch (cumulative increment)
		batch2 := map[string]storage.TrafficDelta{
			email: {Uplink: 500, Downlink: 1500, Timestamp: 200},
		}
		if err := s.SaveTrafficBatch(batch2); err != nil {
			t.Fatalf("SaveTrafficBatch batch2 failed: %v", err)
		}

		traffic, err = s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.Uplink != 1500 || traffic.Downlink != 3500 || traffic.LastSeen != 200 {
			t.Fatalf("unexpected traffic after batch2: %+v", traffic)
		}

		// Batch with older timestamp should NOT rewind LastSeen
		batch3 := map[string]storage.TrafficDelta{
			email: {Uplink: 100, Downlink: 100, Timestamp: 150},
		}
		if err := s.SaveTrafficBatch(batch3); err != nil {
			t.Fatalf("SaveTrafficBatch batch3 failed: %v", err)
		}

		traffic, err = s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.Uplink != 1600 || traffic.Downlink != 3600 || traffic.LastSeen != 200 {
			t.Fatalf("expected LastSeen to remain 200, got %+v", traffic)
		}
	})

	t.Run("default timestamp when 0", func(t *testing.T) {
		email := "bob@test.com"
		before := time.Now().Unix() - 1

		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			email: {Uplink: 200, Downlink: 300, Timestamp: 0},
		}); err != nil {
			t.Fatalf("SaveTrafficBatch failed: %v", err)
		}
		after := time.Now().Unix() + 1

		traffic, err := s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.LastSeen < before || traffic.LastSeen > after {
			t.Fatalf("expected LastSeen around now (%d..%d), got %d", before, after, traffic.LastSeen)
		}
	})

	t.Run("zero traffic delta does not update lastSeen", func(t *testing.T) {
		email := "idle@test.com"

		// Initial traffic at t=500
		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			email: {Uplink: 100, Downlink: 100, Timestamp: 500},
		}); err != nil {
			t.Fatalf("SaveTrafficBatch failed: %v", err)
		}

		// Subsequent delta at t=900 with 0 bytes transferred
		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			email: {Uplink: 0, Downlink: 0, Timestamp: 900},
		}); err != nil {
			t.Fatalf("SaveTrafficBatch failed: %v", err)
		}

		traffic, err := s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.LastSeen != 500 {
			t.Fatalf("expected LastSeen to remain 500 for zero-traffic delta, got %d", traffic.LastSeen)
		}
	})

	t.Run("integer overflow saturates safely", func(t *testing.T) {
		email := "overflow@test.com"

		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			email: {Uplink: math.MaxInt64 - 10, Downlink: math.MaxInt64 - 10, Timestamp: 100},
		}); err != nil {
			t.Fatalf("SaveTrafficBatch failed: %v", err)
		}

		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			email: {Uplink: 100, Downlink: 100, Timestamp: 200},
		}); err != nil {
			t.Fatalf("SaveTrafficBatch failed: %v", err)
		}

		traffic, err := s.GetUserTraffic(email)
		if err != nil {
			t.Fatalf("GetUserTraffic failed: %v", err)
		}
		if traffic.Uplink != math.MaxInt64 || traffic.Downlink != math.MaxInt64 {
			t.Fatalf("expected saturating MaxInt64, got (%d, %d)", traffic.Uplink, traffic.Downlink)
		}
	})
}

func TestGetAllUsersTraffic(t *testing.T) {
	s, _ := newTestStorage(t)

	// Initially empty
	all, err := s.GetAllUsersTraffic()
	if err != nil {
		t.Fatalf("GetAllUsersTraffic failed: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 users, got %d", len(all))
	}

	// Insert 3 users
	batch := map[string]storage.TrafficDelta{
		"u1@test.com": {Uplink: 10, Downlink: 20, Timestamp: 100},
		"u2@test.com": {Uplink: 30, Downlink: 40, Timestamp: 200},
		"u3@test.com": {Uplink: 50, Downlink: 60, Timestamp: 300},
	}
	if err := s.SaveTrafficBatch(batch); err != nil {
		t.Fatalf("SaveTrafficBatch failed: %v", err)
	}

	all, err = s.GetAllUsersTraffic()
	if err != nil {
		t.Fatalf("GetAllUsersTraffic failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 users, got %d", len(all))
	}

	userMap := make(map[string]*storage.UserTraffic)
	for _, u := range all {
		userMap[u.Email] = u
	}

	if u1, ok := userMap["u1@test.com"]; !ok || u1.Uplink != 10 || u1.Downlink != 20 {
		t.Fatalf("unexpected u1: %+v", u1)
	}
	if u2, ok := userMap["u2@test.com"]; !ok || u2.Uplink != 30 || u2.Downlink != 40 {
		t.Fatalf("unexpected u2: %+v", u2)
	}
	if u3, ok := userMap["u3@test.com"]; !ok || u3.Uplink != 50 || u3.Downlink != 60 {
		t.Fatalf("unexpected u3: %+v", u3)
	}
}

func TestGetTrafficHistory(t *testing.T) {
	s, _ := newTestStorage(t)

	emailAlice := "alice@test.com"
	emailAliceSub := "alice@test.com.extra" // Check prefix isolation

	// Add historical data points for alice: timestamps 1000, 2000, 3000
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		emailAlice:    {Uplink: 100, Downlink: 200, Timestamp: 1000},
		emailAliceSub: {Uplink: 999, Downlink: 999, Timestamp: 2000},
	})
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		emailAlice: {Uplink: 300, Downlink: 400, Timestamp: 2000},
	})
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		emailAlice: {Uplink: 500, Downlink: 600, Timestamp: 3000},
	})

	t.Run("full range query", func(t *testing.T) {
		history, err := s.GetTrafficHistory(emailAlice, 500, 3500)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(history) != 3 {
			t.Fatalf("expected 3 snapshots, got %d", len(history))
		}
		if history[0].Timestamp != 1000 || history[0].Uplink != 100 || history[0].Downlink != 200 {
			t.Fatalf("unexpected history[0]: %+v", history[0])
		}
		if history[1].Timestamp != 2000 || history[1].Uplink != 300 || history[1].Downlink != 400 {
			t.Fatalf("unexpected history[1]: %+v", history[1])
		}
		if history[2].Timestamp != 3000 || history[2].Uplink != 500 || history[2].Downlink != 600 {
			t.Fatalf("unexpected history[2]: %+v", history[2])
		}
	})

	t.Run("sub range middle query", func(t *testing.T) {
		history, err := s.GetTrafficHistory(emailAlice, 1500, 2500)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 snapshot, got %d", len(history))
		}
		if history[0].Timestamp != 2000 {
			t.Fatalf("expected timestamp 2000, got %d", history[0].Timestamp)
		}
	})

	t.Run("exact single point match", func(t *testing.T) {
		history, err := s.GetTrafficHistory(emailAlice, 2000, 2000)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(history) != 1 || history[0].Timestamp != 2000 {
			t.Fatalf("expected 1 snapshot at 2000, got %+v", history)
		}
	})

	t.Run("out of range query returns empty slice", func(t *testing.T) {
		history, err := s.GetTrafficHistory(emailAlice, 4000, 5000)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(history) != 0 {
			t.Fatalf("expected 0 snapshots, got %d", len(history))
		}
	})

	t.Run("prefix isolation test", func(t *testing.T) {
		historySub, err := s.GetTrafficHistory(emailAliceSub, 0, 5000)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(historySub) != 1 {
			t.Fatalf("expected exactly 1 snapshot for sub-email, got %d", len(historySub))
		}
		if historySub[0].Uplink != 999 {
			t.Fatalf("unexpected sub snapshot: %+v", historySub[0])
		}
	})

	t.Run("same timestamp accumulates in history", func(t *testing.T) {
		emailBob := "bob_hist@test.com"
		ts := int64(5000)

		_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			emailBob: {Uplink: 100, Downlink: 200, Timestamp: ts},
		})
		_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			emailBob: {Uplink: 50, Downlink: 75, Timestamp: ts},
		})

		history, err := s.GetTrafficHistory(emailBob, ts-10, ts+10)
		if err != nil {
			t.Fatalf("GetTrafficHistory failed: %v", err)
		}
		if len(history) != 1 {
			t.Fatalf("expected 1 accumulated snapshot, got %d", len(history))
		}
		if history[0].Uplink != 150 || history[0].Downlink != 275 {
			t.Fatalf("expected accumulated (150, 275), got (%d, %d)", history[0].Uplink, history[0].Downlink)
		}
	})

	t.Run("invalid arguments error", func(t *testing.T) {
		// Empty email
		_, err := s.GetTrafficHistory("", 100, 200)
		if !errors.Is(err, storage.ErrEmptyEmail) {
			t.Fatalf("expected ErrEmptyEmail, got %v", err)
		}

		// startTime > endTime
		_, err = s.GetTrafficHistory(emailAlice, 300, 200)
		if !errors.Is(err, storage.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
		}

		// startTime < 0
		_, err = s.GetTrafficHistory(emailAlice, -10, 200)
		if !errors.Is(err, storage.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange for negative startTime, got %v", err)
		}

		// endTime < 0
		_, err = s.GetTrafficHistory(emailAlice, 10, -5)
		if !errors.Is(err, storage.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange for negative endTime, got %v", err)
		}

		// both negative
		_, err = s.GetTrafficHistory(emailAlice, -50, -10)
		if !errors.Is(err, storage.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange for negative time range, got %v", err)
		}

		// null byte in email
		_, err = s.GetTrafficHistory("user\x00@test.com", 100, 200)
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for null byte in email, got %v", err)
		}
	})
}

func TestHourlyRollupOption(t *testing.T) {
	s, _ := newTestStorage(t, storage.WithHourlyRollup(true))

	email := "rollup@test.com"
	hour1Base := int64(3600 * 10) // 10:00:00

	// Save three deltas in hour 10: +0s, +500s, +3500s
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 100, Downlink: 100, Timestamp: hour1Base},
	})
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 200, Downlink: 300, Timestamp: hour1Base + 500},
	})
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 300, Downlink: 400, Timestamp: hour1Base + 3500},
	})

	// Save one delta in hour 11
	hour2Base := int64(3600 * 11)
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 1000, Downlink: 1000, Timestamp: hour2Base + 100},
	})

	history, err := s.GetTrafficHistory(email, hour1Base-100, hour2Base+3700)
	if err != nil {
		t.Fatalf("GetTrafficHistory failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("expected 2 hourly aggregated snapshots, got %d", len(history))
	}

	if history[0].Timestamp != hour1Base || history[0].Uplink != 600 || history[0].Downlink != 800 {
		t.Fatalf("unexpected hour 10 rollup: %+v", history[0])
	}

	if history[1].Timestamp != hour2Base || history[1].Uplink != 1000 || history[1].Downlink != 1000 {
		t.Fatalf("unexpected hour 11 rollup: %+v", history[1])
	}
}

func TestSerializerInteroperability(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "interop.db")

	emailJSON := "json_user@test.com"
	emailBin := "bin_user@test.com"

	// 1. Write using JSON serializer
	{
		s, err := storage.Open(dbPath, storage.WithSerializer(storage.SerializerJSON))
		if err != nil {
			t.Fatalf("failed to open with JSON serializer: %v", err)
		}
		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			emailJSON: {Uplink: 100, Downlink: 200, Timestamp: 1000},
		}); err != nil {
			t.Fatalf("failed to save with JSON: %v", err)
		}
		_ = s.Close()
	}

	// 2. Reopen using Binary serializer and verify JSON record is readable, then write binary record
	{
		s, err := storage.Open(dbPath, storage.WithSerializer(storage.SerializerBinary))
		if err != nil {
			t.Fatalf("failed to reopen with Binary serializer: %v", err)
		}

		u, err := s.GetUserTraffic(emailJSON)
		if err != nil {
			t.Fatalf("binary reader failed to read JSON record: %v", err)
		}
		if u.Uplink != 100 || u.Downlink != 200 {
			t.Fatalf("unexpected user data decoded from JSON: %+v", u)
		}

		// Write binary record
		if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
			emailBin: {Uplink: 500, Downlink: 600, Timestamp: 2000},
		}); err != nil {
			t.Fatalf("failed to save binary record: %v", err)
		}

		// Both should be readable
		all, err := s.GetAllUsersTraffic()
		if err != nil {
			t.Fatalf("failed to GetAllUsersTraffic: %v", err)
		}
		if len(all) != 2 {
			t.Fatalf("expected 2 users, got %d", len(all))
		}

		_ = s.Close()
	}

	// 3. Reopen using JSON serializer and verify binary record is also readable
	{
		s, err := storage.Open(dbPath, storage.WithSerializer(storage.SerializerJSON))
		if err != nil {
			t.Fatalf("failed to reopen with JSON serializer: %v", err)
		}

		u, err := s.GetUserTraffic(emailBin)
		if err != nil {
			t.Fatalf("JSON reader failed to read Binary record: %v", err)
		}
		if u.Uplink != 500 || u.Downlink != 600 {
			t.Fatalf("unexpected user data decoded from Binary: %+v", u)
		}

		_ = s.Close()
	}
}

func TestConcurrentAccess(t *testing.T) {
	s, _ := newTestStorage(t)

	const numWriters = 8
	const numReaders = 8
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, (numWriters+numReaders)*iterations)

	// Launch concurrent writers
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				email := fmt.Sprintf("user_%d@stress.test", writerID)
				delta := storage.TrafficDelta{
					Uplink:    10,
					Downlink:  20,
					Timestamp: int64(1000 + i),
				}
				if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{email: delta}); err != nil {
					errCh <- fmt.Errorf("writer %d iter %d: %w", writerID, i, err)
					return
				}
			}
		}(w)
	}

	// Launch concurrent readers
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				email := fmt.Sprintf("user_%d@stress.test", readerID%numWriters)
				_, _ = s.GetUserTraffic(email)
				_, _ = s.GetAllUsersTraffic()
				_, _ = s.GetTrafficHistory(email, 1000, 2000)
			}
		}(r)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent test error: %v", err)
	}

	// Verify all writes succeeded and totals match exactly
	all, err := s.GetAllUsersTraffic()
	if err != nil {
		t.Fatalf("GetAllUsersTraffic after stress failed: %v", err)
	}
	if len(all) != numWriters {
		t.Fatalf("expected %d users, got %d", numWriters, len(all))
	}

	for _, u := range all {
		expectedUp := int64(10 * iterations)
		expectedDown := int64(20 * iterations)
		if u.Uplink != expectedUp || u.Downlink != expectedDown {
			t.Fatalf("user %s traffic mismatch: expected (%d, %d), got (%d, %d)",
				u.Email, expectedUp, expectedDown, u.Uplink, u.Downlink)
		}
	}
}

func TestNilReceiverSafety(t *testing.T) {
	var s *storage.BoltStorage

	if err := s.Close(); err != nil {
		t.Fatalf("expected nil error on nil Close, got %v", err)
	}
	if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{"a@b.com": {Uplink: 1}}); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil SaveTrafficBatch, got %v", err)
	}
	if _, err := s.GetUserTraffic("a@b.com"); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil GetUserTraffic, got %v", err)
	}
	if _, err := s.GetAllUsersTraffic(); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil GetAllUsersTraffic, got %v", err)
	}
	if _, err := s.GetTrafficHistory("a@b.com", 0, 100); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil GetTrafficHistory, got %v", err)
	}
	if err := s.ResetUserTraffic("a@b.com"); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil ResetUserTraffic, got %v", err)
	}
	if err := s.DeleteUserTraffic("a@b.com"); !errors.Is(err, storage.ErrDatabaseClosed) {
		t.Fatalf("expected ErrDatabaseClosed on nil DeleteUserTraffic, got %v", err)
	}
	if s.DB() != nil {
		t.Fatalf("expected nil DB() on nil storage")
	}
	if s.Path() != "" {
		t.Fatalf("expected empty Path() on nil storage")
	}
}

func TestResetAndDeleteUserTraffic(t *testing.T) {
	s, dbPath := newTestStorage(t)

	if s.Path() != dbPath {
		t.Fatalf("expected path %s, got %s", dbPath, s.Path())
	}

	email := "lifecycle_user@test.com"

	// 1. Initial traffic
	if err := s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 5000, Downlink: 8000, Timestamp: 1000},
	}); err != nil {
		t.Fatalf("SaveTrafficBatch failed: %v", err)
	}

	u, err := s.GetUserTraffic(email)
	if err != nil || u.Uplink != 5000 || u.Downlink != 8000 || u.LastSeen != 1000 {
		t.Fatalf("unexpected user state: %+v, err: %v", u, err)
	}

	// 2. Reset traffic
	if err := s.ResetUserTraffic(email); err != nil {
		t.Fatalf("ResetUserTraffic failed: %v", err)
	}

	u, err = s.GetUserTraffic(email)
	if err != nil {
		t.Fatalf("GetUserTraffic failed: %v", err)
	}
	if u.Uplink != 0 || u.Downlink != 0 {
		t.Fatalf("expected 0 traffic after reset, got %+v", u)
	}
	if u.LastSeen != 1000 {
		t.Fatalf("expected LastSeen to remain 1000 after reset, got %d", u.LastSeen)
	}

	// History should still be intact after reset
	hist, err := s.GetTrafficHistory(email, 0, 2000)
	if err != nil || len(hist) != 1 {
		t.Fatalf("expected 1 history record after reset, got %d, err: %v", len(hist), err)
	}

	// 3. Delete user traffic
	if err := s.DeleteUserTraffic(email); err != nil {
		t.Fatalf("DeleteUserTraffic failed: %v", err)
	}

	// User stats should now return ErrUserNotFound
	if _, err := s.GetUserTraffic(email); !errors.Is(err, storage.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound after delete, got %v", err)
	}

	// History should now be empty
	hist, err = s.GetTrafficHistory(email, 0, 2000)
	if err != nil || len(hist) != 0 {
		t.Fatalf("expected 0 history records after delete, got %d, err: %v", len(hist), err)
	}

	// Reset / Delete on non-existent user returns ErrUserNotFound
	if err := s.ResetUserTraffic("ghost@test.com"); !errors.Is(err, storage.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound for ghost user reset, got %v", err)
	}
	if err := s.DeleteUserTraffic("ghost@test.com"); !errors.Is(err, storage.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound for ghost user delete, got %v", err)
	}

	// Invalid arguments
	if err := s.ResetUserTraffic(""); !errors.Is(err, storage.ErrEmptyEmail) {
		t.Fatalf("expected ErrEmptyEmail for reset, got %v", err)
	}
	if err := s.DeleteUserTraffic(""); !errors.Is(err, storage.ErrEmptyEmail) {
		t.Fatalf("expected ErrEmptyEmail for delete, got %v", err)
	}
	if err := s.ResetUserTraffic("bad\x00@test.com"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for reset with null byte, got %v", err)
	}
	if err := s.DeleteUserTraffic("bad\x00@test.com"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for delete with null byte, got %v", err)
	}
}

func TestCustomRollupInterval(t *testing.T) {
	// 10-minute rollup interval (600 seconds)
	s, _ := newTestStorage(t, storage.WithRollupInterval(10*time.Minute))

	email := "rollup10@test.com"

	// Minute 2 (120s) and Minute 5 (300s) should fall into the Minute 0 (0s) bucket
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 100, Downlink: 200, Timestamp: 120},
	})
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 300, Downlink: 400, Timestamp: 300},
	})

	// Minute 12 (720s) should fall into the Minute 10 (600s) bucket
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 500, Downlink: 600, Timestamp: 720},
	})

	hist, err := s.GetTrafficHistory(email, 0, 1000)
	if err != nil {
		t.Fatalf("GetTrafficHistory failed: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 10-min rollup buckets, got %d", len(hist))
	}

	if hist[0].Timestamp != 0 || hist[0].Uplink != 400 || hist[0].Downlink != 600 {
		t.Fatalf("unexpected bucket 0 rollup: %+v", hist[0])
	}
	if hist[1].Timestamp != 600 || hist[1].Uplink != 500 || hist[1].Downlink != 600 {
		t.Fatalf("unexpected bucket 1 rollup: %+v", hist[1])
	}
}

func TestSnapshotEmailPreservedInAccumulation(t *testing.T) {
	s, _ := newTestStorage(t)

	email := "accum_email@test.com"

	// First delta at t=1000
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 100, Downlink: 100, Timestamp: 1000},
	})

	// Second delta at same t=1000 (triggers existingHistBytes accumulation path)
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 200, Downlink: 200, Timestamp: 1000},
	})

	hist, err := s.GetTrafficHistory(email, 900, 1100)
	if err != nil {
		t.Fatalf("GetTrafficHistory failed: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(hist))
	}
	if hist[0].Email != email {
		t.Fatalf("expected Email %q, got %q", email, hist[0].Email)
	}
	if hist[0].Timestamp != 1000 || hist[0].Uplink != 300 || hist[0].Downlink != 300 {
		t.Fatalf("unexpected snapshot data: %+v", hist[0])
	}
}

func TestSubBucketSafety(t *testing.T) {
	s, _ := newTestStorage(t)

	email := "regular@test.com"
	_ = s.SaveTrafficBatch(map[string]storage.TrafficDelta{
		email: {Uplink: 100, Downlink: 200, Timestamp: 1000},
	})

	// Inject a sub-bucket directly into users_traffic and traffic_history
	err := s.DB().Update(func(tx *bbolt.Tx) error {
		ub := tx.Bucket([]byte(storage.BucketUsersTraffic))
		if _, err := ub.CreateBucket([]byte("sub_users_bucket")); err != nil {
			return err
		}
		hb := tx.Bucket([]byte(storage.BucketTrafficHistory))
		if _, err := hb.CreateBucket([]byte("sub_hist_bucket")); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to inject sub-buckets: %v", err)
	}

	// GetAllUsersTraffic should gracefully skip sub-buckets and not error
	all, err := s.GetAllUsersTraffic()
	if err != nil {
		t.Fatalf("GetAllUsersTraffic failed with sub-bucket: %v", err)
	}
	if len(all) != 1 || all[0].Email != email {
		t.Fatalf("unexpected GetAllUsersTraffic result: %+v", all)
	}

	// GetTrafficHistory should also gracefully skip sub-buckets
	hist, err := s.GetTrafficHistory(email, 0, 2000)
	if err != nil {
		t.Fatalf("GetTrafficHistory failed with sub-bucket: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("expected 1 history snapshot, got %d", len(hist))
	}
}
