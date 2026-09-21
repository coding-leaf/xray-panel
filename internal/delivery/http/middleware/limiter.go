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

const defaultShardCount = 16

type ipLimiterShard struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiterEntry
	calls    uint64
}

type shardedIPRateLimiter struct {
	shards [defaultShardCount]*ipLimiterShard
	r      rate.Limit
	b      int
}

func newShardedIPRateLimiter(r rate.Limit, b int) *shardedIPRateLimiter {
	limiter := &shardedIPRateLimiter{
		r: r,
		b: b,
	}
	for i := 0; i < defaultShardCount; i++ {
		limiter.shards[i] = &ipLimiterShard{
			limiters: make(map[string]*ipLimiterEntry),
		}
	}
	return limiter
}

// HashIPToShard 基于 FNV-1a 确定性哈希将 IP 分散到分段锁桶
func HashIPToShard(ip string, shardCount int) int {
	if shardCount <= 0 {
		return 0
	}
	var hash uint32 = 2166136261
	for i := 0; i < len(ip); i++ {
		hash ^= uint32(ip[i])
		hash *= 16777619
	}
	return int(hash % uint32(shardCount))
}

func (l *shardedIPRateLimiter) getLimiter(ip string) *rate.Limiter {
	shardIdx := HashIPToShard(ip, defaultShardCount)
	shard := l.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	shard.calls++
	if shard.calls%256 == 0 && len(shard.limiters) > 50 {
		for k, entry := range shard.limiters {
			if now.Sub(entry.lastSeen) > 10*time.Minute {
				delete(shard.limiters, k)
			}
		}
	}

	entry, exists := shard.limiters[ip]
	if !exists {
		lim := rate.NewLimiter(l.r, l.b)
		shard.limiters[ip] = &ipLimiterEntry{limiter: lim, lastSeen: now}
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

// GetRealClientIP 通用获取客户端真实 IP 函数：
// 优先检查 CF-Connecting-IP，其次 X-Real-IP，其次 X-Forwarded-For（取首个有效 IP），最后回退到 c.ClientIP()。
// 保证去除首尾空格并验证非空。
func GetRealClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if cfIP := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); cfIP != "" {
		return cfIP
	}
	if realIP := strings.TrimSpace(c.GetHeader("X-Real-IP")); realIP != "" {
		return realIP
	}
	if xff := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			ip := strings.TrimSpace(part)
			if ip != "" {
				return ip
			}
		}
	}
	return strings.TrimSpace(c.ClientIP())
}

func NewRateLimiter(rateFormatted string) gin.HandlerFunc {
	r, b := parseRate(rateFormatted, 10)
	limiter := newShardedIPRateLimiter(r, b)

	return func(c *gin.Context) {
		ip := GetRealClientIP(c)
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
