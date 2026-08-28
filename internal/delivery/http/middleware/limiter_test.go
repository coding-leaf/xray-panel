package middleware

import (
	"net/http"
	"net/http/httptest"
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
}
