package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockSettingRepo struct {
	data map[string]string
}

func (m *mockSettingRepo) Get(ctx context.Context, key string) (string, error) {
	if v, ok := m.data[key]; ok {
		return v, nil
	}
	return "", domain.ErrNotFound
}

func (m *mockSettingRepo) Set(ctx context.Context, key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range m.data {
		cp[k] = v
	}
	return cp, nil
}

type mockSettingReloader struct {
	configPath string
	binPath    string
}

func (m *mockSettingReloader) UpdateConfig(configPath, binPath string) {
	m.configPath = configPath
	m.binPath = binPath
}

type mockSupervisorReloader struct {
	serviceName string
	binPath     string
}

func (m *mockSupervisorReloader) UpdateConfig(serviceName, binPath string) {
	m.serviceName = serviceName
	m.binPath = binPath
}

type mockBotReloader struct {
	token   string
	chatID  int64
	sentMsg string
}

func (m *mockBotReloader) UpdateConfig(token string, adminChatID int64) error {
	m.token = token
	m.chatID = adminChatID
	return nil
}

func (m *mockBotReloader) SendMessage(ctx context.Context, text string) error {
	m.sentMsg = text
	return nil
}

func TestSettingService_GetAll_HidesJWTSecret(t *testing.T) {
	repo := &mockSettingRepo{data: map[string]string{
		"jwt_secret": "sensitive-jwt",
		"site_name":  "MyPanel",
	}}
	svc := NewSettingService(repo, nil, nil, nil)

	settings, err := svc.GetAllSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := settings["jwt_secret"]; exists {
		t.Errorf("expected jwt_secret to be stripped from settings")
	}
	if settings["site_name"] != "MyPanel" {
		t.Errorf("expected site_name to be MyPanel, got %s", settings["site_name"])
	}
}

func TestSettingService_UpdateSettings_ReloadsComponents(t *testing.T) {
	repo := &mockSettingRepo{data: make(map[string]string)}
	configMgr := &mockSettingReloader{}
	supervisor := &mockSupervisorReloader{}
	bot := &mockBotReloader{}

	svc := NewSettingService(repo, configMgr, supervisor, bot)

	body := map[string]interface{}{
		"site_name":         "NewPanel",
		"jwt_secret":        "new-secret-should-be-ignored",
		"xray_config_path":  "/etc/xray/config.json",
		"xray_bin_path":     "/usr/local/bin/xray",
		"xray_service_name": "xray.service",
		"tg_bot_token":      "123456:ABC-DEF",
		"tg_admin_chat_id":  "987654321",
	}

	if err := svc.UpdateSettings(context.Background(), body, "127.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.data["site_name"] != "NewPanel" {
		t.Errorf("expected site_name updated, got %s", repo.data["site_name"])
	}
	if _, exists := repo.data["jwt_secret"]; exists {
		t.Errorf("expected jwt_secret not to be updated via general settings")
	}

	if configMgr.configPath != "/etc/xray/config.json" || configMgr.binPath != "/usr/local/bin/xray" {
		t.Errorf("configMgr not reloaded properly: %+v", configMgr)
	}
	if supervisor.serviceName != "xray.service" || supervisor.binPath != "/usr/local/bin/xray" {
		t.Errorf("supervisor not reloaded properly: %+v", supervisor)
	}
	if bot.token != "123456:ABC-DEF" || bot.chatID != 987654321 {
		t.Errorf("bot not reloaded properly: %+v", bot)
	}

	// Test Telegram
	if err := svc.TestTelegram(context.Background()); err != nil {
		t.Fatalf("TestTelegram failed: %v", err)
	}
	if bot.sentMsg == "" {
		t.Errorf("expected test message to be sent")
	}
}
