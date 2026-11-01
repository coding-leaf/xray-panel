package http

import (
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	monitorSvc   *service.MonitorService
	panelVersion string
	commit       string
	buildTime    string
}

func NewDashboardHandler(monitorSvc *service.MonitorService, panelVersion string, meta ...string) *DashboardHandler {
	h := &DashboardHandler{
		monitorSvc:   monitorSvc,
		panelVersion: panelVersion,
	}
	if len(meta) > 0 {
		h.commit = meta[0]
	}
	if len(meta) > 1 {
		h.buildTime = meta[1]
	}
	return h
}

func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	data, err := h.monitorSvc.GetDashboardData(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *DashboardHandler) GetServiceStatus(c *gin.Context) {
	var active bool
	var subState, ver string
	if h.monitorSvc != nil {
		status, v, _ := h.monitorSvc.GetServiceStatus(c.Request.Context())
		active = status.Active
		subState = status.SubState
		ver = v
	}
	c.JSON(http.StatusOK, gin.H{
		"active":       active,
		"subState":     subState,
		"version":      ver,
		"panelVersion": h.panelVersion,
		"commit":       h.commit,
		"buildTime":    h.buildTime,
	})
}
