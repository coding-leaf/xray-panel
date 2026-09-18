package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
	r        rate.Limit
	b        int
	calls    uint64
}

func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*ipLimiterEntry),
		r:        r,
		b:        b,
	}
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.calls++
	if l.calls%256 == 0 && len(l.limiters) > 50 {
		for k, entry := range l.limiters {
			if now.Sub(entry.lastSeen) > 10*time.Minute {
				delete(l.limiters, k)
			}
		}
	}

	entry, exists := l.limiters[ip]
	if !exists {
		lim := rate.NewLimiter(l.r, l.b)
		l.limiters[ip] = &ipLimiterEntry{limiter: lim, lastSeen: now}
		return lim
	}

	entry.lastSeen = now
	return entry.limiter
}

func parseRate(rateFormatted string, defaultLimit int) (rate.Limit, int) {
	parts := strings.Split(rateFormatted, "-")
	if len(parts) != 2 {
		return rate.Limit(float64(defaultLimit) / 60.0), defaultLimit
	}
	limit, err := strconv.Atoi(parts[0])
	if err != nil || limit <= 0 {
		return rate.Limit(float64(defaultLimit) / 60.0), defaultLimit
	}

	var period time.Duration
	switch strings.ToUpper(parts[1]) {
	case "S":
		period = time.Second
	case "M":
		period = time.Minute
	case "H":
		period = time.Hour
	default:
		period = time.Minute
	}

	return rate.Limit(float64(limit) / period.Seconds()), limit
}

func NewRateLimiter(rateFormatted string) gin.HandlerFunc {
	r, b := parseRate(rateFormatted, 10)
	limiter := newIPRateLimiter(r, b)

	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.getLimiter(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，已被限流保护，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

func NewGlobalRateLimiter(rateFormatted string, key string) gin.HandlerFunc {
	r, b := parseRate(rateFormatted, 60)
	limiter := rate.NewLimiter(r, b)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "服务暂时繁忙，全局限流保护中，请稍后再试",
			})
			return
		}
		c.Next()
	}
}
