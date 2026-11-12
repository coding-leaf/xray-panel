package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	limiterMiddleware := NewRateLimiter("3-M") // 3 requests per minute
	r.GET("/test", limiterMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// First 3 requests should succeed
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d", i, w.Code)
		}
	}

	// 4th request should be rate limited (429)
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4 expected status 429, got %d", w.Code)
	}

	// Another IP should still succeed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "192.0.2.2:1234"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("different IP expected status 200, got %d", w2.Code)
	}
}

func TestGlobalRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	limiterMiddleware := NewGlobalRateLimiter("2-M", "test_global")
	r.GET("/global", limiterMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// First 2 requests should succeed even from different IPs
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest("GET", "/global", nil)
		req.RemoteAddr = "192.0.2." + string(rune('0'+i)) + ":1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d expected status 200, got %d", i, w.Code)
		}
	}

	// 3rd request should be blocked globally
	req := httptest.NewRequest("GET", "/global", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3 expected status 429, got %d", w.Code)
	}
}

func TestHashIPToShard(t *testing.T) {
	shardCount := 16
	idx1 := HashIPToShard("192.168.1.1", shardCount)
	idx2 := HashIPToShard("192.168.1.1", shardCount)
	if idx1 != idx2 {
		t.Fatalf("expected hash to be deterministic, got %d and %d", idx1, idx2)
	}
	if idx1 < 0 || idx1 >= shardCount {
		t.Fatalf("hash index out of bounds: %d", idx1)
	}

	// 边界检查: 0 或负数 shardCount
	if idx := HashIPToShard("1.1.1.1", 0); idx != 0 {
		t.Fatalf("expected 0 for shardCount=0, got %d", idx)
	}
}

func TestShardedIPRateLimiter_Concurrent(t *testing.T) {
	limiter := newShardedIPRateLimiter(100, 100)
	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(gid int) {
			defer wg.Done()
			for it := 0; it < iterations; it++ {
				ip := fmt.Sprintf("192.168.%d.%d", gid%16, it%255)
				lim := limiter.getLimiter(ip)
				if lim == nil {
					t.Errorf("expected non-nil limiter for ip %s", ip)
					return
				}
				_ = lim.Allow()
			}
		}(i)
	}
	wg.Wait()
}

func TestGetRealClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("nil context", func(t *testing.T) {
		ip := GetRealClientIP(nil)
		if ip != "" {
			t.Fatalf("expected empty string for nil context, got %q", ip)
		}
	})

	t.Run("CF-Connecting-IP priority", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", " 203.0.113.10 ")
		req.Header.Set("X-Real-IP", "198.51.100.20")
		req.Header.Set("X-Forwarded-For", "192.0.2.30, 10.0.0.1")
		req.RemoteAddr = "127.0.0.1:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "203.0.113.10" {
			t.Fatalf("expected 203.0.113.10, got %q", ip)
		}
	})

	t.Run("X-Real-IP priority when CF-Connecting-IP missing or whitespace", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "   ")
		req.Header.Set("X-Real-IP", " 198.51.100.20 ")
		req.Header.Set("X-Forwarded-For", "192.0.2.30, 10.0.0.1")
		req.RemoteAddr = "127.0.0.1:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "198.51.100.20" {
			t.Fatalf("expected 198.51.100.20, got %q", ip)
		}
	})

	t.Run("X-Forwarded-For first IP when CF and X-Real missing", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "  192.0.2.30  , 10.0.0.1, 10.0.0.2")
		req.RemoteAddr = "127.0.0.1:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "192.0.2.30" {
			t.Fatalf("expected 192.0.2.30, got %q", ip)
		}
	})

	t.Run("fallback to ClientIP when headers empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "")
		req.Header.Set("X-Real-IP", "  ")
		req.Header.Set("X-Forwarded-For", " , ")
		req.RemoteAddr = "192.0.2.99:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "192.0.2.99" {
			t.Fatalf("expected 192.0.2.99, got %q", ip)
		}
	})

	t.Run("untrusted public RemoteAddr ignores forged forwarding headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "198.51.100.1")
		req.Header.Set("X-Real-IP", "198.51.100.2")
		req.Header.Set("X-Forwarded-For", "198.51.100.3, 10.0.0.1")
		req.RemoteAddr = "203.0.113.50:44321"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "203.0.113.50" {
			t.Fatalf("expected direct socket IP 203.0.113.50, got %q", ip)
		}
	})

	t.Run("untrusted IPv6 RemoteAddr ignores forged forwarding headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "198.51.100.1")
		req.RemoteAddr = "[2001:db8::50]:44321"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "2001:db8::50" {
			t.Fatalf("expected direct socket IP 2001:db8::50, got %q", ip)
		}
	})

	t.Run("trusted IPv6 loopback extracts forwarding headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Real-IP", "198.51.100.20")
		req.RemoteAddr = "[::1]:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "198.51.100.20" {
			t.Fatalf("expected 198.51.100.20, got %q", ip)
		}
	})

	t.Run("empty RemoteAddr trusts forwarding headers", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "203.0.113.10")
		req.RemoteAddr = ""
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "203.0.113.10" {
			t.Fatalf("expected 203.0.113.10, got %q", ip)
		}
	})

	t.Run("invalid forwarding header IP is skipped", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "not-an-ip")
		req.Header.Set("X-Real-IP", "bad-ip")
		req.Header.Set("X-Forwarded-For", "also-bad, 192.0.2.44")
		req.RemoteAddr = "127.0.0.1:12345"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "192.0.2.44" {
			t.Fatalf("expected 192.0.2.44, got %q", ip)
		}
	})

	t.Run("custom trusted proxy CIDR", func(t *testing.T) {
		if err := SetTrustedProxyCIDRs([]string{"10.20.0.0/16"}); err != nil {
			t.Fatalf("SetTrustedProxyCIDRs failed: %v", err)
		}
		defer func() {
			if err := SetTrustedProxyCIDRs(nil); err != nil {
				t.Fatalf("reset SetTrustedProxyCIDRs failed: %v", err)
			}
		}()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("CF-Connecting-IP", "203.0.113.88")
		req.RemoteAddr = "10.20.1.5:1234"
		c.Request = req

		ip := GetRealClientIP(c)
		if ip != "203.0.113.88" {
			t.Fatalf("expected 203.0.113.88, got %q", ip)
		}

		if err := SetTrustedProxyCIDRs([]string{"invalid-cidr"}); err == nil {
			t.Fatalf("expected error for invalid CIDR")
		}
	})
}

func TestRateLimiter_UntrustedSpoofedHeadersCannotBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	limiterMiddleware := NewRateLimiter("2-M")
	r.GET("/login", limiterMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest("GET", "/login", nil)
		req.RemoteAddr = "203.0.113.77:10001"
		req.Header.Set("CF-Connecting-IP", fmt.Sprintf("198.51.100.%d", i))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i, w.Code)
		}
	}

	// 3rd request from same untrusted remote IP with a brand new forged header must still be rate-limited
	req := httptest.NewRequest("GET", "/login", nil)
	req.RemoteAddr = "203.0.113.77:10002"
	req.Header.Set("CF-Connecting-IP", "198.51.100.250")
	req.Header.Set("X-Real-IP", "198.51.100.251")
	req.Header.Set("X-Forwarded-For", "192.0.2.250")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for spoofed headers from untrusted IP, got %d", w.Code)
	}
}

func TestShardedIPRateLimiter_MaxShardEntries(t *testing.T) {
	collectShardZeroIPs := func(count int) []string {
		ips := make([]string, 0, count)
		for a := 1; a <= 254 && len(ips) < count; a++ {
			for b := 0; b <= 255 && len(ips) < count; b++ {
				for c := 1; c <= 254 && len(ips) < count; c++ {
					ip := fmt.Sprintf("10.%d.%d.%d", a, b, c)
					if HashIPToShard(ip, defaultShardCount) == 0 {
						ips = append(ips, ip)
					}
				}
			}
		}
		if len(ips) < count {
			t.Fatalf("failed to generate %d IPs for shard 0, got %d", count, len(ips))
		}
		return ips
	}

	t.Run("evicts oldest when shard reaches maxShardEntries", func(t *testing.T) {
		limiter := newShardedIPRateLimiter(10, 10)
		ips := collectShardZeroIPs(maxShardEntries + 50)

		for i := 0; i < maxShardEntries; i++ {
			_ = limiter.getLimiter(ips[i])
		}

		shard := limiter.shards[0]
		shard.mu.Lock()
		// Make ips[0] deterministically the oldest non-expired entry regardless of OS clock granularity.
		shard.limiters[ips[0]].lastSeen = time.Now().Add(-5 * time.Minute)
		shard.mu.Unlock()

		for i := maxShardEntries; i < len(ips); i++ {
			_ = limiter.getLimiter(ips[i])
		}

		shard.mu.Lock()
		defer shard.mu.Unlock()

		if len(shard.limiters) > maxShardEntries {
			t.Fatalf("expected shard length <= %d, got %d", maxShardEntries, len(shard.limiters))
		}
		if len(shard.limiters) != maxShardEntries {
			t.Fatalf("expected shard length == %d, got %d", maxShardEntries, len(shard.limiters))
		}
		if _, exists := shard.limiters[ips[0]]; exists {
			t.Fatalf("expected oldest IP %s to be evicted", ips[0])
		}
		if _, exists := shard.limiters[ips[len(ips)-1]]; !exists {
			t.Fatalf("expected newest IP %s to remain", ips[len(ips)-1])
		}
	})

	t.Run("purges expired entries before evicting active entries", func(t *testing.T) {
		limiter := newShardedIPRateLimiter(10, 10)
		ips := collectShardZeroIPs(maxShardEntries + 1)
		now := time.Now()

		shard := limiter.shards[0]
		shard.mu.Lock()
		for i := 0; i < maxShardEntries; i++ {
			lastSeen := now.Add(-1 * time.Minute)
			if i == 0 {
				lastSeen = now.Add(-15 * time.Minute) // expired (>10m)
			}
			shard.limiters[ips[i]] = &ipLimiterEntry{
				lastSeen: lastSeen,
			}
		}
		// Refresh entry 1 to make sure it's active and newer than entry 2, while entry 2 is active (>1m ago, <10m)
		shard.limiters[ips[2]].lastSeen = now.Add(-5 * time.Minute)
		shard.mu.Unlock()

		_ = limiter.getLimiter(ips[maxShardEntries])

		shard.mu.Lock()
		defer shard.mu.Unlock()

		if _, exists := shard.limiters[ips[0]]; exists {
			t.Fatalf("expected expired IP %s to be purged", ips[0])
		}
		if _, exists := shard.limiters[ips[2]]; !exists {
			t.Fatalf("expected non-expired IP %s to be retained after expired purge", ips[2])
		}
		if _, exists := shard.limiters[ips[maxShardEntries]]; !exists {
			t.Fatalf("expected newly inserted IP %s to be present", ips[maxShardEntries])
		}
	})
}


