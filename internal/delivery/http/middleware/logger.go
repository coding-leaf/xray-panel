package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"panel/internal/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}

		ctx := context.WithValue(c.Request.Context(), logger.RequestIDKey, reqID)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", reqID)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		slog.Info("HTTP Request",
			slog.String("request_id", reqID),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.String("client_ip", c.ClientIP()),
			slog.Duration("latency", latency),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("HTTP Panic Recovered",
					slog.Any("error", err),
					slog.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "internal server error",
				})
			}
		}()
		c.Next()
	}
}
