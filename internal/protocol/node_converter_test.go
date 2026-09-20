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
