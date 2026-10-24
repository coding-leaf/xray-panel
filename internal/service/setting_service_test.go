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

type mockLogLevelSetter struct {
	level string
}

func (m *mockLogLevelSetter) SetLogLevel(lvl string) {
	m.level = lvl
}

func (m *mockLogLevelSetter) GetLogLevel() string {
	return m.level
}

type mockConfigRecompiler struct {
	recompiled bool
	remark     string
}

func (m *mockConfigRecompiler) RecompileAndApply(ctx context.Context, remark string) error {
	m.recompiled = true
	m.remark = remark
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

func TestSettingService_GetAll_DefaultLogLevel(t *testing.T) {
	// Case 1: No log level in repo, no setter -> defaults to "warning"
	repo := &mockSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, nil, nil, nil)

	settings, err := svc.GetAllSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings["xray_log_level"] != "warning" {
		t.Errorf("expected default xray_log_level 'warning', got %s", settings["xray_log_level"])
	}

	// Case 2: Setter provides current level
	setter := &mockLogLevelSetter{level: "debug"}
	svc = NewSettingService(repo, nil, nil, nil).WithLogLevelSetter(setter)
	settings, err = svc.GetAllSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings["xray_log_level"] != "debug" {
		t.Errorf("expected xray_log_level 'debug' from setter, got %s", settings["xray_log_level"])
	}

	// Case 3: Repo already has log level
	repo.data["xray_log_level"] = "error"
	settings, err = svc.GetAllSettings(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings["xray_log_level"] != "error" {
		t.Errorf("expected xray_log_level 'error' from repo, got %s", settings["xray_log_level"])
	}
}

func TestSettingService_UpdateSettings_LogLevel(t *testing.T) {
	repo := &mockSettingRepo{data: map[string]string{}}
	setter := &mockLogLevelSetter{level: "warning"}
	recompiler := &mockConfigRecompiler{}

	svc := NewSettingService(repo, nil, nil, nil).
		WithLogLevelSetter(setter).
		WithRecompiler(recompiler)

	body := map[string]interface{}{
		"xray_log_level": "info",
	}

	if err := svc.UpdateSettings(context.Background(), body, "127.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.data["xray_log_level"] != "info" {
		t.Errorf("expected repo xray_log_level 'info', got %s", repo.data["xray_log_level"])
	}
	if setter.level != "info" {
		t.Errorf("expected setter level 'info', got %s", setter.level)
	}
	if !recompiler.recompiled {
		t.Errorf("expected recompiler to be triggered on log level change")
	}

	// Re-updating with the same level should not trigger recompile
	recompiler.recompiled = false
	if err := svc.UpdateSettings(context.Background(), body, "127.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recompiler.recompiled {
		t.Errorf("expected recompiler NOT to be triggered when log level is unchanged")
	}

	// Updating with alias "warn" when setter is "warning" should normalize and not trigger recompile
	setter.level = "warning"
	recompiler.recompiled = false
	warnBody := map[string]interface{}{
		"xray_log_level": "warn",
	}
	if err := svc.UpdateSettings(context.Background(), warnBody, "127.0.0.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.data["xray_log_level"] != "warning" {
		t.Errorf("expected repo xray_log_level normalized to 'warning', got %s", repo.data["xray_log_level"])
	}
	if recompiler.recompiled {
		t.Errorf("expected recompiler NOT to be triggered for alias 'warn' when already 'warning'")
	}
}
