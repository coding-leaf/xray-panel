package service

import (
	"context"
	"fmt"

	"panel/internal/domain"
)

type SettingReloader interface {
	UpdateConfig(configPath, binPath string)
}

type SupervisorReloader interface {
	UpdateConfig(serviceName, binPath string)
}

type BotReloader interface {
	UpdateConfig(token string, adminChatID int64) error
	SendMessage(ctx context.Context, text string) error
}

type SettingService struct {
	settingRepo domain.SettingRepository
	configMgr   SettingReloader
	supervisor  SupervisorReloader
	botAdapter  BotReloader
	auditSvc    *AuditLogService
}

func NewSettingService(
	settingRepo domain.SettingRepository,
	configMgr SettingReloader,
	supervisor SupervisorReloader,
	botAdapter BotReloader,
	auditSvc ...*AuditLogService,
) *SettingService {
	s := &SettingService{
		settingRepo: settingRepo,
		configMgr:   configMgr,
		supervisor:  supervisor,
		botAdapter:  botAdapter,
	}
	if len(auditSvc) > 0 {
		s.auditSvc = auditSvc[0]
	}
	return s
}

func (s *SettingService) GetAllSettings(ctx context.Context) (map[string]string, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	delete(settings, "jwt_secret")
	return settings, nil
}

func (s *SettingService) UpdateSettings(ctx context.Context, body map[string]interface{}, clientIP string) error {
	for k, v := range body {
		if v == nil || k == "jwt_secret" {
			continue
		}
		strVal := fmt.Sprintf("%v", v)
		_ = s.settingRepo.Set(ctx, k, strVal)
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
	if s.configMgr != nil && configPath != "" {
		s.configMgr.UpdateConfig(configPath, binPath)
	}
	if s.supervisor != nil && serviceName != "" {
		s.supervisor.UpdateConfig(serviceName, binPath)
	}

	// 动态更新 Telegram Bot
	if s.botAdapter != nil {
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
		_ = s.botAdapter.UpdateConfig(tgToken, chatID)
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, "admin", clientIP, domain.ActionSettingUpdate, "settings", "更新系统全局配置", "SUCCESS")
	}

	return nil
}

func (s *SettingService) TestTelegram(ctx context.Context) error {
	if s.botAdapter == nil {
		return fmt.Errorf("Telegram Bot 未初始化")
	}
	return s.botAdapter.SendMessage(ctx, "🔔 <b>测试通知</b>\n恭喜！Xray 解耦面板与 Telegram 告警机器人连接成功！")
}
