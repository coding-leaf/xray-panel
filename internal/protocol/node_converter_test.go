package protocol_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"panel/internal/domain"
	"panel/internal/protocol"
)

func TestBuildShareLink_FullParameterParity(t *testing.T) {
	user := &domain.User{
		UUID:  "11111111-2222-3333-4444-555555555555",
		Email: "test@example.com",
	}

	t.Run("Trojan WS TLS generates sni, path, host", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "trojan-ws-in",
			Protocol:       "trojan",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "Trojan-Node",
			StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"trojan.example.com","alpn":["h2","http/1.1"]},"wsSettings":{"path":"/trojan-path","headers":{"Host":"trojan.example.com"}}}`,
		}

		link := protocol.BuildShareLink(inbound, user, "", 0)
		if !strings.HasPrefix(link, "trojan://") {
			t.Fatalf("expected trojan:// prefix, got %s", link)
		}

		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("failed to parse generated URL: %v", err)
		}

		q := u.Query()
		if q.Get("sni") != "trojan.example.com" {
			t.Errorf("expected sni to be trojan.example.com, got %s", q.Get("sni"))
		}
		if q.Get("path") != "/trojan-path" {
			t.Errorf("expected path to be /trojan-path, got %s", q.Get("path"))
		}
		if q.Get("host") != "trojan.example.com" {
			t.Errorf("expected host to be trojan.example.com, got %s", q.Get("host"))
		}
		if q.Get("alpn") != "h2,http/1.1" {
			t.Errorf("expected alpn to be h2,http/1.1, got %s", q.Get("alpn"))
		}
	})

	t.Run("VMess WS TLS generates sni, path, host", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vmess-ws-in",
			Protocol:       "vmess",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "VMess-Node",
			StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"vmess.example.com"},"wsSettings":{"path":"/vmess-path","headers":{"Host":"vmess.example.com"}}}`,
		}

		link := protocol.BuildShareLink(inbound, user, "", 0)
		if !strings.HasPrefix(link, "vmess://") {
			t.Fatalf("expected vmess:// prefix, got %s", link)
		}

		b64Content := strings.TrimPrefix(link, "vmess://")
		jsonBytes, err := base64.StdEncoding.DecodeString(b64Content)
		if err != nil {
			t.Fatalf("failed to base64 decode vmess link: %v", err)
		}

		var vmessObj map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &vmessObj); err != nil {
			t.Fatalf("failed to unmarshal vmess JSON: %v", err)
		}

		if vmessObj["sni"] != "vmess.example.com" {
			t.Errorf("expected vmess sni to be vmess.example.com, got %v", vmessObj["sni"])
		}
		if vmessObj["path"] != "/vmess-path" {
			t.Errorf("expected vmess path to be /vmess-path, got %v", vmessObj["path"])
		}
		if vmessObj["host"] != "vmess.example.com" {
			t.Errorf("expected vmess host to be vmess.example.com, got %v", vmessObj["host"])
		}
	})

	t.Run("VLESS REALITY adapts to singular serverName and shortId", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "reality-singular-in",
			Protocol:       "vless",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "Reality-Node",
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"apple.com","shortId":"aabbcc1122","publicKey":"test-pbk"}}`,
		}

		link := protocol.BuildShareLink(inbound, user, "", 0)
		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("failed to parse generated URL: %v", err)
		}

		q := u.Query()
		if q.Get("sni") != "apple.com" {
			t.Errorf("expected reality sni to be apple.com from singular serverName, got %s", q.Get("sni"))
		}
		if q.Get("sid") != "aabbcc1122" {
			t.Errorf("expected reality sid to be aabbcc1122 from singular shortId, got %s", q.Get("sid"))
		}
		if q.Get("pbk") != "test-pbk" {
			t.Errorf("expected reality pbk to be test-pbk, got %s", q.Get("pbk"))
		}
	})

	t.Run("IPv6 address preserved without truncation", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-ipv6",
			Protocol:       "vless",
			Port:           443,
			ExternalHost:   "2408:8207:dead:beef::1",
			StreamSettings: `{"network":"tcp","security":"none"}`,
		}

		link := protocol.BuildShareLink(inbound, user, "", 0)
		if !strings.Contains(link, "@[2408:8207:dead:beef::1]:443") {
			t.Fatalf("expected bracketed IPv6 in link, got: %s", link)
		}

		node := protocol.InboundToNodeConfig(inbound, user, "", 0)
		if node.Address != "2408:8207:dead:beef::1" {
			t.Errorf("expected raw IPv6 address preserved in NodeConfig, got: %s", node.Address)
		}
	})

	t.Run("SubRoutes conversion with InboundsToNodeConfigs", func(t *testing.T) {
		subRoutes := []domain.SubRoute{
			{
				ID:          "1",
				Name:        "🇯🇵 日本原生直连",
				RouteID:     1,
				OutboundTag: "direct",
				Enabled:     true,
			},
			{
				ID:          "2",
				Name:        "🇺🇸 美国中转落地",
				RouteID:     2,
				OutboundTag: "us-test",
				Enabled:     true,
			},
			{
				ID:          "3",
				Name:        "🚫 停用线路",
				RouteID:     3,
				OutboundTag: "block",
				Enabled:     false,
			},
		}
		subRoutesBytes, _ := json.Marshal(subRoutes)

		inbound := domain.Inbound{
			Tag:            "vless-reality",
			Port:           4434,
			ExternalPort:   443,
			ExternalHost:   "198.51.100.1",
			Protocol:       "vless",
			StreamSettings: `{"network":"xhttp","security":"reality","realitySettings":{"publicKey":"FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww"}}`,
			SubRoutesJson:  string(subRoutesBytes),
			Enabled:        true,
		}

		userWithInbound := &domain.User{
			Email:       "user@example.com",
			UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
			InboundTags: "vless-reality",
			Enabled:     true,
		}

		nodes := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, userWithInbound, "198.51.100.1", 443, "")
		if len(nodes) != 2 {
			t.Fatalf("expected 2 nodes for enabled subroutes, got %d", len(nodes))
		}

		if nodes[0].Name != "🇯🇵 日本原生直连" || nodes[0].Port != 443 {
			t.Errorf("unexpected node 0: %+v", nodes[0])
		}
		if nodes[0].UUID != "7117295b-4362-0001-a133-b969344dfcd5" {
			t.Errorf("unexpected node 0 UUID: %s", nodes[0].UUID)
		}
		if nodes[1].Name != "🇺🇸 美国中转落地" || nodes[1].Port != 443 {
			t.Errorf("unexpected node 1: %+v", nodes[1])
		}
		if nodes[1].UUID != "7117295b-4362-0002-a133-b969344dfcd5" {
			t.Errorf("unexpected node 1 UUID: %s", nodes[1].UUID)
		}
	})

	t.Run("Shadowsocks 2022 link generation and cipher parity", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:          "ss2022-node",
			Protocol:     "shadowsocks",
			Port:         8388,
			Listen:       "1.2.3.4",
			Remark:       "SS2022-Blake3",
			SettingsJSON: `{"method":"2022-blake3-aes-128-gcm"}`,
		}
		userSS := &domain.User{
			UUID:  "WmdGZzNuWk1QVGJIdkJyV1hKdlhRQUFBQUFBQUFBQUE=",
			Email: "ss2022@example.com",
		}
		link := protocol.BuildShareLink(inbound, userSS, "", 0)
		if !strings.HasPrefix(link, "ss://") {
			t.Fatalf("expected ss:// prefix, got %s", link)
		}
		node := protocol.InboundToNodeConfig(inbound, userSS, "", 0)
		if node.GetParam("method", "") != "2022-blake3-aes-128-gcm" {
			t.Errorf("expected method 2022-blake3-aes-128-gcm, got %s", node.GetParam("method", ""))
		}
		clash, err := protocol.ToClash(node)
		if err != nil {
			t.Fatalf("ToClash failed: %v", err)
		}
		if clash["cipher"] != "2022-blake3-aes-128-gcm" {
			t.Errorf("expected clash cipher 2022-blake3-aes-128-gcm, got %v", clash["cipher"])
		}
		singbox, err := protocol.ToSingBox(node)
		if err != nil {
			t.Fatalf("ToSingBox failed: %v", err)
		}
		if singbox["method"] != "2022-blake3-aes-128-gcm" {
			t.Errorf("expected singbox method 2022-blake3-aes-128-gcm, got %v", singbox["method"])
		}

		link, linkErr := protocol.FormatLink(node)
		if linkErr != nil {
			t.Fatalf("FormatLink failed: %v", linkErr)
		}
		if !strings.HasPrefix(link, "ss://") {
			t.Fatalf("expected ss:// prefix, got %s", link)
		}
	})

	t.Run("Shadowsocks 2022 protocol tag alias ss2022", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "ss2022-tag",
			Protocol:       "ss2022",
			Listen:         "0.0.0.0",
			Port:           8388,
			ExternalHost:   "ss.example.com",
			SettingsJSON:   `{"method":"2022-blake3-aes-256-gcm"}`,
			StreamSettings: `{"network":"tcp","security":"none"}`,
			Remark:         "SS2022 Node",
			Enabled:        true,
		}
		userSS := &domain.User{
			Email:       "ss-user@example.com",
			UUID:        "mypassword123",
			InboundTags: "ss2022-tag",
			Enabled:     true,
		}
		node := protocol.InboundToNodeConfig(inbound, userSS, "", 0)
		if node.GetParam("method", "") != "2022-blake3-aes-256-gcm" {
			t.Errorf("expected method 2022-blake3-aes-256-gcm, got %s", node.GetParam("method", ""))
		}
		link, err := protocol.FormatLink(node)
		if err != nil {
			t.Fatalf("FormatLink failed: %v", err)
		}
		if !strings.HasPrefix(link, "ss://") {
			t.Fatalf("expected ss:// prefix, got %s", link)
		}
	})

	t.Run("VLESS TCP REALITY with inbound flow none still allows explicit user vision", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-flow-none",
			Protocol:       "vless",
			Listen:         "0.0.0.0",
			Port:           443,
			ExternalHost:   "example.com",
			SettingsJSON:   `{"flow":"none"}`,
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"apple.com","shortId":"aabbcc1122","publicKey":"test-pbk"}}`,
			Remark:         "VLESS None Flow Node",
			Enabled:        true,
		}
		userWithFlow := &domain.User{
			Email:       "flow-user@example.com",
			UUID:        "7117295b-4362-0001-a133-b969344dfcd5",
			InboundTags: "vless-flow-none",
			Flow:        "xtls-rprx-vision",
			Enabled:     true,
		}

		node := protocol.InboundToNodeConfig(inbound, userWithFlow, "", 0)
		if flow := node.GetParam("flow", ""); flow != "xtls-rprx-vision" {
			t.Errorf("expected explicit user flow xtls-rprx-vision to override inbound none, got: %s", flow)
		}

		link := protocol.BuildShareLink(inbound, userWithFlow, "", 0)
		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("failed to parse share link: %v", err)
		}
		if qFlow := u.Query().Get("flow"); qFlow != "xtls-rprx-vision" {
			t.Errorf("expected share link flow query param to be xtls-rprx-vision, got: %s", qFlow)
		}

		userFollow := &domain.User{
			Email:       "follow-user@example.com",
			UUID:        "7117295b-4362-0001-a133-b969344dfcd7",
			InboundTags: "vless-flow-none",
			Flow:        "",
			Enabled:     true,
		}
		nodeFollow := protocol.InboundToNodeConfig(inbound, userFollow, "", 0)
		if flow := nodeFollow.GetParam("flow", ""); flow != "" {
			t.Errorf("expected empty flow when user follows inbound none, got: %s", flow)
		}
	})

	t.Run("VLESS TCP REALITY with user flow none also results in empty flow", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-no-inbound-flow",
			Protocol:       "vless",
			Listen:         "0.0.0.0",
			Port:           443,
			ExternalHost:   "example.com",
			SettingsJSON:   `{}`,
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"apple.com","shortId":"aabbcc1122","publicKey":"test-pbk"}}`,
			Remark:         "VLESS Node",
			Enabled:        true,
		}
		userNoneFlow := &domain.User{
			Email:       "user-none@example.com",
			UUID:        "7117295b-4362-0001-a133-b969344dfcd6",
			InboundTags: "vless-no-inbound-flow",
			Flow:        "none",
			Enabled:     true,
		}

		node := protocol.InboundToNodeConfig(inbound, userNoneFlow, "", 0)
		if flow := node.GetParam("flow", ""); flow != "" {
			t.Errorf("expected empty flow when user sets flow none, got: %s", flow)
		}

		link := protocol.BuildShareLink(inbound, userNoneFlow, "", 0)
		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("failed to parse share link: %v", err)
		}
		if qFlow := u.Query().Get("flow"); qFlow != "" {
			t.Errorf("expected share link flow query param to be empty, got: %s", qFlow)
		}
	})
}

func TestInboundsToNodeConfigs_UserIsolation(t *testing.T) {
	subRoutes := []domain.SubRoute{
		{
			ID:           "route-public",
			Name:         "线路A-全员开放",
			RouteID:      1,
			OutboundTag:  "direct",
			Enabled:      true,
			AllowedUsers: nil, // 全员开放
		},
		{
			ID:           "route-vip",
			Name:         "线路B-VIP专享",
			RouteID:      2,
			OutboundTag:  "out-vip",
			Enabled:      true,
			AllowedUsers: []string{"vip@test.com"},
		},
	}
	subRoutesBytes, _ := json.Marshal(subRoutes)

	inbound := domain.Inbound{
		Tag:            "vless-mixed",
		ExternalHost:   "198.51.100.1",
		Protocol:       "vless",
		StreamSettings: `{"network":"tcp","security":"none"}`,
		SubRoutesJson:  string(subRoutesBytes),
		Enabled:        true,
	}

	normalUser := &domain.User{
		Email:       "normal@test.com",
		UUID:        "11111111-1111-1111-1111-111111111111",
		InboundTags: "vless-mixed",
		Enabled:     true,
	}

	vipUser := &domain.User{
		Email:       "vip@test.com",
		UUID:        "22222222-2222-2222-2222-222222222222",
		InboundTags: "vless-mixed",
		Enabled:     true,
	}

	// 1. 普通用户只能看到线路 A
	nodesNormal := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, normalUser, "198.51.100.1", 443, "")
	if len(nodesNormal) != 1 {
		t.Fatalf("expected 1 node for normal user, got %d", len(nodesNormal))
	}
	if nodesNormal[0].Name != "线路A-全员开放" {
		t.Errorf("expected node name '线路A-全员开放', got '%s'", nodesNormal[0].Name)
	}

	// 2. VIP 用户能看到线路 A 与 线路 B
	nodesVIP := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, vipUser, "198.51.100.1", 443, "")
	if len(nodesVIP) != 2 {
		t.Fatalf("expected 2 nodes for vip user, got %d", len(nodesVIP))
	}

	// 3. 所有线路均受限且用户未命中的场景 -> 产出 0 个节点
	exclusiveRoutes := []domain.SubRoute{
		{
			ID:           "route-admin",
			Name:         "线路-管理专享",
			RouteID:      3,
			OutboundTag:  "out-admin",
			Enabled:      true,
			AllowedUsers: []string{"admin@test.com"},
		},
	}
	exclusiveBytes, _ := json.Marshal(exclusiveRoutes)
	exclusiveInbound := domain.Inbound{
		Tag:            "vless-exclusive",
		ExternalHost:   "198.51.100.1",
		Protocol:       "vless",
		StreamSettings: `{"network":"tcp","security":"none"}`,
		SubRoutesJson:  string(exclusiveBytes),
		Enabled:        true,
	}
	normalUser.InboundTags = "vless-exclusive"
	nodesNone := protocol.InboundsToNodeConfigs([]domain.Inbound{exclusiveInbound}, normalUser, "198.51.100.1", 443, "")
	if len(nodesNone) != 0 {
		t.Fatalf("expected 0 nodes when all subroutes unauthorized, got %d", len(nodesNone))
	}
}

func TestInboundsToNodeConfigs_NonVlessIgnoresSubRoutes(t *testing.T) {
	subRoutes := []domain.SubRoute{
		{ID: "1", Name: "Line 1", RouteID: 1, OutboundTag: "direct", Enabled: true},
		{ID: "2", Name: "Line 2", RouteID: 2, OutboundTag: "warp", Enabled: true},
	}
	subRoutesBytes, _ := json.Marshal(subRoutes)

	inbound := domain.Inbound{
		Tag:           "trojan-in",
		Protocol:      "trojan",
		Port:          443,
		SubRoutesJson: string(subRoutesBytes),
		Enabled:       true,
	}
	user := &domain.User{
		Email:       "user@example.com",
		UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
		InboundTags: "trojan-in",
		Enabled:     true,
	}

	nodes := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, user, "127.0.0.1", 443, "")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node for non-vless inbound despite subroutes, got %d", len(nodes))
	}
}

func TestInboundsToNodeConfigs_UnnamedSubRouteFallback(t *testing.T) {
	subRoutes := []domain.SubRoute{
		{ID: "1", Name: "", RouteID: 5, OutboundTag: "direct", Enabled: true},
	}
	subRoutesBytes, _ := json.Marshal(subRoutes)

	inbound := domain.Inbound{
		Tag:           "vless-in",
		Remark:        "香港主节点",
		Protocol:      "vless",
		Port:          443,
		SubRoutesJson: string(subRoutesBytes),
		Enabled:       true,
	}
	user := &domain.User{
		Email:       "user@example.com",
		UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
		InboundTags: "vless-in",
		Enabled:     true,
	}

	nodes := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, user, "127.0.0.1", 443, "")
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	expectedRemark := "香港主节点 - 线路 #5"
	if nodes[0].Name != expectedRemark {
		t.Fatalf("expected remark %q, got %q", expectedRemark, nodes[0].Name)
	}
}

func TestInboundsToNodeConfigs_RouteFilter(t *testing.T) {
	subRoutes := []domain.SubRoute{
		{ID: "1", Name: "线路1", RouteID: 1, OutboundTag: "direct", Enabled: true},
		{ID: "2", Name: "线路2", RouteID: 2, OutboundTag: "warp", Enabled: true},
	}
	subRoutesBytes, _ := json.Marshal(subRoutes)

	inbound := domain.Inbound{
		Tag:           "vless-in",
		Protocol:      "vless",
		Port:          443,
		SubRoutesJson: string(subRoutesBytes),
		Enabled:       true,
	}
	user := &domain.User{
		Email:       "user@example.com",
		UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
		InboundTags: "vless-in",
		Enabled:     true,
	}

	// Filter for route 2 only
	nodes := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, user, "127.0.0.1", 443, "", 2)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node matching route 2, got %d", len(nodes))
	}
	if nodes[0].Name != "线路2" {
		t.Fatalf("expected '线路2', got %q", nodes[0].Name)
	}

	// Filter for non-existent route 99
	nodesNone := protocol.InboundsToNodeConfigs([]domain.Inbound{inbound}, user, "127.0.0.1", 443, "", 99)
	if len(nodesNone) != 0 {
		t.Fatalf("expected 0 nodes matching route 99, got %d", len(nodesNone))
	}
}

func TestInboundToNodeConfig_HttpUpgradeAndKCP(t *testing.T) {
	user := &domain.User{
		UUID:  "11111111-2222-3333-4444-555555555555",
		Email: "test@example.com",
	}

	t.Run("httpupgrade transport extraction", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-httpupgrade-in",
			Protocol:       "vless",
			Port:           8443,
			Listen:         "127.0.0.1",
			Remark:         "VLESS-HTTPUpgrade",
			StreamSettings: `{"network":"httpupgrade","security":"none","httpupgradeSettings":{"path":"/up","host":"up.test.com"}}`,
		}

		node := protocol.InboundToNodeConfig(inbound, user, "", 0)
		if node == nil {
			t.Fatalf("expected non-nil node")
		}
		if node.GetParam("type") != "httpupgrade" {
			t.Errorf("expected type httpupgrade, got %s", node.GetParam("type"))
		}
		if node.GetParam("path") != "/up" {
			t.Errorf("expected path /up, got %s", node.GetParam("path"))
		}
		if node.GetParam("host") != "up.test.com" {
			t.Errorf("expected host up.test.com, got %s", node.GetParam("host"))
		}
	})

	t.Run("mkcp transport extraction", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-mkcp-in",
			Protocol:       "vless",
			Port:           8443,
			Listen:         "127.0.0.1",
			Remark:         "VLESS-MKCP",
			StreamSettings: `{"network":"mkcp","security":"none","kcpSettings":{"header":{"type":"wechat-video"},"seed":"kcp-seed-123"}}`,
		}

		node := protocol.InboundToNodeConfig(inbound, user, "", 0)
		if node == nil {
			t.Fatalf("expected non-nil node")
		}
		if node.GetParam("type") != "mkcp" {
			t.Errorf("expected type mkcp, got %s", node.GetParam("type"))
		}
		if node.GetParam("headerType") != "wechat-video" {
			t.Errorf("expected headerType wechat-video, got %s", node.GetParam("headerType"))
		}
		if node.GetParam("seed") != "kcp-seed-123" {
			t.Errorf("expected seed kcp-seed-123, got %s", node.GetParam("seed"))
		}
	})
}

func TestFlowResolutionAndNodeNamingTemplate(t *testing.T) {
	tcpRealityInbound := &domain.Inbound{
		Tag:            "vless-reality-in",
		Protocol:       "vless",
		Port:           443,
		Listen:         "127.0.0.1",
		Remark:         "Tokyo Node",
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"publicKey":"test-pbk","serverName":"example.com"}}`,
		SettingsJSON:   `{"flow":"xtls-rprx-vision"}`,
	}

	wsInbound := &domain.Inbound{
		Tag:            "vless-ws-in",
		Protocol:       "vless",
		Port:           443,
		Listen:         "127.0.0.1",
		Remark:         "WS Node",
		StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"ws.example.com"}}`,
	}

	t.Run("user Flow none explicitly disables flow on TCP Reality node", func(t *testing.T) {
		user := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "noflow@example.com",
			Flow:  "none",
		}
		node := protocol.InboundToNodeConfig(tcpRealityInbound, user, "", 0)
		if node.GetParam("flow") != "" {
			t.Errorf("expected empty flow for user.Flow=none, got %q", node.GetParam("flow"))
		}
	})

	t.Run("user Flow xtls-rprx-vision applied on compatible node", func(t *testing.T) {
		user := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "vision@example.com",
			Flow:  "xtls-rprx-vision",
		}
		node := protocol.InboundToNodeConfig(tcpRealityInbound, user, "", 0)
		if node.GetParam("flow") != "xtls-rprx-vision" {
			t.Errorf("expected xtls-rprx-vision flow, got %q", node.GetParam("flow"))
		}
	})

	t.Run("user Flow empty follows node default xtls-rprx-vision", func(t *testing.T) {
		user := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "defaultflow@example.com",
			Flow:  "",
		}
		node := protocol.InboundToNodeConfig(tcpRealityInbound, user, "", 0)
		if node.GetParam("flow") != "xtls-rprx-vision" {
			t.Errorf("expected xtls-rprx-vision flow, got %q", node.GetParam("flow"))
		}
	})

	t.Run("user Flow on incompatible transport (WS) is stripped", func(t *testing.T) {
		user := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "wsuser@example.com",
			Flow:  "xtls-rprx-vision",
		}
		node := protocol.InboundToNodeConfig(wsInbound, user, "", 0)
		if node.GetParam("flow") != "" {
			t.Errorf("expected empty flow on WS transport, got %q", node.GetParam("flow"))
		}
	})

	t.Run("RenderNodeName template placeholders", func(t *testing.T) {
		user := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "tmpl@example.com",
		}
		node := protocol.InboundToNodeConfig(tcpRealityInbound, user, "", 0)

		tmpl := "{remark} [{protocol}] #{index} (: {port})"
		rendered := protocol.RenderNodeName(tmpl, node, 1)
		expected := "Tokyo Node [VLESS] #1 (: 443)"
		if rendered != expected {
			t.Errorf("expected %q, got %q", expected, rendered)
		}
	})
}

func TestNodeConverter_DokodemoDoorSkipped(t *testing.T) {
	dokoInbound := &domain.Inbound{
		Tag:      "doko-in",
		Protocol: "dokodemo-door",
		Port:     53,
		Enabled:  true,
	}
	user := &domain.User{
		Email:       "test@example.com",
		UUID:        "11111111-2222-3333-4444-555555555555",
		InboundTags: "doko-in",
		Enabled:     true,
	}

	node := protocol.InboundToNodeConfig(dokoInbound, user, "", 0)
	if node != nil {
		t.Errorf("expected InboundToNodeConfig for dokodemo-door to be nil, got %+v", node)
	}

	nodes := protocol.InboundsToNodeConfigs([]domain.Inbound{*dokoInbound}, user, "127.0.0.1", 443, "")
	if len(nodes) != 0 {
		t.Errorf("expected InboundsToNodeConfigs to skip dokodemo-door, got %d nodes", len(nodes))
	}
}

func TestNodeConverter_Fingerprint(t *testing.T) {
	protocol.SetDefaultFingerprint("safari")
	defer protocol.SetDefaultFingerprint("chrome")

	user := &domain.User{
		Email: "fp@example.com",
		UUID:  "11111111-2222-3333-4444-555555555555",
	}

	// 1. TLS node with explicit fingerprint "firefox"
	tlsExplicitInbound := &domain.Inbound{
		Tag:            "tls-firefox",
		Protocol:       "vless",
		Port:           443,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"example.com","fingerprint":"firefox"}}`,
		Enabled:        true,
	}
	node1 := protocol.InboundToNodeConfig(tlsExplicitInbound, user, "", 0)
	if node1.GetParam("fp") != "firefox" {
		t.Errorf("expected fp='firefox', got %q", node1.GetParam("fp"))
	}

	// 2. TLS node without fingerprint -> falls back to defaultFingerprint ("safari")
	tlsDefaultInbound := &domain.Inbound{
		Tag:            "tls-default",
		Protocol:       "vless",
		Port:           443,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"example.com"}}`,
		Enabled:        true,
	}
	node2 := protocol.InboundToNodeConfig(tlsDefaultInbound, user, "", 0)
	if node2.GetParam("fp") != "safari" {
		t.Errorf("expected fp='safari', got %q", node2.GetParam("fp"))
	}

	// 3. REALITY node with explicit fingerprint "qq"
	realityExplicitInbound := &domain.Inbound{
		Tag:            "reality-qq",
		Protocol:       "vless",
		Port:           443,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.1.1.1:443","serverNames":["example.com"],"privateKey":"dGVzdA==","publicKey":"dGVzdA==","fingerprint":"qq"}}`,
		Enabled:        true,
	}
	node3 := protocol.InboundToNodeConfig(realityExplicitInbound, user, "", 0)
	if node3.GetParam("fp") != "qq" {
		t.Errorf("expected fp='qq', got %q", node3.GetParam("fp"))
	}

	// 4. REALITY node without fingerprint -> falls back to defaultFingerprint ("safari")
	realityDefaultInbound := &domain.Inbound{
		Tag:            "reality-default",
		Protocol:       "vless",
		Port:           443,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.1.1.1:443","serverNames":["example.com"],"privateKey":"dGVzdA==","publicKey":"dGVzdA=="}}`,
		Enabled:        true,
	}
	node4 := protocol.InboundToNodeConfig(realityDefaultInbound, user, "", 0)
	if node4.GetParam("fp") != "safari" {
		t.Errorf("expected fp='safari', got %q", node4.GetParam("fp"))
	}
}

func TestNodeConverter_TrojanFixedPassword(t *testing.T) {
	user := &domain.User{
		Email: "trojan-user@example.com",
		UUID:  "11111111-2222-3333-4444-555555555555",
	}

	inboundWithFixedPwd := &domain.Inbound{
		Tag:            "trojan-fixed",
		Protocol:       "trojan",
		Port:           443,
		SettingsJSON:   `{"password":"my-secret-fixed-password"}`,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"trojan.example.com"}}`,
		Enabled:        true,
	}

	node := protocol.InboundToNodeConfig(inboundWithFixedPwd, user, "", 0)
	if node == nil {
		t.Fatalf("expected node to be generated")
	}
	if node.UUID != "my-secret-fixed-password" {
		t.Errorf("expected node.UUID to be fixed password 'my-secret-fixed-password', got %q", node.UUID)
	}

	// Inbound without fixed password should use user.UUID
	inboundDefault := &domain.Inbound{
		Tag:            "trojan-default",
		Protocol:       "trojan",
		Port:           443,
		SettingsJSON:   `{}`,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"trojan.example.com"}}`,
		Enabled:        true,
	}
	nodeDef := protocol.InboundToNodeConfig(inboundDefault, user, "", 0)
	if nodeDef == nil {
		t.Fatalf("expected node to be generated")
	}
	if nodeDef.UUID != user.UUID {
		t.Errorf("expected node.UUID to be user.UUID %q, got %q", user.UUID, nodeDef.UUID)
	}
}

func TestNodeConverter_StatelessDefaultFingerprint(t *testing.T) {
	protocol.SetDefaultFingerprint("random")
	defer protocol.SetDefaultFingerprint("chrome")

	user := &domain.User{
		Email:       "stateless@example.com",
		UUID:        "11111111-2222-3333-4444-555555555555",
		InboundTags: "tls-no-fp,reality-explicit",
		Enabled:     true,
	}

	tlsNoFP := domain.Inbound{
		ID:             1,
		Tag:            "tls-no-fp",
		Protocol:       "vless",
		Port:           443,
		ExternalHost:   "tls.example.com",
		Enabled:        true,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"tls.example.com"}}`,
	}
	realityExplicit := domain.Inbound{
		ID:             2,
		Tag:            "reality-explicit",
		Protocol:       "vless",
		Port:           443,
		ExternalHost:   "reality.example.com",
		Enabled:        true,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"127.0.0.1:443","serverNames":["reality.example.com"],"privateKey":"dGVzdA==","publicKey":"dGVzdA==","fingerprint":"chrome"}}`,
	}

	// 1. Explicit defaultFP ("safari") applies when inbound has no fp, while explicit inbound fp ("chrome") wins.
	nodes := protocol.InboundsToNodeConfigsWithDefaultFP([]domain.Inbound{tlsNoFP, realityExplicit}, user, "", 0, "", "safari")
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].GetParam("fp") != "safari" {
		t.Errorf("expected tls-no-fp to get fp='safari', got %q", nodes[0].GetParam("fp"))
	}
	if nodes[1].GetParam("fp") != "chrome" {
		t.Errorf("expected reality-explicit to preserve explicit fp='chrome', got %q", nodes[1].GetParam("fp"))
	}

	// 2. Empty defaultFP falls back to "chrome".
	nodeFallback := protocol.InboundToNodeConfigWithDefaultFP(&tlsNoFP, user, "", 0, "")
	if nodeFallback.GetParam("fp") != "chrome" {
		t.Errorf("expected empty defaultFP to fall back to 'chrome', got %q", nodeFallback.GetParam("fp"))
	}

	// 3. BuildShareLinkWithDefaultFP uses passed defaultFP without altering global state.
	link := protocol.BuildShareLinkWithDefaultFP(&tlsNoFP, user, "", 0, "ios")
	if !strings.Contains(link, "fp=ios") {
		t.Errorf("expected BuildShareLinkWithDefaultFP link to contain fp=ios, got %s", link)
	}
	if got := protocol.GetDefaultFingerprint(); got != "random" {
		t.Errorf("expected global defaultFingerprint to remain 'random', got %q", got)
	}
}



