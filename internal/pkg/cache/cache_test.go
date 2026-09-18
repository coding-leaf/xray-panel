package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_BasicOperations(t *testing.T) {
	c := New[string](1*time.Hour, 0)
	defer c.Stop()

	// Initial Get
	if _, found := c.Get("key1"); found {
		t.Fatal("expected key1 not found")
	}

	// Set with default TTL
	c.SetDefault("key1", "val1")
	if val, found := c.Get("key1"); !found || val != "val1" {
		t.Fatalf("expected val1, got %v (found: %v)", val, found)
	}

	// Overwrite with custom TTL
	c.Set("key1", "val1_updated", 10*time.Minute)
	if val, found := c.Get("key1"); !found || val != "val1_updated" {
		t.Fatalf("expected val1_updated, got %v", val)
	}

	// ItemCount
	if count := c.ItemCount(); count != 1 {
		t.Fatalf("expected 1 item, got %d", count)
	}

	// Delete
	c.Delete("key1")
	if _, found := c.Get("key1"); found {
		t.Fatal("expected key1 to be deleted")
	}

	// Flush
	c.SetDefault("k1", "v1")
	c.SetDefault("k2", "v2")
	if c.ItemCount() != 2 {
		t.Fatalf("expected 2 items, got %d", c.ItemCount())
	}
	c.Flush()
	if c.ItemCount() != 0 {
		t.Fatalf("expected 0 items after flush, got %d", c.ItemCount())
	}
}

func TestCache_Expiration(t *testing.T) {
	c := New[int](50*time.Millisecond, 20*time.Millisecond)
	defer c.Stop()

	c.SetDefault("expire_default", 42)
	c.Set("expire_quick", 88, 30*time.Millisecond)
	c.Set("never_expire", 100, NoExpiration)

	// Immediate check
	if val, found := c.Get("expire_quick"); !found || val != 88 {
		t.Fatalf("expected 88, got %v", val)
	}

	// Wait for quick to expire
	time.Sleep(60 * time.Millisecond)
	if _, found := c.Get("expire_quick"); found {
		t.Fatal("expected expire_quick to be expired")
	}

	// Wait for default to expire
	time.Sleep(20 * time.Millisecond)
	if _, found := c.Get("expire_default"); found {
		t.Fatal("expected expire_default to be expired")
	}

	// Never expire should still exist
	if val, found := c.Get("never_expire"); !found || val != 100 {
		t.Fatalf("expected 100, got %v", val)
	}
}

func TestCache_Concurrency(t *testing.T) {
	c := New[string](100*time.Millisecond, 50*time.Millisecond)
	defer c.Stop()

	var wg sync.WaitGroup
	workers := 16
	ops := 100

	// Concurrent writers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				key := fmt.Sprintf("key_%d", (workerID*ops+j)%20)
				c.Set(key, fmt.Sprintf("val_%d", workerID), 50*time.Millisecond)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				key := fmt.Sprintf("key_%d", (workerID*ops+j)%20)
				_, _ = c.Get(key)
			}
		}(i)
	}

	// Concurrent deleters
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				key := fmt.Sprintf("key_%d", (workerID*ops+j)%20)
				c.Delete(key)
			}
		}(i)
	}

	wg.Wait()
}
