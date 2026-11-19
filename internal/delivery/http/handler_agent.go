package http

import (
	"errors"
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	nodeSvc *service.NodeService
}

func NewAgentHandler(nodeSvc *service.NodeService) *AgentHandler {
	return &AgentHandler{nodeSvc: nodeSvc}
}

func (h *AgentHandler) Sync(c *gin.Context) {
	nodeID, ok := GetAgentNodeID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized agent"})
		return
	}

	var req domain.AgentSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.nodeSvc.HandleAgentSync(c.Request.Context(), nodeID, &req)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "node is disabled"})
			return
		}
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent node"})
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
