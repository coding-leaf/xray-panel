package http

import (
	"io"
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type ConfigHandler struct {
	configSvc *service.ConfigService
}

func NewConfigHandler(configSvc *service.ConfigService) *ConfigHandler {
	return &ConfigHandler{configSvc: configSvc}
}

func (h *ConfigHandler) GetRaw(c *gin.Context) {
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

	c.JSON(http.StatusOK, gin.H{"message": "configuration saved and xray reloaded successfully"})
}

func (h *ConfigHandler) RestartService(c *gin.Context) {
	if err := h.configSvc.RestartService(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Xray 核心已成功全量重启"})
}
