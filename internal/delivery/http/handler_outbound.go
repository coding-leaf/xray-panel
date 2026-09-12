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
	outbounds, err := h.configSvc.ListOutbounds(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, outbounds)
}

func (h *OutboundHandler) Save(c *gin.Context) {
	var ob domain.Outbound
	if err := c.ShouldBindJSON(&ob); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ob.Tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.SaveOutbound(c.Request.Context(), ob); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionOutboundSave, ob.Tag, "保存出站规则: "+ob.Tag, "SUCCESS")

	c.JSON(http.StatusOK, ob)
}

func (h *OutboundHandler) Delete(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.DeleteOutbound(c.Request.Context(), tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionOutboundSave, tag, "删除出站规则: "+tag, "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "outbound deleted"})
}
