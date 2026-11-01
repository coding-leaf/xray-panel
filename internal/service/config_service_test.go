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

func TestConfigService_EnsureSubRoutes_CleansOrphanedAndSyncsUpdatedSubRoutes(t *testing.T) {
	// 模拟 config.json 中残留了旧的 vlessRoute: "6" 和 "999" 僵尸规则，以及旧出站的 vlessRoute: "2" -> "old-out"
	rawJSON := []byte(`{
		"routing": {
			"domainStrategy": "IPIfNonMatch",
			"rules": [
				{"type": "field", "inboundTag": ["ipv4入口"], "vlessRoute": "1", "outboundTag": "direct"},
				{"type": "field", "inboundTag": ["ipv4入口"], "vlessRoute": "2", "outboundTag": "old-out"},
				{"type": "field", "inboundTag": ["ipv4入口"], "vlessRoute": "999", "outboundTag": "direct"},
				{"type": "field", "inboundTag": ["ipv4入口"], "vlessRoute": "6", "outboundTag": "out-us2"}
			]
		}
	}`)
	cfgStorage := &mockConfigStorageWithContent{content: rawJSON}
	inboundRepo := &mockInboundRepoWithList{
		inbounds: []domain.Inbound{
			{
				Tag:     "ipv4入口",
				Enabled: true,
				SubRoutesJson: `[
					{"routeId": 1, "name": "日本直连", "outboundTag": "direct", "enabled": true},
					{"routeId": 2, "name": "休斯顿解锁", "outboundTag": "out-us2", "enabled": true},
					{"routeId": 3, "name": "洛杉矶速度", "outboundTag": "out-us", "enabled": true},
					{"routeId": 4, "name": "土耳其特殊", "outboundTag": "Out-TR", "enabled": true}
				]`,
			},
		},
	}
	svc := service.NewConfigService(cfgStorage, nil, inboundRepo, nil, nil, nil)

	routing, err := svc.GetRoutingConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRoutingConfig failed: %v", err)
	}

	// 僵尸规则 999 和 6 应被清除，只剩 1, 2, 3, 4 共 4 条规则，且 vlessRoute 2 的出站更新为 out-us2
	if len(routing.Rules) != 4 {
		t.Fatalf("Expected 4 rules (1,2,3,4 without zombie 6/999), got %d: %+v", len(routing.Rules), routing.Rules)
	}
	expected := map[string]string{
		"1": "direct",
		"2": "out-us2",
		"3": "out-us",
		"4": "Out-TR",
	}
	for _, r := range routing.Rules {
		wantOut, ok := expected[r.VlessRoute]
		if !ok {
			t.Errorf("Unexpected vlessRoute %q still present in rules: %+v", r.VlessRoute, r)
			continue
		}
		if r.OutboundTag != wantOut {
			t.Errorf("vlessRoute %s expected outboundTag %q, got %q", r.VlessRoute, wantOut, r.OutboundTag)
		}
	}
}

type mockStatefulInboundRepo struct {
	domain.InboundRepository
	byTag map[string]*domain.Inbound
}

func (m *mockStatefulInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	if in, ok := m.byTag[tag]; ok {
		cp := *in
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockStatefulInboundRepo) Update(ctx context.Context, inbound *domain.Inbound) error {
	cp := *inbound
	m.byTag[inbound.Tag] = &cp
	return nil
}

func (m *mockStatefulInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	var list []domain.Inbound
	for _, in := range m.byTag {
		list = append(list, *in)
	}
	return list, nil
}

type mockStatefulUserRepo struct {
	domain.UserRepository
	byEmail map[string]*domain.User
}

func (m *mockStatefulUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if u, ok := m.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockStatefulUserRepo) Update(ctx context.Context, user *domain.User) error {
	cp := *user
	m.byEmail[user.Email] = &cp
	return nil
}

func (m *mockStatefulUserRepo) ListAll(ctx context.Context) ([]domain.User, error) {
	var list []domain.User
	for _, u := range m.byEmail {
		list = append(list, *u)
	}
	return list, nil
}

func TestConfigService_SyncFromFile_PreservesEmptyFlowAcrossRestart(t *testing.T) {
	ctx := context.Background()

	// 1. 初始状态：VLESS + TCP + Reality 入站在面板被显式设为无流控 ("flow": "none")，
	//    同时有用户 user-none@test.com 显式设置 Flow: "none"，user-follow@test.com 设置 Flow: "" (跟随节点)。
	inboundRepo := &mockStatefulInboundRepo{
		byTag: map[string]*domain.Inbound{
			"vless-reality-noflow": {
				ID:             1,
				Tag:            "vless-reality-noflow",
				Port:           443,
				Listen:         "0.0.0.0",
				Protocol:       "vless",
				Enabled:        true,
				SettingsJSON:   `{"decryption":"none","flow":"none"}`,
				StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"www.microsoft.com:443","serverNames":["www.microsoft.com"],"privateKey":"priv","shortIds":[""]}}`,
			},
		},
	}
	userRepo := &mockStatefulUserRepo{
		byEmail: map[string]*domain.User{
			"user-follow@test.com": {
				ID:          1,
				Email:       "user-follow@test.com",
				UUID:        "11111111-1111-4111-8111-111111111111",
				InboundTags: "vless-reality-noflow",
				Flow:        "",
				Enabled:     true,
			},
			"user-none@test.com": {
				ID:          2,
				Email:       "user-none@test.com",
				UUID:        "22222222-2222-4222-8222-222222222222",
				InboundTags: "vless-reality-noflow",
				Flow:        "none",
				Enabled:     true,
			},
		},
	}

	cfgStorage := &mockConfigStorageWithContent{
		content: []byte(`{"inbounds":[],"outbounds":[{"tag":"direct","protocol":"freedom"}]}`),
	}
	compiler := xray.NewXrayCompiler(10085)
	svc := service.NewConfigService(cfgStorage, nil, inboundRepo, userRepo, nil, compiler)

	// 2. 首次编译落盘 (模拟用户在面板保存无流控配置)
	if err := svc.RecompileAndApply(ctx, "保存无流控入站"); err != nil {
		t.Fatalf("First RecompileAndApply failed: %v", err)
	}

	// 3. 模拟面板重启：main.go 依次调用 SyncFromFile -> RecompileAndApply
	if err := svc.SyncFromFile(ctx); err != nil {
		t.Fatalf("SyncFromFile failed: %v", err)
	}
	if err := svc.RecompileAndApply(ctx, "面板启动自动同步与编译配置"); err != nil {
		t.Fatalf("Restart RecompileAndApply failed: %v", err)
	}

	// 4. 断言重启后 DB 中的 Inbound SettingsJSON 仍然保持 flow="none"，绝不能丢失
	afterInbound, _ := inboundRepo.GetByTag(ctx, "vless-reality-noflow")
	var settingsMap map[string]interface{}
	_ = json.Unmarshal([]byte(afterInbound.SettingsJSON), &settingsMap)
	if settingsMap["flow"] != "none" {
		t.Errorf("Expected inbound SettingsJSON flow to remain 'none' after restart, got %v (SettingsJSON=%s)", settingsMap["flow"], afterInbound.SettingsJSON)
	}

	// 5. 断言重启后 DB 中的 User Flow 策略未被篡改
	uNone, _ := userRepo.GetByEmail(ctx, "user-none@test.com")
	if uNone.Flow != "none" {
		t.Errorf("Expected user-none Flow to remain 'none' after restart, got %q", uNone.Flow)
	}

	// 6. 断言重启后重新编译出的 config.json 中，clients 的 flow 依然为空，绝不能自动复活为 xtls-rprx-vision
	var compiledRoot struct {
		Inbounds []struct {
			Tag      string `json:"tag"`
			Settings struct {
				Clients []struct {
					Email string `json:"email"`
					Flow  string `json:"flow"`
				} `json:"clients"`
			} `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cfgStorage.content, &compiledRoot); err != nil {
		t.Fatalf("Failed to unmarshal compiled config: %v", err)
	}
	for _, ib := range compiledRoot.Inbounds {
		if ib.Tag != "vless-reality-noflow" {
			continue
		}
		for _, c := range ib.Settings.Clients {
			if c.Flow != "" {
				t.Errorf("Client %s flow resurrected to %q after restart! Expected empty flow", c.Email, c.Flow)
			}
		}
	}
}


