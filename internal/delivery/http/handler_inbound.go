package http

import (
	"fmt"
	"net/http"
	"strconv"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type InboundHandler struct {
	configSvc         *service.ConfigService
	auditSvc          *service.AuditLogService
	realityMonitorSvc *service.RealityMonitorService
}

func NewInboundHandler(configSvc *service.ConfigService, auditSvc *service.AuditLogService, realityMonitorSvc ...*service.RealityMonitorService) *InboundHandler {
	h := &InboundHandler{
		configSvc: configSvc,
		auditSvc:  auditSvc,
	}
	if len(realityMonitorSvc) > 0 {
		h.realityMonitorSvc = realityMonitorSvc[0]
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
	pair, err := h.configSvc.GenerateRealityKeyPair()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pair)
}

// GetRealityStatus retrieves the latest aggregated Reality disguise domain status.
func (h *InboundHandler) GetRealityStatus(c *gin.Context) {
	if h.realityMonitorSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "success",
			"data": domain.RealitySummaryStatus{
				Items: []domain.RealityCheckItem{},
			},
		})
		return
	}

	summary := h.realityMonitorSvc.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": summary,
	})
}

// TriggerRealityCheck immediately triggers an active probe check for all Reality inbounds.
func (h *InboundHandler) TriggerRealityCheck(c *gin.Context) {
	if h.realityMonitorSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"msg":  "reality check completed",
			"data": domain.RealitySummaryStatus{
				Items: []domain.RealityCheckItem{},
			},
		})
		return
	}

	summary, err := h.realityMonitorSvc.CheckAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":  -1,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "reality check completed",
		"data": summary,
	})
}
