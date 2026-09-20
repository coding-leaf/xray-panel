package http

import (
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type SettingHandler struct {
	settingSvc *service.SettingService
}

func NewSettingHandler(settingSvc *service.SettingService) *SettingHandler {
	return &SettingHandler{
		settingSvc: settingSvc,
	}
}

func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingSvc.GetAllSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) SaveSettings(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.settingSvc.UpdateSettings(c.Request.Context(), body, c.ClientIP()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated successfully"})
}

func (h *SettingHandler) TestTelegram(c *gin.Context) {
	if err := h.settingSvc.TestTelegram(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "测试消息发送成功"})
}
