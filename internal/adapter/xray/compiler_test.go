package xray_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"panel/internal/adapter/xray"
	"panel/internal/domain"

	"github.com/xtls/xray-core/infra/conf/serial"
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

	// Rule 0: API
	if rules[0].OutboundTag != "api" {
		t.Errorf("Rule 0 should be api, got %s", rules[0].OutboundTag)
	}

	// Rule 1: Scoped SubRoute 1 (vlessRoute: 1, inboundTag: ["vless-tcp-reality"])
	if rules[1].VlessRoute != "1" || rules[1].OutboundTag != "jp-direct" {
		t.Errorf("Rule 1 mismatch: %+v", rules[1])
	}
	if len(rules[1].InboundTag) == 0 || rules[1].InboundTag[0] != "vless-tcp-reality" {
		t.Errorf("Rule 1 should be scoped to vless-tcp-reality, got %+v", rules[1].InboundTag)
	}

	// Rule 2: Scoped SubRoute 2 (vlessRoute: 2, inboundTag: ["vless-tcp-reality"])
	if rules[2].VlessRoute != "2" || rules[2].OutboundTag != "us-relay" {
		t.Errorf("Rule 2 mismatch: %+v", rules[2])
	}

	// Rule 3: Layer 3 自定义规则 (geosite:category-ads-all -> block)
	if rules[3].OutboundTag != "block" {
		t.Errorf("Rule 3 should be block, got %s", rules[3].OutboundTag)
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
