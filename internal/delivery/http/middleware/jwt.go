package middleware

import (
	"net/http"
	"strings"
	"time"

	paneljwt "panel/internal/pkg/jwt"

	"github.com/gin-gonic/gin"
)

type JWTClaims = paneljwt.Claims

func GenerateToken(username, secret string, expireDuration time.Duration) (string, error) {
	return paneljwt.GenerateToken(username, secret, expireDuration)
}

func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token format"})
			return
		}

		tokenString := parts[1]
		claims, err := paneljwt.ParseToken(tokenString, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token is invalid or expired"})
			return
		}

		c.Set("username", claims.Username)
		c.Next()
	}
}
