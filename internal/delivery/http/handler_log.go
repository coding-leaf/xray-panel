package http

import (
	"net/http"
	"strconv"
	"strings"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	logSvc   *service.LogService
	auditSvc *service.AuditLogService
	nodeSvc  *service.NodeService
}

func NewLogHandler(logSvc *service.LogService, auditSvc ...*service.AuditLogService) *LogHandler {
	h := &LogHandler{logSvc: logSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

// WithNodeService attaches NodeService for on-demand remote node log retrieval.
func (h *LogHandler) WithNodeService(nodeSvc *service.NodeService) *LogHandler {
	h.nodeSvc = nodeSvc
	return h
}

func (h *LogHandler) GetLogs(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	if nodeID > 0 {
		if h.nodeSvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "node service not configured"})
			return
		}
		svcName := c.DefaultQuery("service", "xray")
		linesStr := c.DefaultQuery("lines", "100")
		lines, err := strconv.Atoi(linesStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid lines"})
			return
		}
		refresh := strings.EqualFold(c.Query("refresh"), "true") || c.Query("refresh") == "1"
		resp, err := h.nodeSvc.RequestNodeLogs(c.Request.Context(), nodeID, svcName, lines, refresh)
		if err != nil {
			writePerNodeConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	logType := c.DefaultQuery("type", "access")
	linesStr := c.DefaultQuery("lines", "100")
	lines, _ := strconv.Atoi(linesStr)

	inbound := c.Query("inbound")
	outbound := c.Query("outbound")
	level := c.Query("level")
	keyword := c.Query("keyword")

	filter := domain.LogFilter{
		InboundTag:  inbound,
		OutboundTag: outbound,
		Level:       level,
		Keyword:     keyword,
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
