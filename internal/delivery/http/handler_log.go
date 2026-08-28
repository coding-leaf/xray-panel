package http

import (
	"net/http"
	"strconv"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	logSvc *service.LogService
}

func NewLogHandler(logSvc *service.LogService) *LogHandler {
	return &LogHandler{logSvc: logSvc}
}

func (h *LogHandler) GetLogs(c *gin.Context) {
	logType := c.DefaultQuery("type", "access")
	linesStr := c.DefaultQuery("lines", "100")
	lines, _ := strconv.Atoi(linesStr)

	resp, err := h.logSvc.GetRecentLogs(c.Request.Context(), logType, lines)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
