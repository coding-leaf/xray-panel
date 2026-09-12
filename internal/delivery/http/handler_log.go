package http

import (
	"net/http"
	"strconv"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	logSvc   *service.LogService
	auditSvc *service.AuditLogService
}

func NewLogHandler(logSvc *service.LogService, auditSvc ...*service.AuditLogService) *LogHandler {
	h := &LogHandler{logSvc: logSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *LogHandler) GetLogs(c *gin.Context) {
	logType := c.DefaultQuery("type", "access")
	linesStr := c.DefaultQuery("lines", "100")
	lines, _ := strconv.Atoi(linesStr)

	inbound := c.Query("inbound")
	keyword := c.Query("keyword")

	filter := xray.LogFilter{
		InboundTag: inbound,
		Keyword:    keyword,
	}

	resp, err := h.logSvc.GetRecentLogs(c.Request.Context(), logType, lines, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ClearLogs 高危操作：清空日志文件并记录操作审查日志
func (h *LogHandler) ClearLogs(c *gin.Context) {
	logType := c.DefaultQuery("type", "access")

	if err := h.logSvc.ClearLogs(c.Request.Context(), logType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionLogClear, logType, "清空日志文件: "+logType, "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"success": true})
}
