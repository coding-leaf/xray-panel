package http

import (
	"errors"
	"net/http"
	"strings"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

const contextKeyAgentNodeID = "agentNodeID"

// AgentAuth authenticates remote node agents via Bearer SHA-256 token lookup and injects nodeID into context.
func AgentAuth(nodeSvc *service.NodeService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if nodeSvc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "agent service unavailable"})
			return
		}
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token format"})
			return
		}

		node, err := nodeSvc.AuthenticateAgent(c.Request.Context(), strings.TrimSpace(parts[1]))
		if err != nil {
			if errors.Is(err, domain.ErrUnauthorized) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
				return
			}
			if errors.Is(err, domain.ErrForbidden) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "node is disabled"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "agent authentication failed"})
			return
		}

		c.Set(contextKeyAgentNodeID, node.ID)
		c.Next()
	}
}

// GetAgentNodeID extracts the authenticated remote node ID from the Gin request context.
func GetAgentNodeID(c *gin.Context) (uint, bool) {
	if c == nil {
		return 0, false
	}
	v, ok := c.Get(contextKeyAgentNodeID)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok && id != 0
}
