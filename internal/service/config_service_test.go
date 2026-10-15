package service_test

import (
	"context"
	"encoding/json"
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

type mockInboundRepoWithList struct {
	domain.InboundRepository
	inbounds []domain.Inbound
}

func (m *mockInboundRepoWithList) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}

type mockConfigStorageWithContent struct {
	service.ConfigStorage
	content []byte
}

func (m *mockConfigStorageWithContent) ReadRawConfig() ([]byte, error) {
	return m.content, nil
}

func (m *mockConfigStorageWithContent) WriteConfig(ctx context.Context, raw []byte) error {
	m.content = raw
	return nil
}

func TestConfigService_GetRoutingConfig_DeduplicationAndApiFiltering(t *testing.T) {
	rawJSON := []byte(`{
		"routing": {
			"domainStrategy": "IPIfNonMatch",
			"rules": [
				{"type": "field", "inboundTag": ["api"], "outboundTag": "api"},
				{"type": "field", "domain": ["geosite:cn"], "outboundTag": "direct"},
				{"type": "field", "domain": ["geosite:cn"], "outboundTag": "direct"},
				{"type": "field", "domain": ["geosite:category-ads-all"], "outboundTag": "block"}
			]
		}
	}`)
	cfgStorage := &mockConfigStorageWithContent{content: rawJSON}
	svc := service.NewConfigService(cfgStorage, nil, nil, nil, nil, nil)

	routing, err := svc.GetRoutingConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRoutingConfig failed: %v", err)
	}

	// api 规则被过滤，重复的 geosite:cn 被去重，最终只剩 2 条：geosite:cn 与 ads
	if len(routing.Rules) != 2 {
		t.Fatalf("Expected 2 rules, got %d", len(routing.Rules))
	}
	if routing.Rules[0].Domain[0] != "geosite:cn" {
		t.Errorf("Rule 0 expected geosite:cn, got %+v", routing.Rules[0])
	}
	if routing.Rules[1].Domain[0] != "geosite:category-ads-all" {
		t.Errorf("Rule 1 expected ads, got %+v", routing.Rules[1])
	}
}

func TestConfigService_EnsureSubRoutes_AppendsToTailWithoutDuplicate(t *testing.T) {
	rawJSON := []byte(`{
		"routing": {
			"domainStrategy": "IPIfNonMatch",
			"rules": [
				{"type": "field", "domain": ["geosite:google"], "outboundTag": "proxy"}
			]
		}
	}`)
	cfgStorage := &mockConfigStorageWithContent{content: rawJSON}
	inboundRepo := &mockInboundRepoWithList{
		inbounds: []domain.Inbound{
			{
				Tag:     "vless-in",
				Enabled: true,
				SubRoutesJson: `[
					{"routeId": 1, "outboundTag": "us-exit", "enabled": true},
					{"routeId": 2, "outboundTag": "jp-exit", "enabled": true}
				]`,
			},
		},
	}
	svc := service.NewConfigService(cfgStorage, nil, inboundRepo, nil, nil, nil)

	// 第一次调用 GetRoutingConfig：SubRoute 1 和 2 应该被自动追加在尾部
	routing, err := svc.GetRoutingConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRoutingConfig failed: %v", err)
	}

	// 总规则数：1 (google) + 2 (SubRoutes 追加至尾部) = 3
	if len(routing.Rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(routing.Rules))
	}
	if routing.Rules[0].Domain[0] != "geosite:google" {
		t.Errorf("Rule 0 should be google")
	}
	if routing.Rules[1].VlessRoute != "1" || routing.Rules[1].OutboundTag != "us-exit" {
		t.Errorf("Rule 1 should be vlessRoute 1 -> us-exit, got %+v", routing.Rules[1])
	}
	if routing.Rules[2].VlessRoute != "2" || routing.Rules[2].OutboundTag != "jp-exit" {
		t.Errorf("Rule 2 should be vlessRoute 2 -> jp-exit, got %+v", routing.Rules[2])
	}

	// 模拟将已追加的规则写回存储，再次获取，验证不重复追加
	cleanJSON, _ := json.Marshal(map[string]interface{}{
		"routing": routing,
	})
	cfgStorage.content = cleanJSON

	routing2, err := svc.GetRoutingConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRoutingConfig second call failed: %v", err)
	}
	if len(routing2.Rules) != 3 {
		t.Fatalf("Expected strictly 3 rules on second call (no duplicate explosion), got %d", len(routing2.Rules))
	}
}
