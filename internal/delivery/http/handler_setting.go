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
	delete(settings, "jwt_secret")
	c.JSON(http.StatusOK, settings)
}

func (h *SettingHandler) SaveSettings(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for k, v := range body {
		if v == nil || k == "jwt_secret" {
			continue
		}
		strVal := fmt.Sprintf("%v", v)
		_ = h.settingRepo.Set(c.Request.Context(), k, strVal)
	}

	var configPath, binPath, serviceName string
	if v, ok := body["xray_config_path"].(string); ok && v != "" {
		configPath = v
	}
	if v, ok := body["xray_bin_path"].(string); ok && v != "" {
		binPath = v
	}
	if v, ok := body["xray_service_name"].(string); ok && v != "" {
		serviceName = v
	}

	// 动态更新配置管理器与 supervisor
	if h.configMgr != nil && configPath != "" {
		h.configMgr.UpdateConfig(configPath, binPath)
	}
	if h.supervisor != nil && serviceName != "" {
		h.supervisor.UpdateConfig(serviceName, binPath)
	}

	// 动态更新 Telegram Bot
	if h.botAdapter != nil {
		var chatID int64
		if chatIDVal, ok := body["tg_admin_chat_id"]; ok && chatIDVal != nil {
			chatIDStr := fmt.Sprintf("%v", chatIDVal)
			if chatIDStr != "" && chatIDStr != "<nil>" {
				_, _ = fmt.Sscanf(chatIDStr, "%d", &chatID)
			}
		}
		tgToken := ""
		if tokVal, ok := body["tg_bot_token"]; ok && tokVal != nil {
			tgToken = fmt.Sprintf("%v", tokVal)
			if tgToken == "<nil>" {
				tgToken = ""
			}
		}
		_ = h.botAdapter.UpdateConfig(tgToken, chatID)
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
