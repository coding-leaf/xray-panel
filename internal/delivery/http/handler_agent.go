package http

import (
	"errors"
	"net/http"
	"strings"

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

func (h *AgentHandler) ReportCommandResult(c *gin.Context) {
	nodeID, ok := GetAgentNodeID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized agent"})
		return
	}

	cmdID := strings.TrimSpace(c.Param("id"))
	if cmdID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command id"})
		return
	}

	var res domain.AgentCommandResult
	if err := c.ShouldBindJSON(&res); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bodyID := strings.TrimSpace(res.ID)
	if bodyID == "" {
		res.ID = cmdID
	} else if bodyID != cmdID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command id mismatch"})
		return
	}

	if err := h.nodeSvc.ReportAgentCommandResult(c.Request.Context(), nodeID, &res); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, domain.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
