package http

import (
	"net/http"

	"panel/internal/adapter/telegram"
	"panel/internal/domain"

	"github.com/gin-gonic/gin"
)

type SettingHandler struct {
	settingRepo domain.SettingRepository
	botAdapter  *telegram.BotAdapter
}

func NewSettingHandler(settingRepo domain.SettingRepository, botAdapter *telegram.BotAdapter) *SettingHandler {
	return &SettingHandler{
		settingRepo: settingRepo,
		botAdapter:  botAdapter,
	}
}

func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingRepo.GetAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) SaveSettings(c *gin.Context) {
	var body map[string]string
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for k, v := range body {
		_ = h.settingRepo.Set(c.Request.Context(), k, v)
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated successfully"})
}
