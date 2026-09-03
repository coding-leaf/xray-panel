package xray_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"

	"github.com/xtls/xray-core/infra/conf/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
)

func TestXrayCompiler_Compile(t *testing.T) {
	compiler := xray.NewXrayCompiler()

	inbounds := []domain.Inbound{
		{
			ID:             1,
			Tag:            "vless-tcp-reality",
			Port:           4434,
			Listen:         "0.0.0.0",
			Protocol:       "vless",
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["apple.com"]}}`,
			SubRoutesJson: `[
				{"id":"sr-1","name":"🇯🇵 日本原生","routeId":1,"outboundTag":"jp-direct","enabled":true},
				{"id":"sr-2","name":"🇺🇸 美国落地","routeId":2,"outboundTag":"us-relay","enabled":true}
			]`,
			Enabled: true,
		},
		{
			ID:             2,
			Tag:            "vless-xhttp",
			Port:           4435,
			Listen:         "0.0.0.0",
			Protocol:       "vless",
			StreamSettings: `{"network":"xhttp","security":"reality","xhttpSettings":{"path":"/xh","mode":"auto"}}`,
			Enabled:        true,
		},
		{
			ID:             3,
			Tag:            "trojan-tls",
			Port:           4436,
			Listen:         "0.0.0.0",
			Protocol:       "trojan",
			StreamSettings: `{"network":"tcp","security":"tls"}`,
			Enabled:        true,
		},
	}

	outbounds := []domain.Outbound{
		{
			Tag:      "direct",
			Protocol: "freedom",
		},
		{
			Tag:      "jp-direct",
			Protocol: "freedom",
		},
		{
			Tag:          "us-relay",
			Protocol:     "vless",
			SettingsJSON: `{"vnext":[{"address":"1.2.3.4","port":443}]}`,
		},
	}

	routing := &domain.RoutingConfig{
		DomainStrategy: "IPIfNonMatch",
		Rules: []domain.RoutingRule{
			{
				Type:        "field",
				Domain:      []string{"geosite:category-ads-all"},
				OutboundTag: "block",
			},
		},
	}

	dns := &domain.DNSConfig{
		Servers:       []interface{}{"8.8.8.8"},
		QueryStrategy: "UseIP",
	}

	users := []domain.User{
		{
			Email:       "user1@test.com",
			UUID:        "7117295b-4362-0000-a133-b969344dfcd5",
			InboundTags: "vless-tcp-reality,vless-xhttp,trojan-tls",
			Enabled:     true,
		},
		{
			Email:       "user2@test.com",
			UUID:        "11111111-2222-3333-4444-555555555555",
			InboundTags: "trojan-tls",
			Enabled:     true,
		},
	}

	cfg, err := compiler.Compile(inbounds, outbounds, routing, dns, users)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	if cfg == nil {
		t.Fatal("Expected non-nil XrayConfigFile")
	}

	// 1. 验证 Inbound 数量 (3 个业务 Inbound + 1 个 api Inbound)
	if len(cfg.Inbounds) != 4 {
		t.Errorf("Expected 4 inbounds, got %d", len(cfg.Inbounds))
	}

	// 2. 验证 VLESS TCP Reality 自动分配 Vision Flow
	var vlessTCPInbound *xray.XrayInbound
	var vlessXHTTPInbound *xray.XrayInbound
	for i := range cfg.Inbounds {
		if cfg.Inbounds[i].Tag == "vless-tcp-reality" {
			vlessTCPInbound = &cfg.Inbounds[i]
		}
		if cfg.Inbounds[i].Tag == "vless-xhttp" {
			vlessXHTTPInbound = &cfg.Inbounds[i]
		}
	}

	if vlessTCPInbound == nil {
		t.Fatal("vless-tcp-reality inbound not found")
	}

	var tcpSettings map[string]interface{}
	_ = json.Unmarshal(vlessTCPInbound.Settings, &tcpSettings)
	tcpClients := tcpSettings["clients"].([]interface{})
	if len(tcpClients) != 1 {
		t.Errorf("Expected 1 client for vless-tcp, got %d", len(tcpClients))
	} else {
		c0 := tcpClients[0].(map[string]interface{})
		if c0["flow"] != "xtls-rprx-vision" {
			t.Errorf("Expected flow xtls-rprx-vision, got %v", c0["flow"])
		}
	}

	// 3. 验证 XHTTP Inbound 自动禁用 Flow
	if vlessXHTTPInbound == nil {
		t.Fatal("vless-xhttp inbound not found")
	}
	var xhttpSettings map[string]interface{}
	_ = json.Unmarshal(vlessXHTTPInbound.Settings, &xhttpSettings)
	xhttpClients := xhttpSettings["clients"].([]interface{})
	if len(xhttpClients) != 1 {
		t.Errorf("Expected 1 client for vless-xhttp, got %d", len(xhttpClients))
	} else {
		c0 := xhttpClients[0].(map[string]interface{})
		if c0["flow"] != nil && c0["flow"] != "" {
			t.Errorf("Expected empty flow for xhttp, got %v", c0["flow"])
		}
	}

	// 4. 验证 Layer 1 ~ Layer 3 路由表顺序与 Scoped 限定
	rules := cfg.Routing.Rules
	if len(rules) < 4 {
		t.Fatalf("Expected at least 4 routing rules, got %d", len(rules))
	}

	// Rule 0: API 系统规则
	if rules[0].OutboundTag != "api" {
		t.Errorf("Rule 0 should be api, got %s", rules[0].OutboundTag)
	}

	// Rule 1: Layer 2 自定义规则优先 (geosite:category-ads-all -> block)
	if rules[1].OutboundTag != "block" {
		t.Errorf("Rule 1 should be block, got %s", rules[1].OutboundTag)
	}

	// Rule 2: Scoped SubRoute 1 兜底 (vlessRoute: 1, inboundTag: ["vless-tcp-reality"])
	if rules[2].VlessRoute != "1" || rules[2].OutboundTag != "jp-direct" {
		t.Errorf("Rule 2 mismatch: %+v", rules[2])
	}
	if len(rules[2].InboundTag) == 0 || rules[2].InboundTag[0] != "vless-tcp-reality" {
		t.Errorf("Rule 2 should be scoped to vless-tcp-reality, got %+v", rules[2].InboundTag)
	}

	// Rule 3: Scoped SubRoute 2 兜底 (vlessRoute: 2, inboundTag: ["vless-tcp-reality"])
	if rules[3].VlessRoute != "2" || rules[3].OutboundTag != "us-relay" {
		t.Errorf("Rule 3 mismatch: %+v", rules[3])
	}
}

func TestXrayRealityValidation(t *testing.T) {
	compiler := xray.NewXrayCompiler()

	// 模拟前端或旧配置中传入了混用的字段 (服务端带单数 serverName 和 publicKey，客户端带复数 serverNames 和 dest)
	inbounds := []domain.Inbound{
		{
			ID:             1,
			Tag:            "vless-reality",
			Port:           443,
			Protocol:       "vless",
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"www.titech.ac.jp:443","serverName":"www.titech.ac.jp","publicKey":"some_pbk","privateKey":"OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc","shortIds":["0123456789abcdef"]}}`,
			Enabled:        true,
		},
	}

	outbounds := []domain.Outbound{
		{
			Tag:          "us-reality-exit",
			Protocol:     "vless",
			SettingsJSON: `{"vnext":[{"address":"1.2.3.4","port":443,"users":[{"id":"7117295b-4362-0000-a133-b969344dfcd5","encryption":"none"}]}]}`,
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["apple.com"],"publicKey":"abc","shortIds":["123456"]}}`,
		},
	}

	users := []domain.User{
		{
			Email:       "test@test.com",
			UUID:        "7117295b-4362-0000-a133-b969344dfcd5",
			InboundTags: "vless-reality",
			Enabled:     true,
		},
	}

	jsonBytes, err := compiler.CompileToJSON(inbounds, outbounds, nil, nil, users)
	if err != nil {
		t.Fatalf("CompileToJSON error: %v", err)
	}

	t.Logf("Generated JSON:\n%s", string(jsonBytes))

	coreConfig, err := serial.DecodeJSONConfig(bytes.NewReader(jsonBytes))
	if err != nil {
		t.Fatalf("Xray Core serial.DecodeJSONConfig failed: %v", err)
	}
	if coreConfig == nil {
		t.Fatal("Expected non-nil coreConfig")
	}
}

func TestCompiler_RealityDestAndDynamicGRPCPort(t *testing.T) {
	c := xray.NewXrayCompiler(9090)

	inbounds := []domain.Inbound{
		{
			Tag:      "vless-reality",
			Listen:   "0.0.0.0",
			Port:     443,
			Protocol: "vless",
			StreamSettings: `{
				"network": "tcp",
				"security": "reality",
				"realitySettings": {
					"target": "gateway.icloud.com:443",
					"serverNames": ["gateway.icloud.com"],
					"privateKey": "testkey"
				}
			}`,
			Enabled: true,
		},
	}

	cfg, err := c.Compile(inbounds, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Verify dynamic gRPC port
	var apiInbound *xray.XrayInbound
	for _, in := range cfg.Inbounds {
		if in.Tag == "api" {
			apiInbound = &in
			break
		}
	}
	if apiInbound == nil || apiInbound.Port != 9090 {
		t.Fatalf("expected api inbound port 9090, got %v", apiInbound)
	}

	// 2. Verify reality dest normalization from target
	var realityInbound *xray.XrayInbound
	for _, in := range cfg.Inbounds {
		if in.Tag == "vless-reality" {
			realityInbound = &in
			break
		}
	}
	if realityInbound == nil || realityInbound.StreamSettings == nil || realityInbound.StreamSettings.RealitySettings == nil {
		t.Fatalf("expected reality settings, got nil")
	}
	if realityInbound.StreamSettings.RealitySettings.Dest != "gateway.icloud.com:443" {
		t.Errorf("expected dest gateway.icloud.com:443, got %q", realityInbound.StreamSettings.RealitySettings.Dest)
	}
}

func TestBuildAccountMessage_ShadowsocksCipher(t *testing.T) {
	user := &domain.User{UUID: "pass123"}

	tests := []struct {
		name         string
		settingsJSON string
		wantCipher   shadowsocks.CipherType
	}{
		{
			name:         "default aes-128-gcm",
			settingsJSON: `{}`,
			wantCipher:   shadowsocks.CipherType_AES_128_GCM,
		},
		{
			name:         "method aes-256-gcm",
			settingsJSON: `{"method":"aes-256-gcm"}`,
			wantCipher:   shadowsocks.CipherType_AES_256_GCM,
		},
		{
			name:         "cipher chacha20-poly1305",
			settingsJSON: `{"cipher":"chacha20-poly1305"}`,
			wantCipher:   shadowsocks.CipherType_CHACHA20_POLY1305,
		},
		{
			name:         "case insensitive and trimmed",
			settingsJSON: `{"method":" ChaCha20-Poly1305 "}`,
			wantCipher:   shadowsocks.CipherType_CHACHA20_POLY1305,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inbound := &domain.Inbound{
				Protocol:     "shadowsocks",
				SettingsJSON: tt.settingsJSON,
			}
			msg, err := xray.BuildAccountMessage(inbound, user)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			raw, err := msg.GetInstance()
			if err != nil {
				t.Fatalf("failed to get instance: %v", err)
			}
			acc, ok := raw.(*shadowsocks.Account)
			if !ok {
				t.Fatalf("expected *shadowsocks.Account, got %T", raw)
			}
			if acc.CipherType != tt.wantCipher {
				t.Errorf("expected cipher %v, got %v", tt.wantCipher, acc.CipherType)
			}
			if acc.Password != "pass123" {
				t.Errorf("expected password pass123, got %s", acc.Password)
			}
		})
	}
}

func TestCompiler_FilterInactiveUsers(t *testing.T) {
	compiler := xray.NewXrayCompiler()

	inbounds := []domain.Inbound{
		{
			ID:       1,
			Tag:      "vless-in",
			Port:     4433,
			Listen:   "0.0.0.0",
			Protocol: "vless",
			Enabled:  true,
		},
	}
	outbounds := []domain.Outbound{
		{
			Tag:      "direct",
			Protocol: "freedom",
		},
	}

	now := time.Now()
	activeUser := domain.User{
		Email:       "active@test.com",
		UUID:        "11111111-1111-1111-1111-111111111111",
		InboundTags: "vless-in",
		Enabled:     true,
		TotalBytes:  1000,
		UpBytes:     100,
		DownBytes:   200,
		ExpireTime:  now.Add(24 * time.Hour).UnixMilli(),
	}
	disabledUser := domain.User{
		Email:       "disabled@test.com",
		UUID:        "22222222-2222-2222-2222-222222222222",
		InboundTags: "vless-in",
		Enabled:     false,
		TotalBytes:  1000,
		UpBytes:     0,
		DownBytes:   0,
		ExpireTime:  now.Add(24 * time.Hour).UnixMilli(),
	}
	expiredUser := domain.User{
		Email:       "expired@test.com",
		UUID:        "33333333-3333-3333-3333-333333333333",
		InboundTags: "vless-in",
		Enabled:     true,
		TotalBytes:  1000,
		UpBytes:     100,
		DownBytes:   100,
		ExpireTime:  now.Add(-1 * time.Hour).UnixMilli(),
	}
	overQuotaUser := domain.User{
		Email:       "overquota@test.com",
		UUID:        "44444444-4444-4444-4444-444444444444",
		InboundTags: "vless-in",
		Enabled:     true,
		TotalBytes:  1000,
		UpBytes:     600,
		DownBytes:   500,
		ExpireTime:  now.Add(24 * time.Hour).UnixMilli(),
	}

	users := []domain.User{activeUser, disabledUser, expiredUser, overQuotaUser}

	cfg, err := compiler.Compile(inbounds, outbounds, nil, nil, users)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	var targetInb *xray.XrayInbound
	for i := range cfg.Inbounds {
		if cfg.Inbounds[i].Tag == "vless-in" {
			targetInb = &cfg.Inbounds[i]
			break
		}
	}
	if targetInb == nil {
		t.Fatal("vless-in inbound not found in compiled config")
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(targetInb.Settings, &settings); err != nil {
		t.Fatalf("Failed to unmarshal settings: %v", err)
	}

	rawClients, ok := settings["clients"].([]interface{})
	if !ok {
		t.Fatalf("Expected clients in settings, got %v", settings["clients"])
	}

	clientEmails := make(map[string]bool)
	for _, rawClient := range rawClients {
		cMap, ok := rawClient.(map[string]interface{})
		if ok {
			if email, ok := cMap["email"].(string); ok {
				clientEmails[email] = true
			}
		}
	}

	if !clientEmails["active@test.com"] {
		t.Errorf("Expected active user active@test.com to be included in clients")
	}
	if clientEmails["disabled@test.com"] {
		t.Errorf("Expected disabled user disabled@test.com to be excluded from clients")
	}
	if clientEmails["expired@test.com"] {
		t.Errorf("Expected expired user expired@test.com to be excluded from clients")
	}
	if clientEmails["overquota@test.com"] {
		t.Errorf("Expected over-quota user overquota@test.com to be excluded from clients")
	}
	if len(rawClients) != 1 {
		t.Errorf("Expected exactly 1 client, got %d", len(rawClients))
	}
}

func TestCompiler_ShadowsocksMethodBuild(t *testing.T) {
	c := xray.NewXrayCompiler(8080)

	t.Run("Shadowsocks with active user generates method and builds in Xray-core", func(t *testing.T) {
		inbounds := []domain.Inbound{
			{
				Tag:          "ss-in",
				Listen:       "0.0.0.0",
				Port:         8388,
				Protocol:     "shadowsocks",
				SettingsJSON: `{"method":"aes-128-gcm","network":"tcp,udp"}`,
				Enabled:      true,
			},
		}
		users := []domain.User{
			{
				Email:       "ss-user@test.com",
				UUID:        "test-password-123",
				Enabled:     true,
				InboundTags: "ss-in",
			},
		}
		outbounds := []domain.Outbound{{Tag: "direct", Protocol: "freedom"}}

		jsonBytes, err := c.CompileToJSON(inbounds, outbounds, nil, nil, users)
		if err != nil {
			t.Fatalf("CompileToJSON failed: %v", err)
		}

		coreConfig, err := serial.DecodeJSONConfig(bytes.NewReader(jsonBytes))
		if err != nil {
			t.Fatalf("DecodeJSONConfig failed: %v", err)
		}
		_, err = coreConfig.Build()
		if err != nil {
			t.Fatalf("coreConfig.Build() failed for Shadowsocks with user: %v", err)
		}
	})

	t.Run("Shadowsocks placeholder client generates method and builds in Xray-core", func(t *testing.T) {
		inbounds := []domain.Inbound{
			{
				Tag:          "ss-empty-in",
				Listen:       "0.0.0.0",
				Port:         8389,
				Protocol:     "shadowsocks",
				SettingsJSON: `{"method":"chacha20-poly1305","network":"tcp,udp"}`,
				Enabled:      true,
			},
		}
		outbounds := []domain.Outbound{{Tag: "direct", Protocol: "freedom"}}

		jsonBytes, err := c.CompileToJSON(inbounds, outbounds, nil, nil, nil)
		if err != nil {
			t.Fatalf("CompileToJSON failed: %v", err)
		}

		coreConfig, err := serial.DecodeJSONConfig(bytes.NewReader(jsonBytes))
		if err != nil {
			t.Fatalf("DecodeJSONConfig failed: %v", err)
		}
		_, err = coreConfig.Build()
		if err != nil {
			t.Fatalf("coreConfig.Build() failed for Shadowsocks placeholder: %v", err)
		}
	})
}




