package http

import (
	"fmt"
	"net/http"

	"panel/internal/adapter/telegram"
	"panel/internal/adapter/xray"
	"panel/internal/domain"

	"github.com/gin-gonic/gin"
)

type SettingHandler struct {
	settingRepo domain.SettingRepository
	botAdapter  *telegram.BotAdapter
	configMgr   *xray.ConfigManager
	supervisor  *xray.SystemdSupervisor
}

func NewSettingHandler(
	settingRepo domain.SettingRepository,
	botAdapter *telegram.BotAdapter,
	configMgr *xray.ConfigManager,
	supervisor *xray.SystemdSupervisor,
) *SettingHandler {
	return &SettingHandler{
		settingRepo: settingRepo,
		botAdapter:  botAdapter,
		configMgr:   configMgr,
		supervisor:  supervisor,
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

	// 动态更新配置管理器与 supervisor
	if h.configMgr != nil {
		h.configMgr.UpdateConfig(body["xray_config_path"], body["xray_bin_path"])
	}
	if h.supervisor != nil {
		h.supervisor.UpdateConfig(body["xray_service_name"], body["xray_bin_path"])
	}

	// 动态更新 Telegram Bot
	if h.botAdapter != nil {
		var chatID int64
		if chatIDStr, ok := body["tg_admin_chat_id"]; ok && chatIDStr != "" {
			_, _ = fmt.Sscanf(chatIDStr, "%d", &chatID)
		}
		_ = h.botAdapter.UpdateConfig(body["tg_bot_token"], chatID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated successfully"})
}

func (h *SettingHandler) TestTelegram(c *gin.Context) {
	if h.botAdapter == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram Bot 未初始化"})
		return
	}
	err := h.botAdapter.SendMessage(c.Request.Context(), "🔔 <b>测试通知</b>\n恭喜！Xray 解耦面板与 Telegram 告警机器人连接成功！")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("发送失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "测试消息发送成功"})
}
