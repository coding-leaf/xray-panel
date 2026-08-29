package http

import (
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	monitorSvc *service.MonitorService
}

func NewDashboardHandler(monitorSvc *service.MonitorService) *DashboardHandler {
	return &DashboardHandler{monitorSvc: monitorSvc}
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	data, err := h.monitorSvc.GetDashboardData(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}
