package http

import (
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type RoutingHandler struct {
	configSvc *service.ConfigService
	auditSvc  *service.AuditLogService
}

func NewRoutingHandler(configSvc *service.ConfigService, auditSvc ...*service.AuditLogService) *RoutingHandler {
	h := &RoutingHandler{configSvc: configSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *RoutingHandler) Get(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	cfg, err := h.configSvc.GetRoutingConfigByNode(c.Request.Context(), nodeID)
	if err != nil {
		writePerNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *RoutingHandler) Save(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	var cfg domain.RoutingConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.SaveRoutingConfigByNode(c.Request.Context(), nodeID, &cfg); err != nil {
		writePerNodeConfigError(c, err)
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionRoutingSave, "routing", "保存分流路由规则", "SUCCESS")

	c.JSON(http.StatusOK, cfg)
}
