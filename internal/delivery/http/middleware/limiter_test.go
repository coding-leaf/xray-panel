package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
}


