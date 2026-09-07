package xray_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
)

func TestBuildShareLink_FullParameterParity(t *testing.T) {
	user := &domain.User{
		UUID:  "11111111-2222-3333-4444-555555555555",
		Email: "test@example.com",
	}

	t.Run("Trojan WS TLS generates sni, path, host (FUNC-08)", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "trojan-ws-in",
			Protocol:       "trojan",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "Trojan-Node",
			StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"trojan.example.com","alpn":["h2","http/1.1"]},"wsSettings":{"path":"/trojan-path","headers":{"Host":"trojan.example.com"}}}`,
		}

		link := xray.BuildShareLink(inbound, user, "", 0)
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

	t.Run("VMess WS TLS generates sni, path, host (FUNC-09)", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vmess-ws-in",
			Protocol:       "vmess",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "VMess-Node",
			StreamSettings: `{"network":"ws","security":"tls","tlsSettings":{"serverName":"vmess.example.com"},"wsSettings":{"path":"/vmess-path","headers":{"Host":"vmess.example.com"}}}`,
		}

		link := xray.BuildShareLink(inbound, user, "", 0)
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

	t.Run("VLESS REALITY adapts to singular serverName and shortId (FUNC-10)", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "reality-singular-in",
			Protocol:       "vless",
			Port:           443,
			Listen:         "127.0.0.1",
			Remark:         "Reality-Node",
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"apple.com","shortId":"aabbcc1122","publicKey":"test-pbk"}}`,
		}

		link := xray.BuildShareLink(inbound, user, "", 0)
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

	t.Run("IPv6 address preserved without truncation (cleanHost bug fix)", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-ipv6",
			Protocol:       "vless",
			Port:           443,
			ExternalHost:   "2408:8207:dead:beef::1", // 裸 IPv6
			StreamSettings: `{"network":"tcp","security":"none"}`,
		}

		link := xray.BuildShareLink(inbound, user, "", 0)
		if !strings.Contains(link, "@[2408:8207:dead:beef::1]:443") {
			t.Fatalf("expected bracketed IPv6 in link, got: %s", link)
		}

		node := xray.InboundToNodeConfig(inbound, user, "", 0)
		if node.Address != "2408:8207:dead:beef::1" {
			t.Errorf("expected raw IPv6 address preserved in NodeConfig, got: %s", node.Address)
		}
	})

	t.Run("XHTTP host header extraction", func(t *testing.T) {
		inbound := &domain.Inbound{
			Tag:            "vless-xhttp",
			Protocol:       "vless",
			Port:           443,
			ExternalHost:   "1.2.3.4",
			StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"path":"/xhttp-path","mode":"stream-up","host":"xhttp.origin.com"}}`,
		}

		node := xray.InboundToNodeConfig(inbound, user, "", 0)
		if node.GetParam("host") != "xhttp.origin.com" {
			t.Errorf("expected xhttp host to be xhttp.origin.com, got: %s", node.GetParam("host"))
		}
		if node.GetParam("mode") != "stream-up" {
			t.Errorf("expected xhttp mode to be stream-up, got: %s", node.GetParam("mode"))
		}
	})

	t.Run("VLESS XHTTP Reality with user flow does NOT emit flow", func(t *testing.T) {
		userWithFlow := &domain.User{
			UUID:  "11111111-2222-3333-4444-555555555555",
			Email: "test@example.com",
			Flow:  "xtls-rprx-vision",
		}
		inbound := &domain.Inbound{
			Tag:            "vless-xhttp-reality",
			Protocol:       "vless",
			Port:           443,
			ExternalHost:   "1.2.3.4",
			StreamSettings: `{"network":"xhttp","security":"reality","realitySettings":{"publicKey":"test-pbk","serverName":"example.com"}}`,
		}

		node := xray.InboundToNodeConfig(inbound, userWithFlow, "", 0)
		if node.GetParam("flow") != "" {
			t.Errorf("expected empty flow in NodeConfig for xhttp, got: %s", node.GetParam("flow"))
		}

		link := xray.BuildShareLink(inbound, userWithFlow, "", 0)
		u, err := url.Parse(link)
		if err != nil {
			t.Fatalf("failed to parse link: %v", err)
		}
		if u.Query().Get("flow") != "" {
			t.Errorf("expected no flow query param for xhttp reality, got: %s", u.Query().Get("flow"))
		}
	})
}
