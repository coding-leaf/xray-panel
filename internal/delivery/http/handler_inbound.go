package http

import (
	"fmt"
	"net/http"
	"strconv"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type InboundHandler struct {
	configSvc *service.ConfigService
	auditSvc  *service.AuditLogService
}

func NewInboundHandler(configSvc *service.ConfigService, auditSvc ...*service.AuditLogService) *InboundHandler {
	h := &InboundHandler{configSvc: configSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *InboundHandler) List(c *gin.Context) {
	inbounds, err := h.configSvc.ListInbounds(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, inbounds)
}

func (h *InboundHandler) Create(c *gin.Context) {
	var in domain.Inbound
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.CreateInbound(c.Request.Context(), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionInboundCreate, in.Tag, fmt.Sprintf("创建入站节点: %s (端口: %d, 协议: %s)", in.Tag, in.Port, in.Protocol), "SUCCESS")

	c.JSON(http.StatusCreated, in)
}

func (h *InboundHandler) Update(c *gin.Context) {
	var in domain.Inbound
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idStr := c.Param("id")
	if idStr != "" {
		if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
			in.ID = uint(id)
		}
	}

	if in.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "inbound id is required"})
		return
	}

	if err := h.configSvc.UpdateInbound(c.Request.Context(), &in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionInboundUpdate, in.Tag, fmt.Sprintf("更新入站节点: %s (端口: %d, 协议: %s)", in.Tag, in.Port, in.Protocol), "SUCCESS")

	c.JSON(http.StatusOK, in)
}

func (h *InboundHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid inbound id"})
		return
	}

	if err := h.configSvc.DeleteInbound(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionInboundDelete, idStr, fmt.Sprintf("删除入站节点 ID: %d", id), "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "inbound deleted"})
}

func (h *InboundHandler) GenerateRealityKey(c *gin.Context) {
	pair, err := xray.GenerateRealityKeyPair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pair)
}
