package http

import (
	"net/http"
	"strconv"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct {
	auditSvc *service.AuditLogService
}

func NewAuditLogHandler(auditSvc *service.AuditLogService) *AuditLogHandler {
	return &AuditLogHandler{auditSvc: auditSvc}
}

func (h *AuditLogHandler) GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	action := c.Query("action")
	operator := c.Query("operator")
	keyword := c.Query("keyword")

	items, total, err := h.auditSvc.List(c.Request.Context(), page, pageSize, action, operator, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (h *AuditLogHandler) ClearAuditLogs(c *gin.Context) {
	if err := h.auditSvc.Clear(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 记录清空审计日志本身的操作
	h.auditSvc.RecordFromGin(c, domain.ActionAuditClear, "all", "管理员清空历史操作审查日志", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"success": true})
}
