package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	memory "github.com/ulule/limiter/v3/drivers/store/memory"
)

func NewRateLimiter(rateFormatted string) gin.HandlerFunc {
	rate, err := limiter.NewRateFromFormatted(rateFormatted)
	if err != nil {
		rate = limiter.Rate{Period: 60, Limit: 10}
	}
	store := memory.NewStore()
	instance := limiter.New(store, rate)

	return ginlimiter.NewMiddleware(instance, ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "请求过于频繁，已被限流保护，请稍后再试",
		})
	}))
}
