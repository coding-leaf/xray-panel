package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

func parseOptionalNodeID(c *gin.Context) (uint, bool) {
	nodeIDStr := strings.TrimSpace(c.Query("nodeId"))
	if nodeIDStr == "" || nodeIDStr == "0" {
		return 0, true
	}
	id, err := strconv.ParseUint(nodeIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid nodeId"})
		return 0, false
	}
	return uint(id), true
}

func writePerNodeConfigError(c *gin.Context, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidConfig) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

type ConfigHandler struct {
	configSvc *service.ConfigService
	auditSvc  *service.AuditLogService
}

func NewConfigHandler(configSvc *service.ConfigService, auditSvc ...*service.AuditLogService) *ConfigHandler {
	h := &ConfigHandler{configSvc: configSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func (h *ConfigHandler) GetRaw(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	if nodeID > 0 {
		raw, err := h.configSvc.GetNodeCompiledConfigRaw(c.Request.Context(), nodeID)
		if err != nil {
			writePerNodeConfigError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"raw": string(raw), "readOnly": true})
		return
	}

	raw, err := h.configSvc.GetRawConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}

func (h *ConfigHandler) ValidateRaw(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read request body failed"})
		return
	}

	if err := h.configSvc.ValidateRawConfig(c.Request.Context(), body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "valid": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"valid": true, "message": "xray configuration is valid"})
}

func (h *ConfigHandler) SaveAndApply(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read request body failed"})
		return
	}

	if err := h.configSvc.SaveAndApplyRawConfig(c.Request.Context(), body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionConfigApply, "config.json", "在线保存并重新应用 Xray 核心配置", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "configuration saved and xray reloaded successfully"})
}

func (h *ConfigHandler) RestartService(c *gin.Context) {
	if err := h.configSvc.RestartService(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionCoreRestart, "xray", "平滑重启 Xray 核心进程", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "Xray 核心已成功全量重启"})
}

func (h *ConfigHandler) ListSnapshots(c *gin.Context) {
	list, err := h.configSvc.ListSnapshots(c.Request.Context(), 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *ConfigHandler) RollbackSnapshot(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot id"})
		return
	}

	if err := h.configSvc.RollbackSnapshot(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionConfigRollback, idStr, fmt.Sprintf("回滚配置快照 ID: %d", id), "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "配置已成功回滚至历史快照并重新应用"})
}
