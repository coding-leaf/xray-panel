package middleware

import (
	"fmt"
	"net"
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

const (
	defaultShardCount = 16
	maxShardEntries   = 4096
	limiterEntryTTL   = 10 * time.Minute
)

var (
	trustedProxiesMu sync.RWMutex
	trustedProxyNets = defaultTrustedProxyNets()
)

func defaultTrustedProxyNets() []*net.IPNet {
	nets, err := ParseTrustedProxyCIDRs([]string{"127.0.0.1/8", "::1/128"})
	if err != nil {
		return nil
	}
	return nets
}

// ParseTrustedProxyCIDRs parses a list of CIDR strings or bare IP addresses into []*net.IPNet.
func ParseTrustedProxyCIDRs(cidrs []string) ([]*net.IPNet, error) {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, raw := range cidrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if strings.Contains(s, "/") {
			_, ipNet, err := net.ParseCIDR(s)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", raw, err)
			}
			nets = append(nets, ipNet)
			continue
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy IP %q", raw)
		}
		maskBits := 128
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
			maskBits = 32
		}
		nets = append(nets, &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(maskBits, maskBits),
		})
	}
	return nets, nil
}

// SetTrustedProxyCIDRs configures optional extra trusted proxy CIDRs alongside loopback (127.0.0.1/8, ::1/128).
// Passing nil or an empty slice resets trusted proxies to loopback defaults.
func SetTrustedProxyCIDRs(extraCIDRs []string) error {
	extraNets, err := ParseTrustedProxyCIDRs(extraCIDRs)
	if err != nil {
		return err
	}
	combined := defaultTrustedProxyNets()
	combined = append(combined, extraNets...)

	trustedProxiesMu.Lock()
	trustedProxyNets = combined
	trustedProxiesMu.Unlock()
	return nil
}

func isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	trustedProxiesMu.RLock()
	nets := trustedProxyNets
	trustedProxiesMu.RUnlock()

	for _, ipNet := range nets {
		if ipNet != nil && ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func extractRemoteIP(remoteAddr string) (net.IP, string) {
	raw := strings.TrimSpace(remoteAddr)
	if raw == "" {
		return nil, ""
	}
	host := raw
	if h, _, err := net.SplitHostPort(raw); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if ip := net.ParseIP(host); ip != nil {
		return ip, ip.String()
	}
	return nil, host
}

func parseValidIP(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return ""
}

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

func evictExpiredLocked(shard *ipLimiterShard, now time.Time) {
	for k, entry := range shard.limiters {
		if now.Sub(entry.lastSeen) > limiterEntryTTL {
			delete(shard.limiters, k)
		}
	}
}

func evictOldestLocked(shard *ipLimiterShard) {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, entry := range shard.limiters {
		if first || entry.lastSeen.Before(oldestTime) {
			oldestKey = k
			oldestTime = entry.lastSeen
			first = false
		}
	}
	if !first {
		delete(shard.limiters, oldestKey)
	}
}

func (l *shardedIPRateLimiter) getLimiter(ip string) *rate.Limiter {
	shardIdx := HashIPToShard(ip, defaultShardCount)
	shard := l.shards[shardIdx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	shard.calls++
	if shard.calls%256 == 0 && len(shard.limiters) > 50 {
		evictExpiredLocked(shard, now)
	}

	if entry, exists := shard.limiters[ip]; exists {
		entry.lastSeen = now
		return entry.limiter
	}

	if len(shard.limiters) >= maxShardEntries {
		evictExpiredLocked(shard, now)
		for len(shard.limiters) >= maxShardEntries {
			evictOldestLocked(shard)
		}
	}

	lim := rate.NewLimiter(l.r, l.b)
	shard.limiters[ip] = &ipLimiterEntry{limiter: lim, lastSeen: now}
	return lim
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
// 仅当直连 RemoteAddr 属于受信代理（默认 127.0.0.1/8、::1/128 及空 RemoteAddr，支持 SetTrustedProxyCIDRs 扩展）时，
// 才按 CF-Connecting-IP -> X-Real-IP -> X-Forwarded-For 顺序解析有效 IP；
// 非受信直连对端强制忽略所有转发请求头并直接返回直连 socket IP，防止伪造头绕过限流。
func GetRealClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}

	remoteAddr := strings.TrimSpace(c.Request.RemoteAddr)
	remoteIP, socketIP := extractRemoteIP(remoteAddr)
	if remoteAddr != "" && !isTrustedProxyIP(remoteIP) {
		if socketIP != "" {
			return socketIP
		}
		return strings.TrimSpace(c.ClientIP())
	}

	if cfIP := parseValidIP(c.GetHeader("CF-Connecting-IP")); cfIP != "" {
		return cfIP
	}
	if realIP := parseValidIP(c.GetHeader("X-Real-IP")); realIP != "" {
		return realIP
	}
	if xff := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			if ip := parseValidIP(part); ip != "" {
				return ip
			}
		}
	}

	if socketIP != "" {
		return socketIP
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
