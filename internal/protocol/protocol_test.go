package protocol_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"

	"panel/internal/protocol"
)

func TestNodeConfig_Methods(t *testing.T) {
	node := protocol.NewNodeConfig("Node 1", "vless", "127.0.0.1", 443, "uuid-1234")

	if node.Name != "Node 1" || node.Protocol != "vless" || node.Address != "127.0.0.1" || node.Port != 443 || node.UUID != "uuid-1234" {
		t.Fatalf("unexpected node values: %+v", node)
	}

	// GetParam with fallback
	if val := node.GetParam("non_existent", "default_val"); val != "default_val" {
		t.Errorf("expected default_val, got %s", val)
	}
	if val := node.GetParam("non_existent"); val != "" {
		t.Errorf("expected empty string, got %s", val)
	}

	// SetParam and GetParam
	node.SetParam("sni", "example.com")
	if val := node.GetParam("sni"); val != "example.com" {
		t.Errorf("expected example.com, got %s", val)
	}

	// Nil params safety
	var nilNode *protocol.NodeConfig
	if val := nilNode.GetParam("key", "fallback"); val != "fallback" {
		t.Errorf("expected fallback on nil node, got %s", val)
	}
	nilNode.SetParam("key", "val") // should not panic
	if clone := nilNode.Clone(); clone != nil {
		t.Errorf("expected nil clone for nil node, got %+v", clone)
	}

	// Deep clone test
	cloned := node.Clone()
	if cloned == node {
		t.Fatal("cloned pointer should not equal original pointer")
	}
	cloned.SetParam("sni", "different.com")
	if node.GetParam("sni") != "example.com" {
		t.Errorf("original node param mutated after clone modification: %s", node.GetParam("sni"))
	}

	// IPv6 formatting
	nodeIPv6 := protocol.NewNodeConfig("IPv6 Node", "vless", "2001:db8::1", 443, "uuid-1234")
	if host := nodeIPv6.FormattedHost(); host != "[2001:db8::1]" {
		t.Errorf("expected [2001:db8::1], got %s", host)
	}
	if addrPort := nodeIPv6.FormattedAddressPort(); addrPort != "[2001:db8::1]:443" {
		t.Errorf("expected [2001:db8::1]:443, got %s", addrPort)
	}
}

func TestRegistry_DynamicRegistrationAndResolution(t *testing.T) {
	reg := protocol.NewRegistry()

	// 1. 未注册协议
	_, ok := reg.Get("custom")
	if ok {
		t.Fatal("expected custom protocol to not be registered")
	}
	_, err := reg.FormatLink(&protocol.NodeConfig{Protocol: "custom"})
	if !errors.Is(err, protocol.ErrUnsupportedProtocol) {
		t.Fatalf("expected ErrUnsupportedProtocol, got %v", err)
	}

	// 2. 自定义注册
	mockFormatter := &mockCustomFormatter{proto: "custom"}
	reg.Register(mockFormatter)

	retrieved, ok := reg.Get("CUSTOM") // 大小写不敏感
	if !ok || retrieved.Protocol() != "custom" {
		t.Fatalf("failed to retrieve registered formatter with case-insensitivity")
	}

	link, err := reg.FormatLink(&protocol.NodeConfig{Protocol: "Custom", Address: "1.2.3.4", Port: 80})
	if err != nil {
		t.Fatalf("FormatLink failed: %v", err)
	}
	if link != "custom://1.2.3.4:80" {
		t.Fatalf("unexpected formatted link: %s", link)
	}

	// 3. 协议列表
	protos := reg.ListSupportedProtocols()
	if len(protos) != 1 || protos[0] != "custom" {
		t.Fatalf("expected ['custom'], got %+v", protos)
	}

	// 4. 注销
	reg.Unregister("custom")
	if _, ok := reg.Get("custom"); ok {
		t.Fatal("expected protocol to be unregistered")
	}

	// 5. 边界校验
	reg.Register(nil) // 安全忽略
	_, err = reg.FormatLink(nil)
	if !errors.Is(err, protocol.ErrNilNode) {
		t.Fatalf("expected ErrNilNode, got %v", err)
	}
}

func TestRegistry_Concurrency(t *testing.T) {
	reg := protocol.NewRegistry()
	reg.Register(&mockCustomFormatter{proto: "worker"})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			reg.Register(&mockCustomFormatter{proto: "concurrent"})
		}()
		go func() {
			defer wg.Done()
			_, _ = reg.Get("worker")
		}()
		go func() {
			defer wg.Done()
			_, _ = reg.FormatLink(&protocol.NodeConfig{Protocol: "worker", Address: "10.0.0.1", Port: 8080})
		}()
	}
	wg.Wait()
}

type mockCustomFormatter struct {
	proto string
}

func (m *mockCustomFormatter) Protocol() string {
	return m.proto
}

func (m *mockCustomFormatter) FormatLink(node *protocol.NodeConfig) (string, error) {
	return m.proto + "://" + node.Address + ":80", nil
}

func TestVlessFormatter_FullCombinations(t *testing.T) {
	// 1. Reality + TCP + Flow
	nodeReality := &protocol.NodeConfig{
		Name:     "🇯🇵 日本 Reality 节点",
		Protocol: "vless",
		Address:  "198.51.100.1",
		Port:     443,
		UUID:     "7117295b-4362-0001-a133-b969344dfcd5",
		Params: map[string]string{
			"type":     "tcp",
			"security": "reality",
			"pbk":      "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww",
			"sni":      "www.apple.com",
			"sid":      "0123456789abcdef",
			"spx":      "/spider",
			"fp":       "chrome",
		},
	}

	link, err := protocol.FormatLink(nodeReality)
	if err != nil {
		t.Fatalf("failed to format vless reality link: %v", err)
	}

	if !strings.HasPrefix(link, "vless://7117295b-4362-0001-a133-b969344dfcd5@198.51.100.1:443?") {
		t.Fatalf("unexpected link prefix: %s", link)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	q := u.Query()
	if q.Get("security") != "reality" || q.Get("type") != "tcp" {
		t.Errorf("unexpected security/type: %s/%s", q.Get("security"), q.Get("type"))
	}
	if q.Get("pbk") != "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww" {
		t.Errorf("unexpected pbk: %s", q.Get("pbk"))
	}
	if q.Get("sni") != "www.apple.com" || q.Get("sid") != "0123456789abcdef" || q.Get("spx") != "/spider" {
		t.Errorf("unexpected sni/sid/spx: %s/%s/%s", q.Get("sni"), q.Get("sid"), q.Get("spx"))
	}
	if q.Get("flow") != "xtls-rprx-vision" {
		t.Errorf("expected default flow xtls-rprx-vision, got: %s", q.Get("flow"))
	}
	if !strings.HasSuffix(link, "#"+url.QueryEscape(nodeReality.Name)) {
		t.Errorf("link does not end with escaped name: %s", link)
	}

	// 2. XHTTP + Reality
	nodeXhttp := &protocol.NodeConfig{
		Name:     "XHTTP-Node",
		Protocol: "vless",
		Address:  "example.com",
		Port:     8443,
		UUID:     "uuid-xhttp",
		Params: map[string]string{
			"type":     "xhttp",
			"security": "reality",
			"pbk":      "test-pbk",
			"sni":      "gateway.com",
			"path":     "/my-xhttp-path",
			"mode":     "auto",
			"host":     "origin.com",
		},
	}
	linkXhttp, err := protocol.FormatLink(nodeXhttp)
	if err != nil {
		t.Fatalf("failed to format xhttp link: %v", err)
	}
	uXhttp, _ := url.Parse(linkXhttp)
	qXhttp := uXhttp.Query()
	if qXhttp.Get("type") != "xhttp" || qXhttp.Get("path") != "/my-xhttp-path" || qXhttp.Get("mode") != "auto" || qXhttp.Get("host") != "origin.com" {
		t.Errorf("unexpected xhttp params: %+v", qXhttp)
	}

	// 3. WS + CDN 回源组合
	nodeCDN := &protocol.NodeConfig{
		Name:     "WS CDN Node",
		Protocol: "vless",
		Address:  "104.16.1.1", // Cloudflare CDN 优选 IP
		Port:     443,
		UUID:     "uuid-cdn",
		Params: map[string]string{
			"type":     "ws",
			"security": "tls",
			"sni":      "cdn.example.com",
			"host":     "cdn.example.com", // CDN 回源 Host
			"path":     "/websocket-path",
		},
	}
	linkCDN, err := protocol.FormatLink(nodeCDN)
	if err != nil {
		t.Fatalf("failed to format cdn link: %v", err)
	}
	uCDN, _ := url.Parse(linkCDN)
	qCDN := uCDN.Query()
	if qCDN.Get("type") != "ws" || qCDN.Get("host") != "cdn.example.com" || qCDN.Get("sni") != "cdn.example.com" || qCDN.Get("path") != "/websocket-path" {
		t.Errorf("unexpected cdn params: %+v", qCDN)
	}

	// 4. IPv6 地址安全转义
	nodeIPv6 := &protocol.NodeConfig{
		Name:     "IPv6",
		Protocol: "vless",
		Address:  "2606:4700:4700::1111",
		Port:     443,
		UUID:     "uuid-ipv6",
	}
	linkIPv6, err := protocol.FormatLink(nodeIPv6)
	if err != nil {
		t.Fatalf("failed to format ipv6 link: %v", err)
	}
	if !strings.Contains(linkIPv6, "@[2606:4700:4700::1111]:443") {
		t.Errorf("expected wrapped ipv6 in link, got %s", linkIPv6)
	}

	// 5. 参数校验异常路径
	invalidNodes := []*protocol.NodeConfig{
		nil,
		{Protocol: "vless", Port: 443, UUID: "u"},
		{Protocol: "vless", Address: "1.1.1.1", Port: 0, UUID: "u"},
		{Protocol: "vless", Address: "1.1.1.1", Port: 70000, UUID: "u"},
		{Protocol: "vless", Address: "1.1.1.1", Port: 443, UUID: ""},
	}
	for i, inv := range invalidNodes {
		if _, err := protocol.FormatLink(inv); err == nil {
			t.Errorf("case %d: expected error for invalid node, got nil", i)
		}
	}
}

func TestTrojanFormatter(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "Trojan WS Node",
		Protocol: "trojan",
		Address:  "trojan.example.com",
		Port:     443,
		UUID:     "secret-password",
		Params: map[string]string{
			"type":     "ws",
			"security": "tls",
			"sni":      "trojan.example.com",
			"alpn":     "h2,http/1.1",
			"path":     "/trojan-ws",
			"host":     "trojan.example.com",
		},
	}

	link, err := protocol.FormatLink(node)
	if err != nil {
		t.Fatalf("failed to format trojan link: %v", err)
	}

	if !strings.HasPrefix(link, "trojan://secret-password@trojan.example.com:443?") {
		t.Fatalf("unexpected trojan link: %s", link)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	q := u.Query()
	if q.Get("sni") != "trojan.example.com" || q.Get("alpn") != "h2,http/1.1" || q.Get("path") != "/trojan-ws" || q.Get("host") != "trojan.example.com" {
		t.Errorf("unexpected trojan query params: %+v", q)
	}
}

func TestHysteria2Formatter(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "Hysteria2 Node",
		Protocol: "hysteria2",
		Address:  "hy2.example.com",
		Port:     443,
		UUID:     "hy2-secret-auth",
		Params: map[string]string{
			"sni":           "hy2.example.com",
			"insecure":      "1",
			"obfs":          "salamander",
			"obfs-password": "obfs-password-123",
			"mport":         "20000-30000",
		},
	}

	link, err := protocol.FormatLink(node)
	if err != nil {
		t.Fatalf("failed to format hysteria2 link: %v", err)
	}

	if !strings.HasPrefix(link, "hysteria2://hy2-secret-auth@hy2.example.com:443?") {
		t.Fatalf("unexpected hysteria2 link: %s", link)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	q := u.Query()
	if q.Get("sni") != "hy2.example.com" || q.Get("insecure") != "1" || q.Get("obfs") != "salamander" || q.Get("obfs-password") != "obfs-password-123" || q.Get("mport") != "20000-30000" {
		t.Errorf("unexpected hysteria2 query params: %+v", q)
	}

	// 验证 hy2 别名注册
	nodeAlias := node.Clone()
	nodeAlias.Protocol = "hy2"
	linkAlias, err := protocol.FormatLink(nodeAlias)
	if err != nil {
		t.Fatalf("failed to format with hy2 alias: %v", err)
	}
	if !strings.HasPrefix(linkAlias, "hysteria2://") {
		t.Errorf("expected hysteria2:// prefix for hy2 alias, got %s", linkAlias)
	}
}

func TestVmessFormatter(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "VMess Node",
		Protocol: "vmess",
		Address:  "vmess.example.com",
		Port:     443,
		UUID:     "vmess-uuid-1111",
		Params: map[string]string{
			"type":     "ws",
			"security": "tls",
			"sni":      "vmess.example.com",
			"path":     "/vmess-path",
			"host":     "vmess.example.com",
		},
	}

	link, err := protocol.FormatLink(node)
	if err != nil {
		t.Fatalf("failed to format vmess link: %v", err)
	}

	if !strings.HasPrefix(link, "vmess://") {
		t.Fatalf("expected vmess:// prefix, got %s", link)
	}

	b64Part := strings.TrimPrefix(link, "vmess://")
	rawJSON, err := base64.StdEncoding.DecodeString(b64Part)
	if err != nil {
		t.Fatalf("failed to base64 decode vmess link: %v", err)
	}

	var vmessObj map[string]interface{}
	if err := json.Unmarshal(rawJSON, &vmessObj); err != nil {
		t.Fatalf("failed to unmarshal vmess json: %v", err)
	}

	if vmessObj["id"] != "vmess-uuid-1111" || vmessObj["sni"] != "vmess.example.com" || vmessObj["path"] != "/vmess-path" || vmessObj["host"] != "vmess.example.com" {
		t.Errorf("unexpected vmess json payload: %+v", vmessObj)
	}
}

func TestShadowsocksFormatter(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "SS Node",
		Protocol: "shadowsocks",
		Address:  "ss.example.com",
		Port:     8388,
		UUID:     "mypassword",
		Params: map[string]string{
			"method": "aes-256-gcm",
		},
	}

	link, err := protocol.FormatLink(node)
	if err != nil {
		t.Fatalf("failed to format ss link: %v", err)
	}

	if !strings.HasPrefix(link, "ss://") {
		t.Fatalf("expected ss:// prefix, got %s", link)
	}

	// 验证 ss 别名
	nodeAlias := node.Clone()
	nodeAlias.Protocol = "ss"
	linkAlias, err := protocol.FormatLink(nodeAlias)
	if err != nil {
		t.Fatalf("failed to format with ss alias: %v", err)
	}
	if link != linkAlias {
		t.Errorf("expected identical link with ss alias, got %s vs %s", link, linkAlias)
	}
}

// mockWireguardFormatter 测试零代码修改扩展新协议
type mockWireguardFormatter struct{}

func (w *mockWireguardFormatter) Protocol() string {
	return "wireguard"
}

func (w *mockWireguardFormatter) FormatLink(node *protocol.NodeConfig) (string, error) {
	return fmt.Sprintf("wireguard://%s@%s:%d#%s", node.UUID, node.FormattedHost(), node.Port, url.QueryEscape(node.Name)), nil
}

func (w *mockWireguardFormatter) ToClash(node *protocol.NodeConfig) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":       node.Name,
		"type":       "wireguard",
		"server":     node.RawHost(),
		"port":       node.Port,
		"privatekey": node.UUID,
	}, nil
}

func (w *mockWireguardFormatter) ToSingBox(node *protocol.NodeConfig) (map[string]interface{}, error) {
	return map[string]interface{}{
		"tag":         node.Name,
		"type":        "wireguard",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"private_key": node.UUID,
	}, nil
}

func TestDynamicNewProtocolRegistration_ZeroCoupling(t *testing.T) {
	reg := protocol.NewRegistry()
	reg.Register(&mockWireguardFormatter{})

	node := &protocol.NodeConfig{
		Name:     "WG-Node",
		Protocol: "wireguard",
		Address:  "wg.example.com",
		Port:     51820,
		UUID:     "privkey-1234",
	}

	// 1. FormatLink
	link, err := reg.FormatLink(node)
	if err != nil {
		t.Fatalf("FormatLink failed on dynamically registered protocol: %v", err)
	}
	if link != "wireguard://privkey-1234@wg.example.com:51820#WG-Node" {
		t.Errorf("unexpected link: %s", link)
	}

	// 2. ToClash
	clashProxy, err := reg.ToClash(node)
	if err != nil {
		t.Fatalf("ToClash failed on dynamically registered protocol: %v", err)
	}
	if clashProxy["type"] != "wireguard" || clashProxy["privatekey"] != "privkey-1234" {
		t.Errorf("unexpected clash proxy: %+v", clashProxy)
	}

	// 3. ToSingBox
	singBoxOutbound, err := reg.ToSingBox(node)
	if err != nil {
		t.Fatalf("ToSingBox failed on dynamically registered protocol: %v", err)
	}
	if singBoxOutbound["type"] != "wireguard" || singBoxOutbound["private_key"] != "privkey-1234" {
		t.Errorf("unexpected singbox outbound: %+v", singBoxOutbound)
	}
}

func TestDynamicCustomParamsPreservedInLink(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "VLESS Custom",
		Protocol: "vless",
		Address:  "custom.example.com",
		Port:     443,
		UUID:     "uuid-custom",
		Params: map[string]string{
			"type":                   "ws",
			"security":               "tls",
			"obfs-host":              "cdn-gateway.com",
			"early_data_header_name": "Sec-WebSocket-Protocol",
			"headerType":             "http",
		},
	}

	link, err := protocol.FormatLink(node)
	if err != nil {
		t.Fatalf("FormatLink failed: %v", err)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}
	q := u.Query()
	if q.Get("obfs-host") != "cdn-gateway.com" {
		t.Errorf("expected obfs-host=cdn-gateway.com, got %s", q.Get("obfs-host"))
	}
	if q.Get("early_data_header_name") != "Sec-WebSocket-Protocol" {
		t.Errorf("expected early_data_header_name=Sec-WebSocket-Protocol, got %s", q.Get("early_data_header_name"))
	}
	if q.Get("headerType") != "http" {
		t.Errorf("expected headerType=http, got %s", q.Get("headerType"))
	}
}

func TestIPv6_RawHostAndFormattedHost(t *testing.T) {
	testCases := []struct {
		input         string
		expectedRaw   string
		expectedHost  string
		expectedAddrP string
	}{
		{
			input:         "2001:db8::1",
			expectedRaw:   "2001:db8::1",
			expectedHost:  "[2001:db8::1]",
			expectedAddrP: "[2001:db8::1]:443",
		},
		{
			input:         "[2001:db8::1]",
			expectedRaw:   "2001:db8::1",
			expectedHost:  "[2001:db8::1]",
			expectedAddrP: "[2001:db8::1]:443",
		},
		{
			input:         "[2001:db8::1]:8443",
			expectedRaw:   "2001:db8::1",
			expectedHost:  "[2001:db8::1]",
			expectedAddrP: "[2001:db8::1]:443",
		},
		{
			input:         "example.com",
			expectedRaw:   "example.com",
			expectedHost:  "example.com",
			expectedAddrP: "example.com:443",
		},
		{
			input:         "1.2.3.4",
			expectedRaw:   "1.2.3.4",
			expectedHost:  "1.2.3.4",
			expectedAddrP: "1.2.3.4:443",
		},
	}

	for _, tc := range testCases {
		n := protocol.NewNodeConfig("node", "vless", tc.input, 443, "uuid")
		if raw := n.RawHost(); raw != tc.expectedRaw {
			t.Errorf("input %s: expected RawHost %s, got %s", tc.input, tc.expectedRaw, raw)
		}
		if host := n.FormattedHost(); host != tc.expectedHost {
			t.Errorf("input %s: expected FormattedHost %s, got %s", tc.input, tc.expectedHost, host)
		}
		if ap := n.FormattedAddressPort(); ap != tc.expectedAddrP {
			t.Errorf("input %s: expected FormattedAddressPort %s, got %s", tc.input, tc.expectedAddrP, ap)
		}
	}
}

func TestFormatters_ToClashAndToSingBox(t *testing.T) {
	node := &protocol.NodeConfig{
		Name:     "Test VLESS Reality",
		Protocol: "vless",
		Address:  "2001:db8::1",
		Port:     443,
		UUID:     "uuid-vless",
		Params: map[string]string{
			"type":     "tcp",
			"security": "reality",
			"pbk":      "pubkey-test",
			"sni":      "sni.example.com",
			"sid":      "sid123",
			"fp":       "safari",
		},
	}

	// 1. VLESS Clash
	clashMap, err := protocol.ToClash(node)
	if err != nil {
		t.Fatalf("ToClash failed: %v", err)
	}
	if clashMap["server"] != "2001:db8::1" { // 无括号
		t.Errorf("expected clean server in clash, got %v", clashMap["server"])
	}
	if clashMap["client-fingerprint"] != "safari" {
		t.Errorf("expected fp safari, got %v", clashMap["client-fingerprint"])
	}

	// 2. VLESS Sing-box
	singBoxMap, err := protocol.ToSingBox(node)
	if err != nil {
		t.Fatalf("ToSingBox failed: %v", err)
	}
	if singBoxMap["server"] != "2001:db8::1" {
		t.Errorf("expected clean server in sing-box, got %v", singBoxMap["server"])
	}

	// 3. Trojan Clash & Sing-box with insecure and alpn
	nodeTrojan := &protocol.NodeConfig{
		Name:     "Trojan",
		Protocol: "trojan",
		Address:  "trojan.com",
		Port:     443,
		UUID:     "pwd",
		Params: map[string]string{
			"type":     "tcp",
			"security": "tls",
			"sni":      "trojan.com",
			"alpn":     "h2, http/1.1",
			"insecure": "1",
		},
	}
	trojanClash, err := protocol.ToClash(nodeTrojan)
	if err != nil {
		t.Fatalf("trojan clash failed: %v", err)
	}
	if trojanClash["skip-cert-verify"] != true {
		t.Errorf("expected skip-cert-verify true, got %v", trojanClash["skip-cert-verify"])
	}
	alpns := trojanClash["alpn"].([]string)
	if len(alpns) != 2 || alpns[0] != "h2" || alpns[1] != "http/1.1" {
		t.Errorf("expected trimmed alpn, got %+v", alpns)
	}

	trojanSingBox, err := protocol.ToSingBox(nodeTrojan)
	if err != nil {
		t.Fatalf("trojan sing-box failed: %v", err)
	}
	tlsObj := trojanSingBox["tls"].(map[string]interface{})
	if tlsObj["insecure"] != true {
		t.Errorf("expected insecure true in sing-box tls, got %+v", tlsObj)
	}
}
