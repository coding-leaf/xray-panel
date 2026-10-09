package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"
)

type mockUserRepoForConfig struct {
	domain.UserRepository
	users map[string]*domain.User
}

func (m *mockUserRepoForConfig) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepoForConfig) ListAll(ctx context.Context) ([]domain.User, error) {
	var list []domain.User
	for _, u := range m.users {
		list = append(list, *u)
	}
	return list, nil
}

type mockInboundRepoForConfig struct {
	domain.InboundRepository
	created []*domain.Inbound
	updated []*domain.Inbound
}

func (m *mockInboundRepoForConfig) Create(ctx context.Context, inbound *domain.Inbound) error {
	m.created = append(m.created, inbound)
	return nil
}

func (m *mockInboundRepoForConfig) Update(ctx context.Context, inbound *domain.Inbound) error {
	m.updated = append(m.updated, inbound)
	return nil
}

func (m *mockInboundRepoForConfig) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return nil, nil
}

type mockConfigStorage struct {
	service.ConfigStorage
}

func (m *mockConfigStorage) ReadRawConfig() ([]byte, error) {
	return []byte(`{"inbounds":[],"outbounds":[]}`), nil
}

func (m *mockConfigStorage) ValidateConfig(ctx context.Context, raw []byte) error {
	return nil
}

func (m *mockConfigStorage) WriteConfig(ctx context.Context, raw []byte) error {
	return nil
}

type mockCompiler struct {
	service.ConfigCompiler
}

func (m *mockCompiler) CompileToJSON(inbounds []domain.Inbound, outbounds []domain.Outbound, routing *domain.RoutingConfig, dns *domain.DNSConfig, users []domain.User) ([]byte, error) {
	return []byte(`{}`), nil
}

func TestConfigService_LegacyBoundUser_DoesNotBlockEditOrCreation(t *testing.T) {
	userRepo := &mockUserRepoForConfig{
		users: map[string]*domain.User{
			"valid@example.com": {ID: 1, Email: "valid@example.com"},
		},
	}
	inboundRepo := &mockInboundRepoForConfig{}
	cfgStorage := &mockConfigStorage{}
	compiler := &mockCompiler{}
	svc := service.NewConfigService(cfgStorage, nil, inboundRepo, userRepo, nil, compiler)

	ctx := context.Background()

	// 1. Create with empty BoundUserEmail should succeed
	err := svc.CreateInbound(ctx, &domain.Inbound{Tag: "socks-noauth"})
	if err != nil {
		t.Fatalf("expected nil error for empty bound user, got %v", err)
	}

	// 2. Create with existing BoundUserEmail should succeed
	err = svc.CreateInbound(ctx, &domain.Inbound{Tag: "socks-bound", BoundUserEmail: "valid@example.com"})
	if err != nil {
		t.Fatalf("expected nil error for valid bound user, got %v", err)
	}

	// 3. Create with deleted/non-existing legacy BoundUserEmail should ALSO succeed (does not block creation)
	err = svc.CreateInbound(ctx, &domain.Inbound{Tag: "socks-legacy", BoundUserEmail: "deleted-user@example.com"})
	if err != nil {
		t.Fatalf("expected nil error for legacy nonexistent bound user on create, got %v", err)
	}

	// 4. Update with deleted/nonexistent BoundUserEmail should ALSO succeed (does not block edit)
	err = svc.UpdateInbound(ctx, &domain.Inbound{Tag: "socks-legacy", BoundUserEmail: "deleted-user@example.com"})
	if err != nil {
		t.Fatalf("expected nil error for legacy nonexistent bound user on update, got %v", err)
	}
}

func TestConfigService_SaveConfigQuietly_GuardsAgainstSilentWipe(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "xray_cfg_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	corruptedConfigPath := filepath.Join(tmpDir, "corrupted_config.json")
	corruptedContent := []byte(`{ "outbounds": [ { "tag": "proxy", "protocol": invalid_json }`)
	if err := os.WriteFile(corruptedConfigPath, corruptedContent, 0644); err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}

	configMgr := xray.NewConfigManager(corruptedConfigPath, "")
	compiler := xray.NewXrayCompiler(8080)
	svc := service.NewConfigService(configMgr, nil, nil, nil, nil, compiler)

	err = svc.SaveConfigQuietly(context.Background(), "test guard")
	if err == nil {
		t.Fatalf("expected SaveConfigQuietly to fail on corrupted config to protect file, but returned nil!")
	}

	// Verify the original file content was preserved and NOT wiped
	preservedContent, err := os.ReadFile(corruptedConfigPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(preservedContent) != string(corruptedContent) {
		t.Fatalf("config file was overwritten despite read error! Got:\n%s", string(preservedContent))
	}
}
