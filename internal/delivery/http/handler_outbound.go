package http

import (
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type OutboundHandler struct {
	configSvc *service.ConfigService
	auditSvc  *service.AuditLogService
}

func NewOutboundHandler(configSvc *service.ConfigService, auditSvc ...*service.AuditLogService) *OutboundHandler {
	h := &OutboundHandler{configSvc: configSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *OutboundHandler) List(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	outbounds, err := h.configSvc.ListOutboundsByNode(c.Request.Context(), nodeID)
	if err != nil {
		writePerNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusOK, outbounds)
}

func (h *OutboundHandler) Save(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	var ob domain.Outbound
	if err := c.ShouldBindJSON(&ob); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ob.Tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.SaveOutboundByNode(c.Request.Context(), nodeID, ob); err != nil {
		writePerNodeConfigError(c, err)
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionOutboundSave, ob.Tag, "保存出站规则: "+ob.Tag, "SUCCESS")

	c.JSON(http.StatusOK, ob)
}

func (h *OutboundHandler) Delete(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.DeleteOutboundByNode(c.Request.Context(), nodeID, tag); err != nil {
		writePerNodeConfigError(c, err)
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionOutboundSave, tag, "删除出站规则: "+tag, "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "outbound deleted"})
}
