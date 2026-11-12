package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestConfigService_OutboundMuxAndSchemaAlignment_RoundTrip(t *testing.T) {
	ctx := context.Background()

	rawInitialConfig := `{
		"outbounds": [
			{
				"tag": "proxy-us",
				"protocol": "trojan",
				"sendThrough": "192.168.1.50",
				"targetStrategy": "UseIPv4",
				"settings": {"servers":[{"address":"us.node.com","port":443,"password":"secret"}]},
				"streamSettings": {
					"network": "tcp",
					"security": "tls",
					"sockopt": {
						"dialerProxy": "warp-out",
						"acceptProxyProtocol": true
					},
					"tcpSettings": {
						"header": {
							"type": "http"
						}
					}
				},
				"mux": {
					"enabled": true,
					"concurrency": 8,
					"xudpConcurrency": 16,
					"xudpProxyUDP443": "reject"
				}
			},
			{
				"tag": "direct",
				"protocol": "freedom"
			}
		],
		"routing": {
			"domainStrategy": "IPIfNonMatch",
			"rules": [
				{
					"type": "field",
					"domain": ["geosite:google"],
					"source": ["192.168.1.0/24"],
					"sourcePort": "1000-2000",
					"domainMatcher": "mph",
					"outboundTag": "proxy-us"
				},
				{
					"type": "field",
					"domain": ["geosite:netflix"],
					"balancerTag": "balancer-pool"
				}
			]
		}
	}`

	cfgStorage := &mockConfigStorageWithContent{
		content: []byte(rawInitialConfig),
	}
	compiler := xray.NewXrayCompiler(10085)
	svc := service.NewConfigService(cfgStorage, nil, nil, nil, nil, compiler)

	// 1. 测试 ListOutbounds 正确反序列化 mux、sendThrough、targetStrategy
	outbounds, err := svc.ListOutbounds(ctx)
	if err != nil {
		t.Fatalf("ListOutbounds failed: %v", err)
	}
	if len(outbounds) != 2 {
		t.Fatalf("Expected 2 outbounds, got %d", len(outbounds))
	}
	proxyUs := outbounds[0]
	if proxyUs.Tag != "proxy-us" {
		t.Errorf("Expected tag proxy-us, got %q", proxyUs.Tag)
	}
	if proxyUs.SendThrough != "192.168.1.50" {
		t.Errorf("Expected sendThrough 192.168.1.50, got %q", proxyUs.SendThrough)
	}
	if proxyUs.TargetStrategy != "UseIPv4" {
		t.Errorf("Expected targetStrategy UseIPv4, got %q", proxyUs.TargetStrategy)
	}
	var muxObj struct {
		Enabled     bool  `json:"enabled"`
		Concurrency int16 `json:"concurrency"`
	}
	if err := json.Unmarshal([]byte(proxyUs.Mux), &muxObj); err != nil || !muxObj.Enabled || muxObj.Concurrency != 8 {
		t.Errorf("Expected mux to be read correctly with enabled=true, concurrency=8, got %q (err: %v)", proxyUs.Mux, err)
	}

	// 2. 测试 SaveOutbound 更新节点但不传 mux（空字符串）时，继承保留原有 Mux 配置
	updateWithoutMux := domain.Outbound{
		Tag:            "proxy-us",
		Protocol:       "trojan",
		SendThrough:    "192.168.1.50",
		TargetStrategy: "UseIPv4",
		SettingsJSON:   `{"servers":[{"address":"us-new.node.com","port":443,"password":"secret"}]}`,
		StreamSettings: proxyUs.StreamSettings,
		Mux:            "", // 旧版/缺省 caller 传递空字符串
	}
	if err := svc.SaveOutbound(ctx, updateWithoutMux); err != nil {
		t.Fatalf("SaveOutbound with empty mux failed: %v", err)
	}

	outboundsAfterEmptySave, _ := svc.ListOutbounds(ctx)
	if outboundsAfterEmptySave[0].Mux == "" {
		t.Errorf("SaveOutbound with empty mux should have preserved existing Mux, but Mux was cleared")
	}

	// 3. 测试 SaveOutbound 传入 "{}" 或 `{"enabled":false}` 时，显式清空 Mux
	updateWithDisabledMux := updateWithoutMux
	updateWithDisabledMux.Mux = `{"enabled":false}`
	if err := svc.SaveOutbound(ctx, updateWithDisabledMux); err != nil {
		t.Fatalf("SaveOutbound with disabled mux failed: %v", err)
	}

	outboundsAfterDisable, _ := svc.ListOutbounds(ctx)
	if outboundsAfterDisable[0].Mux != "" {
		t.Errorf("SaveOutbound with enabled:false should have cleared Mux, got %q", outboundsAfterDisable[0].Mux)
	}

	// 4. 测试 SaveOutbound 传入 "{}" 清空 Mux
	updateWithoutMux.Mux = `{"enabled":true,"concurrency":16}`
	_ = svc.SaveOutbound(ctx, updateWithoutMux)
	updateWithEmptyObjMux := updateWithoutMux
	updateWithEmptyObjMux.Mux = "{}"
	if err := svc.SaveOutbound(ctx, updateWithEmptyObjMux); err != nil {
		t.Fatalf("SaveOutbound with {} failed: %v", err)
	}
	outboundsAfterEmptyObj, _ := svc.ListOutbounds(ctx)
	if outboundsAfterEmptyObj[0].Mux != "" {
		t.Errorf("SaveOutbound with {} should have cleared Mux, got %q", outboundsAfterEmptyObj[0].Mux)
	}

	// 5. 测试 GetRoutingConfig 与 SaveRoutingConfig 对 Source, SourcePort, BalancerTag, DomainMatcher 的完整保留
	routing, err := svc.GetRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("GetRoutingConfig failed: %v", err)
	}
	if len(routing.Rules) != 2 {
		t.Fatalf("Expected 2 routing rules, got %d", len(routing.Rules))
	}
	rule1 := routing.Rules[0]
	if len(rule1.Source) != 1 || rule1.Source[0] != "192.168.1.0/24" {
		t.Errorf("Expected rule 1 source 192.168.1.0/24, got %+v", rule1.Source)
	}
	if rule1.SourcePort != "1000-2000" {
		t.Errorf("Expected rule 1 sourcePort 1000-2000, got %q", rule1.SourcePort)
	}
	if rule1.DomainMatcher != "mph" {
		t.Errorf("Expected rule 1 domainMatcher mph, got %q", rule1.DomainMatcher)
	}
	rule2 := routing.Rules[1]
	if rule2.BalancerTag != "balancer-pool" {
		t.Errorf("Expected rule 2 balancerTag balancer-pool, got %q", rule2.BalancerTag)
	}

	// 保存路由配置并验证重编译后仍完好
	if err := svc.SaveRoutingConfig(ctx, routing); err != nil {
		t.Fatalf("SaveRoutingConfig failed: %v", err)
	}
	reloadedRouting, _ := svc.GetRoutingConfig(ctx)
	if len(reloadedRouting.Rules) != 2 || reloadedRouting.Rules[0].SourcePort != "1000-2000" {
		t.Errorf("SaveRoutingConfig round trip failed: %+v", reloadedRouting.Rules)
	}

	// 6. 测试 ListOutbounds 从包含 disabled Mux 的原始配置中读取时，自动规范化为 ""
	rawConfigWithDisabledMux := `{
		"outbounds": [
			{
				"tag": "disabled-node",
				"protocol": "trojan",
				"mux": {"enabled": false, "concurrency": 8}
			},
			{
				"tag": "empty-mux-node",
				"protocol": "freedom",
				"mux": {}
			}
		]
	}`
	svcDisabled := service.NewConfigService(&mockConfigStorageWithContent{content: []byte(rawConfigWithDisabledMux)}, nil, nil, nil, nil, compiler)
	outboundsDisabled, err := svcDisabled.ListOutbounds(ctx)
	if err != nil {
		t.Fatalf("ListOutbounds with disabled mux failed: %v", err)
	}
	for _, ob := range outboundsDisabled {
		if ob.Mux != "" {
			t.Errorf("Expected Mux to be normalized to empty for %s, got %q", ob.Tag, ob.Mux)
		}
	}
}

type syncConfigStorage struct {
	mu      sync.Mutex
	content []byte
}

func (m *syncConfigStorage) ReadRawConfig() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(m.content))
	copy(cp, m.content)
	return cp, nil
}

func (m *syncConfigStorage) ValidateConfig(ctx context.Context, raw []byte) error {
	return nil
}

func (m *syncConfigStorage) WriteConfig(ctx context.Context, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(raw))
	copy(cp, raw)
	m.content = cp
	return nil
}

type mockNoopSupervisor struct{}

func (m *mockNoopSupervisor) Reload(ctx context.Context) error  { return nil }
func (m *mockNoopSupervisor) Restart(ctx context.Context) error { return nil }

func TestConfigService_ConcurrentSaveOutbound_NoLostUpdate(t *testing.T) {
	ctx := context.Background()
	cfgStorage := &syncConfigStorage{
		content: []byte(`{"inbounds":[],"outbounds":[]}`),
	}
	compiler := xray.NewXrayCompiler(10085)
	svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, nil, nil, nil, compiler)

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ob := domain.Outbound{
				Tag:      fmt.Sprintf("ob-%d", idx),
				Protocol: "freedom",
			}
			if err := svc.SaveOutbound(ctx, ob); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent SaveOutbound failed: %v", err)
	}

	outbounds, err := svc.ListOutbounds(ctx)
	if err != nil {
		t.Fatalf("ListOutbounds failed: %v", err)
	}
	foundTags := make(map[string]bool, len(outbounds))
	for _, ob := range outbounds {
		foundTags[ob.Tag] = true
	}
	for i := 0; i < workers; i++ {
		tag := fmt.Sprintf("ob-%d", i)
		if !foundTags[tag] {
			t.Fatalf("lost update detected: missing outbound tag %q (got %d outbounds)", tag, len(outbounds))
		}
	}
}

type failingInboundRepo struct {
	domain.InboundRepository
	listErr   error
	getTagErr error
	createErr error
}

func (m *failingInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return nil, nil
}

func (m *failingInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	if m.getTagErr != nil {
		return nil, m.getTagErr
	}
	return nil, domain.ErrNotFound
}

func (m *failingInboundRepo) Create(ctx context.Context, inbound *domain.Inbound) error {
	return m.createErr
}

type failingUserRepo struct {
	domain.UserRepository
	listErr     error
	getEmailErr error
}

func (m *failingUserRepo) ListAll(ctx context.Context) ([]domain.User, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return nil, nil
}

func (m *failingUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	if m.getEmailErr != nil {
		return nil, m.getEmailErr
	}
	return nil, domain.ErrNotFound
}

func TestConfigService_ErrorPropagation_NoSilentSwallow(t *testing.T) {
	ctx := context.Background()
	dbErr := errors.New("sqlite db locked")
	initialRaw := []byte(`{"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"domainStrategy":"IPIfNonMatch","rules":[]},"dns":{"servers":["1.1.1.1"]}}`)

	t.Run("SaveOutbound aborts on inboundRepo ListAll failure", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{listErr: dbErr}, &failingUserRepo{}, nil, xray.NewXrayCompiler(10085))
		err := svc.SaveOutbound(ctx, domain.Outbound{Tag: "proxy", Protocol: "freedom"})
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr, got %v", err)
		}
	})

	t.Run("SaveOutbound aborts on userRepo ListAll failure", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{}, &failingUserRepo{listErr: dbErr}, nil, xray.NewXrayCompiler(10085))
		err := svc.SaveOutbound(ctx, domain.Outbound{Tag: "proxy", Protocol: "freedom"})
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr, got %v", err)
		}
	})

	t.Run("SaveRoutingConfig aborts on inboundRepo ListAll failure", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{listErr: dbErr}, &failingUserRepo{}, nil, xray.NewXrayCompiler(10085))
		err := svc.SaveRoutingConfig(ctx, &domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"})
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr, got %v", err)
		}
	})

	t.Run("SaveDNSConfig aborts on userRepo ListAll failure", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{}, &failingUserRepo{listErr: dbErr}, nil, xray.NewXrayCompiler(10085))
		err := svc.SaveDNSConfig(ctx, &domain.DNSConfig{QueryStrategy: "UseIP"})
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr, got %v", err)
		}
	})

	t.Run("SaveAndApplyRawConfig propagates syncFromRawJSON DB error", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{getTagErr: dbErr}, &failingUserRepo{}, nil, xray.NewXrayCompiler(10085))
		rawWithInbound := []byte(`{"inbounds":[{"tag":"vless-in","port":443,"protocol":"vless","settings":{}}],"outbounds":[{"tag":"direct","protocol":"freedom"}]}`)
		err := svc.SaveAndApplyRawConfig(ctx, rawWithInbound)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr from syncFromRawJSON, got %v", err)
		}
	})

	t.Run("GetRoutingConfig propagates inboundRepo ListAll failure", func(t *testing.T) {
		cfgStorage := &syncConfigStorage{content: initialRaw}
		svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, &failingInboundRepo{listErr: dbErr}, &failingUserRepo{}, nil, xray.NewXrayCompiler(10085))
		_, err := svc.GetRoutingConfig(ctx)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected wrapped dbErr, got %v", err)
		}
	})
}

func TestConfigService_ConcurrentMixedOperations_NoDeadlockOrRace(t *testing.T) {
	ctx := context.Background()
	cfgStorage := &syncConfigStorage{
		content: []byte(`{"inbounds":[],"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"domainStrategy":"IPIfNonMatch","rules":[]},"dns":{"servers":["1.1.1.1"]}}`),
	}
	inboundRepo := &mockStatefulInboundRepo{byTag: map[string]*domain.Inbound{}}
	userRepo := &mockStatefulUserRepo{byEmail: map[string]*domain.User{}}
	svc := service.NewConfigService(cfgStorage, &mockNoopSupervisor{}, inboundRepo, userRepo, nil, xray.NewXrayCompiler(10085))

	const workers = 6
	const iters = 10
	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				var err error
				switch (id + i) % 5 {
				case 0:
					err = svc.RecompileAndApply(ctx, "recompile")
				case 1:
					err = svc.SaveOutbound(ctx, domain.Outbound{Tag: fmt.Sprintf("out-%d", id), Protocol: "freedom"})
				case 2:
					err = svc.SaveRoutingConfig(ctx, &domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"})
				case 3:
					err = svc.SaveDNSConfig(ctx, &domain.DNSConfig{QueryStrategy: "UseIP"})
				case 4:
					_, err = svc.GetRoutingConfig(ctx)
				}
				if err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent mixed operation failed: %v", err)
	}
}

type mockStatusSupervisor struct {
	mockNoopSupervisor
	active     bool
	err        error
	statusCall atomic.Int32
}

func (m *mockStatusSupervisor) GetStatus(ctx context.Context) (domain.ServiceStatus, error) {
	m.statusCall.Add(1)
	if m.err != nil {
		return domain.ServiceStatus{}, m.err
	}
	return domain.ServiceStatus{Active: m.active}, nil
}

func getUnusedTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func TestConfigService_ListInbounds_ConcurrentAndUDPHandling(t *testing.T) {
	ctx := context.Background()

	// 1. Open one real TCP listener on 127.0.0.1
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on 127.0.0.1: %v", err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// Open a listener specifically for disabled inbound to prove disabled inbounds are skipped
	disabledLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for disabled inbound: %v", err)
	}
	defer disabledLn.Close()
	disabledPort := disabledLn.Addr().(*net.TCPAddr).Port

	// Unused ports for UDP / closed TCP checks
	hy2Port := getUnusedTCPPort(t)
	dokoUDPPort := getUnusedTCPPort(t)
	dokoStreamUDPPort := getUnusedTCPPort(t)
	dokoTCPUDPPort := getUnusedTCPPort(t)

	inbounds := []domain.Inbound{
		{Tag: "tcp-open-empty-listen", Listen: "", Port: openPort, Protocol: "vless", Enabled: true},
		{Tag: "tcp-open-all-listen", Listen: "0.0.0.0", Port: openPort, Protocol: "trojan", Enabled: true},
		{Tag: "tcp-disabled", Listen: "127.0.0.1", Port: disabledPort, Protocol: "vless", Enabled: false},
		{Tag: "hy2-enabled", Listen: "0.0.0.0", Port: hy2Port, Protocol: "hysteria2", Enabled: true},
		{Tag: "hy2-disabled", Listen: "0.0.0.0", Port: hy2Port, Protocol: "hysteria2", Enabled: false},
		{Tag: "doko-settings-udp", Listen: "0.0.0.0", Port: dokoUDPPort, Protocol: "dokodemo-door", SettingsJSON: `{"network":"udp"}`, Enabled: true},
		{Tag: "doko-stream-udp", Listen: "0.0.0.0", Port: dokoStreamUDPPort, Protocol: "dokodemo-door", StreamSettings: `{"network":"udp"}`, Enabled: true},
		{Tag: "doko-tcp-udp", Listen: "127.0.0.1", Port: dokoTCPUDPPort, Protocol: "dokodemo-door", SettingsJSON: `{"network":"tcp,udp"}`, Enabled: true},
	}

	// Optional IPv6 loopback check for "::" -> "::1" mapping
	hasIPv6Loopback := false
	if ln6, err6 := net.Listen("tcp", "[::1]:0"); err6 == nil {
		defer ln6.Close()
		hasIPv6Loopback = true
		port6 := ln6.Addr().(*net.TCPAddr).Port
		inbounds = append(inbounds, domain.Inbound{
			Tag:      "tcp-ipv6-any",
			Listen:   "::",
			Port:     port6,
			Protocol: "vless",
			Enabled:  true,
		})
	}

	t.Run("supervisor active marks pure UDP alive with single GetStatus call", func(t *testing.T) {
		sup := &mockStatusSupervisor{active: true}
		svc := service.NewConfigService(&mockConfigStorage{}, sup, &mockInboundRepoWithList{inbounds: inbounds}, nil, nil, nil)

		got, err := svc.ListInbounds(ctx)
		if err != nil {
			t.Fatalf("ListInbounds failed: %v", err)
		}
		if calls := sup.statusCall.Load(); calls != 1 {
			t.Fatalf("expected GetStatus to be called exactly once, got %d", calls)
		}

		byTag := make(map[string]domain.Inbound, len(got))
		for _, ib := range got {
			byTag[ib.Tag] = ib
		}

		if !byTag["tcp-open-empty-listen"].IsAlive || byTag["tcp-open-empty-listen"].LatencyMs < 1 {
			t.Errorf("expected tcp-open-empty-listen alive with latency >= 1, got %+v", byTag["tcp-open-empty-listen"])
		}
		if !byTag["tcp-open-all-listen"].IsAlive || byTag["tcp-open-all-listen"].LatencyMs < 1 {
			t.Errorf("expected tcp-open-all-listen alive with latency >= 1, got %+v", byTag["tcp-open-all-listen"])
		}
		if byTag["tcp-disabled"].IsAlive || byTag["tcp-disabled"].LatencyMs != 0 {
			t.Errorf("expected disabled tcp inbound to be skipped (IsAlive=false, LatencyMs=0), got %+v", byTag["tcp-disabled"])
		}
		if !byTag["hy2-enabled"].IsAlive {
			t.Errorf("expected enabled hysteria2 to be alive when core is running, got %+v", byTag["hy2-enabled"])
		}
		if byTag["hy2-disabled"].IsAlive {
			t.Errorf("expected disabled hysteria2 to be offline, got %+v", byTag["hy2-disabled"])
		}
		if !byTag["doko-settings-udp"].IsAlive {
			t.Errorf("expected dokodemo-door network=udp (settings) to be alive when core is running, got %+v", byTag["doko-settings-udp"])
		}
		if !byTag["doko-stream-udp"].IsAlive {
			t.Errorf("expected dokodemo-door network=udp (stream) to be alive when core is running, got %+v", byTag["doko-stream-udp"])
		}
		if byTag["doko-tcp-udp"].IsAlive {
			t.Errorf("expected dokodemo-door network=tcp,udp on closed port to be offline, got %+v", byTag["doko-tcp-udp"])
		}
		if hasIPv6Loopback && !byTag["tcp-ipv6-any"].IsAlive {
			t.Errorf("expected '::' inbound to probe '::1' and be alive, got %+v", byTag["tcp-ipv6-any"])
		}
	})

	t.Run("supervisor inactive marks pure UDP offline", func(t *testing.T) {
		sup := &mockStatusSupervisor{active: false}
		svc := service.NewConfigService(&mockConfigStorage{}, sup, &mockInboundRepoWithList{inbounds: inbounds}, nil, nil, nil)

		got, err := svc.ListInbounds(ctx)
		if err != nil {
			t.Fatalf("ListInbounds failed: %v", err)
		}
		for _, ib := range got {
			if ib.Tag == "hy2-enabled" || ib.Tag == "doko-settings-udp" || ib.Tag == "doko-stream-udp" {
				if ib.IsAlive {
					t.Errorf("expected %s to be offline when core is stopped, got alive", ib.Tag)
				}
			}
		}
	})

	t.Run("nil supervisor defaults pure UDP alive", func(t *testing.T) {
		svc := service.NewConfigService(&mockConfigStorage{}, nil, &mockInboundRepoWithList{inbounds: []domain.Inbound{
			{Tag: "hy2-only", Listen: "0.0.0.0", Port: hy2Port, Protocol: "hysteria2", Enabled: true},
		}}, nil, nil, nil)

		got, err := svc.ListInbounds(ctx)
		if err != nil {
			t.Fatalf("ListInbounds failed: %v", err)
		}
		if len(got) != 1 || !got[0].IsAlive {
			t.Errorf("expected enabled hysteria2 with nil supervisor to default alive, got %+v", got)
		}
	})

	t.Run("concurrent TCP probes complete within single-hop timeout bound", func(t *testing.T) {
		var closedInbounds []domain.Inbound
		for i := 0; i < 6; i++ {
			closedInbounds = append(closedInbounds, domain.Inbound{
				Tag:      fmt.Sprintf("closed-%d", i),
				Listen:   "127.0.0.1",
				Port:     getUnusedTCPPort(t),
				Protocol: "vless",
				Enabled:  true,
			})
		}
		svc := service.NewConfigService(&mockConfigStorage{}, nil, &mockInboundRepoWithList{inbounds: closedInbounds}, nil, nil, nil)

		start := time.Now()
		got, err := svc.ListInbounds(ctx)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("ListInbounds failed: %v", err)
		}
		if len(got) != 6 {
			t.Fatalf("expected 6 inbounds, got %d", len(got))
		}
		for _, ib := range got {
			if ib.IsAlive {
				t.Errorf("expected closed inbound %s to be offline", ib.Tag)
			}
		}
		// Sequential 6 * 200ms = 1200ms on Windows timeout; concurrent must finish well under 800ms
		if elapsed > 800*time.Millisecond {
			t.Fatalf("expected concurrent probing to finish within 800ms, took %v", elapsed)
		}
	})
}

type countingConfigStorage struct {
	raw        []byte
	err        error
	readCalls  atomic.Int32
	writeCalls atomic.Int32
}

func (m *countingConfigStorage) ReadRawConfig() ([]byte, error) {
	m.readCalls.Add(1)
	if m.err != nil {
		return nil, m.err
	}
	return m.raw, nil
}

func (m *countingConfigStorage) ValidateConfig(ctx context.Context, raw []byte) error {
	return nil
}

func (m *countingConfigStorage) WriteConfig(ctx context.Context, raw []byte) error {
	m.writeCalls.Add(1)
	return nil
}

type countingInboundRepo struct {
	domain.InboundRepository
	inbounds  []domain.Inbound
	listCalls atomic.Int32
}

func (m *countingInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	m.listCalls.Add(1)
	return m.inbounds, nil
}

type recordingSnapshotRepo struct {
	domain.ConfigSnapshotRepository
	saved []*domain.ConfigSnapshot
}

func (m *recordingSnapshotRepo) Save(ctx context.Context, s *domain.ConfigSnapshot) error {
	m.saved = append(m.saved, s)
	return nil
}

type capturingCompiler struct {
	outbounds []domain.Outbound
	routing   *domain.RoutingConfig
	dns       *domain.DNSConfig
	inbounds  []domain.Inbound
	users     []domain.User
}

func (m *capturingCompiler) CompileToJSON(inbounds []domain.Inbound, outbounds []domain.Outbound, routing *domain.RoutingConfig, dns *domain.DNSConfig, users []domain.User) ([]byte, error) {
	m.inbounds = inbounds
	m.outbounds = outbounds
	m.routing = routing
	m.dns = dns
	m.users = users
	return []byte(`{"inbounds":[],"outbounds":[]}`), nil
}

func TestConfigService_SaveConfigQuietly_SingleReadAndListAll(t *testing.T) {
	ctx := context.Background()
	rawConfig := []byte(`{
		// JSONC comment should be stripped once
		"outbounds": [
			{"tag": "direct", "protocol": "freedom", "targetStrategy": "UseIPv4v6"}
		],
		"routing": {
			"domainStrategy": "AsIs",
			"rules": [
				{"type": "field", "inboundTag": ["api"], "outboundTag": "api"},
				{"type": "field", "domain": ["geosite:cn"], "outboundTag": "direct"}
			]
		},
		"dns": {
			"servers": ["1.1.1.1"],
			"queryStrategy": "UseIPv4"
		}
	}`)

	storage := &countingConfigStorage{raw: rawConfig}
	inboundRepo := &countingInboundRepo{
		inbounds: []domain.Inbound{
			{
				Tag:      "vless-in",
				Port:     443,
				Protocol: "vless",
				Enabled:  true,
			},
		},
	}
	userRepo := &mockUserRepoForConfig{
		users: map[string]*domain.User{
			"alice@example.com": {ID: 1, Email: "alice@example.com", Enabled: true},
		},
	}
	snapRepo := &recordingSnapshotRepo{}
	compiler := &capturingCompiler{}

	svc := service.NewConfigService(storage, nil, inboundRepo, userRepo, snapRepo, compiler)
	if err := svc.SaveConfigQuietly(ctx, "single-read-check"); err != nil {
		t.Fatalf("SaveConfigQuietly failed: %v", err)
	}

	if got := storage.readCalls.Load(); got != 1 {
		t.Fatalf("expected ReadRawConfig to be called exactly 1 time, got %d", got)
	}
	if got := inboundRepo.listCalls.Load(); got != 1 {
		t.Fatalf("expected inboundRepo.ListAll to be called exactly 1 time, got %d", got)
	}
	if got := storage.writeCalls.Load(); got != 1 {
		t.Fatalf("expected WriteConfig to be called 1 time, got %d", got)
	}
	if len(snapRepo.saved) != 1 || snapRepo.saved[0].RawConfig != string(rawConfig) {
		t.Fatalf("expected snapshot to reuse exact oldRaw bytes, got %+v", snapRepo.saved)
	}
	if len(compiler.outbounds) != 1 || compiler.outbounds[0].Tag != "direct" || compiler.outbounds[0].TargetStrategy != "UseIPv4v6" {
		t.Fatalf("unexpected compiled outbounds: %+v", compiler.outbounds)
	}
	if compiler.routing == nil || compiler.routing.DomainStrategy != "AsIs" || len(compiler.routing.Rules) != 1 {
		t.Fatalf("unexpected compiled routing: %+v", compiler.routing)
	}
	if compiler.dns == nil || compiler.dns.QueryStrategy != "UseIPv4" {
		t.Fatalf("unexpected compiled dns: %+v", compiler.dns)
	}
	if len(compiler.inbounds) != 1 || len(compiler.users) != 1 {
		t.Fatalf("unexpected compiled inbounds=%d users=%d", len(compiler.inbounds), len(compiler.users))
	}

	t.Run("missing raw config falls back gracefully without snapshot", func(t *testing.T) {
		missingStorage := &countingConfigStorage{err: domain.ErrNotFound}
		missingSnap := &recordingSnapshotRepo{}
		missingCompiler := &capturingCompiler{}
		missingInbounds := &countingInboundRepo{inbounds: inboundRepo.inbounds}

		svc2 := service.NewConfigService(missingStorage, nil, missingInbounds, userRepo, missingSnap, missingCompiler)
		if err := svc2.SaveConfigQuietly(ctx, "missing-raw"); err != nil {
			t.Fatalf("SaveConfigQuietly with ErrNotFound failed: %v", err)
		}
		if got := missingStorage.readCalls.Load(); got != 1 {
			t.Fatalf("expected 1 ReadRawConfig call, got %d", got)
		}
		if len(missingSnap.saved) != 0 {
			t.Fatalf("expected 0 snapshots when config does not exist, got %d", len(missingSnap.saved))
		}
		if missingCompiler.routing == nil || missingCompiler.routing.DomainStrategy != "IPIfNonMatch" {
			t.Fatalf("expected default routing fallback, got %+v", missingCompiler.routing)
		}
	})
}




