package cache

import (
	"runtime"
	"sync"
	"time"
)

const (
	// NoExpiration specifies that an item should not expire.
	NoExpiration time.Duration = -1
	// DefaultExpiration specifies using the cache's default expiration.
	DefaultExpiration time.Duration = 0
)

type item[T any] struct {
	val       T
	expiresAt int64 // Unix nano, 0 means no expiration
}

// Cache is a thread-safe in-memory key-value cache with generic values and TTL expiration.
type Cache[T any] struct {
	mu         sync.RWMutex
	defaultTTL time.Duration
	items      map[string]item[T]
	stopChan   chan struct{}
	stopOnce   sync.Once
}

// New creates a new Cache instance with specified default TTL and cleanup interval.
func New[T any](defaultTTL, cleanupInterval time.Duration) *Cache[T] {
	c := &Cache[T]{
		defaultTTL: defaultTTL,
		items:      make(map[string]item[T]),
		stopChan:   make(chan struct{}),
	}

	if cleanupInterval > 0 {
		go c.startJanitor(cleanupInterval)
		runtime.SetFinalizer(c, func(cache *Cache[T]) {
			cache.Stop()
		})
	}

	return c
}

func (c *Cache[T]) startJanitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.deleteExpired()
		}
	}
}

func (c *Cache[T]) deleteExpired() {
	now := time.Now().UnixNano()
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, it := range c.items {
		if it.expiresAt > 0 && now > it.expiresAt {
			delete(c.items, k)
		}
	}
}

// Set adds or overwrites an item with a given TTL.
// If ttl == DefaultExpiration (0) and defaultTTL > 0, defaultTTL is used.
// If ttl == NoExpiration (-1), the item will never expire.
func (c *Cache[T]) Set(key string, val T, ttl time.Duration) {
	var expiresAt int64
	if ttl == DefaultExpiration {
		ttl = c.defaultTTL
	}

	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	c.items[key] = item[T]{
		val:       val,
		expiresAt: expiresAt,
	}
	c.mu.Unlock()
}

// SetDefault adds or overwrites an item using the cache's default TTL.
func (c *Cache[T]) SetDefault(key string, val T) {
	c.Set(key, val, DefaultExpiration)
}

// Get retrieves an item by key. Returns the value and true if found and not expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	it, found := c.items[key]
	c.mu.RUnlock()

	var zero T
	if !found {
		return zero, false
	}

	if it.expiresAt > 0 && time.Now().UnixNano() > it.expiresAt {
		return zero, false
	}

	return it.val, true
}

// Delete removes an item from the cache.
func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Flush removes all items from the cache.
func (c *Cache[T]) Flush() {
	c.mu.Lock()
	c.items = make(map[string]item[T])
	c.mu.Unlock()
}

// ItemCount returns the number of unexpired items in the cache.
func (c *Cache[T]) ItemCount() int {
	now := time.Now().UnixNano()
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := 0
	for _, it := range c.items {
		if it.expiresAt == 0 || now <= it.expiresAt {
			count++
		}
	}
	return count
}

// Stop terminates the background cleanup goroutine.
func (c *Cache[T]) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
}
