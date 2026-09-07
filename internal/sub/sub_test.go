package sub_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"panel/internal/protocol"
	"panel/internal/sub"

	"gopkg.in/yaml.v2"
)

func createSampleNodes() []*protocol.NodeConfig {
	return []*protocol.NodeConfig{
		{
			Name:     "🇯🇵 日本 Reality",
			Protocol: "vless",
			Address:  "198.51.100.1",
			Port:     443,
			UUID:     "7117295b-4362-0001-a133-b969344dfcd5",
			Params: map[string]string{
				"type":     "tcp",
				"security": "reality",
				"pbk":      "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww",
				"sni":      "www.titech.ac.jp",
				"sid":      "0123456789abcdef",
				"spx":      "/spider",
				"fp":       "chrome",
			},
		},
		{
			Name:     "🇺🇸 美国 Trojan WS",
			Protocol: "trojan",
			Address:  "us.example.com",
			Port:     443,
			UUID:     "trojan-password",
			Params: map[string]string{
				"type":     "ws",
				"security": "tls",
				"sni":      "us.example.com",
				"path":     "/trojan-ws",
				"host":     "us.example.com",
				"alpn":     "h2,http/1.1",
			},
		},
		{
			Name:     "🇩🇪 德国 Hysteria2",
			Protocol: "hysteria2",
			Address:  "de.example.com",
			Port:     8443,
			UUID:     "hy2-auth-key",
			Params: map[string]string{
				"sni":           "de.example.com",
				"insecure":      "1",
				"obfs":          "salamander",
				"obfs-password": "secret-obfs-pwd",
			},
		},
		{
			Name:     "🇸🇬 新加坡 VMess",
			Protocol: "vmess",
			Address:  "sg.example.com",
			Port:     443,
			UUID:     "vmess-uuid-2222",
			Params: map[string]string{
				"type":     "ws",
				"security": "tls",
				"sni":      "sg.example.com",
				"path":     "/vmess",
				"host":     "sg.example.com",
			},
		},
		{
			Name:     "🇭🇰 香港 SS",
			Protocol: "shadowsocks",
			Address:  "hk.example.com",
			Port:     8388,
			UUID:     "ss-secret",
			Params: map[string]string{
				"method": "aes-128-gcm",
			},
		},
	}
}

func TestExportSubscription_Base64AndRaw(t *testing.T) {
	nodes := createSampleNodes()

	// 1. Raw 导出测试
	raw, err := sub.ExportSubscription(nodes, "raw")
	if err != nil {
		t.Fatalf("ExportRaw failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != len(nodes) {
		t.Fatalf("expected %d links, got %d", len(nodes), len(lines))
	}
	if !strings.HasPrefix(lines[0], "vless://") {
		t.Errorf("line 0 should be vless, got %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "trojan://") {
		t.Errorf("line 1 should be trojan, got %s", lines[1])
	}
	if !strings.HasPrefix(lines[2], "hysteria2://") {
		t.Errorf("line 2 should be hysteria2, got %s", lines[2])
	}

	// 2. Base64 导出测试
	b64, err := sub.ExportSubscription(nodes, "base64")
	if err != nil {
		t.Fatalf("ExportBase64 failed: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("failed to decode base64 output: %v", err)
	}
	if string(decoded) != raw {
		t.Fatalf("decoded base64 string does not match raw text")
	}

	// 3. 空节点列表测试
	emptyRaw, err := sub.ExportSubscription([]*protocol.NodeConfig{}, "raw")
	if err != nil || emptyRaw != "" {
		t.Fatalf("expected empty raw string, got %q, err %v", emptyRaw, err)
	}
	emptyB64, err := sub.ExportSubscription([]*protocol.NodeConfig{}, "base64")
	if err != nil || emptyB64 != "" {
		t.Fatalf("expected empty base64 string, got %q, err %v", emptyB64, err)
	}
}

func TestExportSubscription_Clash(t *testing.T) {
	nodes := createSampleNodes()

	clashYAML, err := sub.ExportSubscription(nodes, "clash")
	if err != nil {
		t.Fatalf("ExportClash failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(clashYAML), &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated Clash YAML: %v\nYAML content:\n%s", err, clashYAML)
	}

	proxiesRaw, ok := parsed["proxies"].([]interface{})
	if !ok || len(proxiesRaw) != len(nodes) {
		t.Fatalf("expected %d proxies, got %+v", len(nodes), proxiesRaw)
	}

	// 验证第一个节点（VLESS Reality）的 Clash 结构
	node0 := proxiesRaw[0].(map[interface{}]interface{})
	if node0["type"] != "vless" || node0["server"] != "198.51.100.1" || node0["port"] != 443 {
		t.Errorf("unexpected node0 config: %+v", node0)
	}
	realityOpts, ok := node0["reality-opts"].(map[interface{}]interface{})
	if !ok || realityOpts["public-key"] != "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww" {
		t.Errorf("unexpected reality-opts: %+v", realityOpts)
	}

	// 验证 ProxyGroups 存在 PROXIES 和 AUTO
	groupsRaw, ok := parsed["proxy-groups"].([]interface{})
	if !ok || len(groupsRaw) != 2 {
		t.Fatalf("expected 2 proxy-groups, got %+v", groupsRaw)
	}

	// 验证别名：clash-meta, mihomo
	for _, alias := range []string{"clash-meta", "mihomo"} {
		aliasOut, err := sub.ExportSubscription(nodes, alias)
		if err != nil || len(aliasOut) == 0 {
			t.Errorf("expected success with alias %s, got err %v", alias, err)
		}
	}

	// 空列表测试
	emptyClash, err := sub.ExportSubscription([]*protocol.NodeConfig{}, "clash")
	if err != nil {
		t.Fatalf("expected success on empty clash export, got: %v", err)
	}
	if !strings.Contains(emptyClash, "DIRECT") {
		t.Errorf("expected DIRECT fallback in empty clash export, got: %s", emptyClash)
	}
}

func TestExportSubscription_SingBox(t *testing.T) {
	nodes := createSampleNodes()

	singBoxJSON, err := sub.ExportSubscription(nodes, "sing-box")
	if err != nil {
		t.Fatalf("ExportSingBox failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(singBoxJSON), &parsed); err != nil {
		t.Fatalf("failed to unmarshal generated Sing-box JSON: %v\nJSON content:\n%s", err, singBoxJSON)
	}

	outboundsRaw, ok := parsed["outbounds"].([]interface{})
	if !ok {
		t.Fatalf("expected outbounds array, got %+v", parsed["outbounds"])
	}

	// 包含: selector(1) + urltest(1) + proxies(5) + direct(1) + block(1) + dns(1) = 10
	expectedOutbounds := len(nodes) + 5
	if len(outboundsRaw) != expectedOutbounds {
		t.Fatalf("expected %d outbounds, got %d", expectedOutbounds, len(outboundsRaw))
	}

	// 验证别名：singbox
	aliasOut, err := sub.ExportSubscription(nodes, "singbox")
	if err != nil || len(aliasOut) == 0 {
		t.Errorf("expected success with alias singbox, got err %v", err)
	}

	// 空列表测试
	emptySingBox, err := sub.ExportSubscription([]*protocol.NodeConfig{}, "sing-box")
	if err != nil {
		t.Fatalf("expected success on empty singbox export, got: %v", err)
	}
	if !strings.Contains(emptySingBox, "direct") {
		t.Errorf("expected direct fallback in empty singbox export, got: %s", emptySingBox)
	}
}

func TestExportSubscription_ErrorHandling(t *testing.T) {
	// 不支持的格式
	_, err := sub.ExportSubscription(nil, "quantumult_x")
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}

	// 节点中存在非法配置
	badNodes := []*protocol.NodeConfig{
		{
			Name:     "Invalid Node",
			Protocol: "vless",
			Address:  "", // 缺失地址
			Port:     443,
			UUID:     "uuid",
		},
	}
	_, err = sub.ExportSubscription(badNodes, "base64")
	if err == nil {
		t.Fatal("expected error when formatting invalid node, got nil")
	}

	_, err = sub.ExportSubscription(badNodes, "clash")
	if err == nil {
		t.Fatal("expected error when converting invalid node to clash, got nil")
	}

	_, err = sub.ExportSubscription(badNodes, "sing-box")
	if err == nil {
		t.Fatal("expected error when converting invalid node to sing-box, got nil")
	}
}

// mockSubProtocol 测试零修改导出新协议
type mockSubProtocol struct{}

func (m *mockSubProtocol) Protocol() string {
	return "mockproto"
}

func (m *mockSubProtocol) FormatLink(node *protocol.NodeConfig) (string, error) {
	return "mockproto://" + node.UUID + "@" + node.FormattedHost() + ":8080#" + node.Name, nil
}

func (m *mockSubProtocol) ToClash(node *protocol.NodeConfig) (map[string]interface{}, error) {
	return map[string]interface{}{
		"name":   node.Name,
		"type":   "mockproto",
		"server": node.RawHost(),
		"port":   node.Port,
		"token":  node.UUID,
	}, nil
}

func (m *mockSubProtocol) ToSingBox(node *protocol.NodeConfig) (map[string]interface{}, error) {
	return map[string]interface{}{
		"tag":         node.Name,
		"type":        "mockproto",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"token":       node.UUID,
	}, nil
}

func TestExportSubscription_DynamicNewProtocolZeroChange(t *testing.T) {
	protocol.Register(&mockSubProtocol{})
	defer protocol.DefaultRegistry().Unregister("mockproto")

	node := &protocol.NodeConfig{
		Name:     "Mock-Node",
		Protocol: "mockproto",
		Address:  "mock.domain.com",
		Port:     8080,
		UUID:     "token-abc-123",
	}

	nodes := []*protocol.NodeConfig{node}

	// 1. Raw / Base64
	raw, err := sub.ExportSubscription(nodes, "raw")
	if err != nil {
		t.Fatalf("export raw failed: %v", err)
	}
	if !strings.HasPrefix(raw, "mockproto://token-abc-123@mock.domain.com:8080#Mock-Node") {
		t.Errorf("unexpected raw output: %s", raw)
	}

	// 2. Clash (无需修改 clash.go)
	clashYAML, err := sub.ExportSubscription(nodes, "clash")
	if err != nil {
		t.Fatalf("export clash with new protocol failed: %v", err)
	}
	if !strings.Contains(clashYAML, "type: mockproto") || !strings.Contains(clashYAML, "token: token-abc-123") {
		t.Errorf("clash yaml missing mockproto fields: %s", clashYAML)
	}

	// 3. Sing-box (无需修改 singbox.go)
	singBoxJSON, err := sub.ExportSubscription(nodes, "sing-box")
	if err != nil {
		t.Fatalf("export sing-box with new protocol failed: %v", err)
	}
	if !strings.Contains(singBoxJSON, `"type": "mockproto"`) || !strings.Contains(singBoxJSON, `"token": "token-abc-123"`) {
		t.Errorf("sing-box json missing mockproto fields: %s", singBoxJSON)
	}
}

func TestExportSubscription_DynamicFormatRegistry(t *testing.T) {
	// 动态注册新格式
	sub.RegisterExporter("mock-format", func(nodes []*protocol.NodeConfig) (string, error) {
		return fmt.Sprintf("MOCK_EXPORTED_COUNT_%d", len(nodes)), nil
	})
	defer sub.UnregisterExporter("mock-format")

	res, err := sub.ExportSubscription([]*protocol.NodeConfig{{Protocol: "vless"}}, "mock-format")
	if err != nil {
		t.Fatalf("custom format export failed: %v", err)
	}
	if res != "MOCK_EXPORTED_COUNT_1" {
		t.Errorf("expected MOCK_EXPORTED_COUNT_1, got %s", res)
	}

	// 注销后应提示不支持
	sub.UnregisterExporter("mock-format")
	_, err = sub.ExportSubscription(nil, "mock-format")
	if err == nil {
		t.Fatal("expected error after unregistering exporter, got nil")
	}
}

func TestExportSubscription_IPv6InClashAndSingBox(t *testing.T) {
	nodeIPv6 := &protocol.NodeConfig{
		Name:     "IPv6 Node",
		Protocol: "vless",
		Address:  "[2606:4700:4700::1111]", // 包含括号输入
		Port:     443,
		UUID:     "uuid-v6",
		Params: map[string]string{
			"type":     "tcp",
			"security": "reality",
			"pbk":      "test-pbk",
		},
	}

	nodes := []*protocol.NodeConfig{nodeIPv6}

	// 1. Clash 中 server 必须为纯净 IPv6 无括号
	clashYAML, err := sub.ExportSubscription(nodes, "clash")
	if err != nil {
		t.Fatalf("clash export failed: %v", err)
	}
	if !strings.Contains(clashYAML, "server: 2606:4700:4700::1111") {
		t.Errorf("clash yaml should have raw IPv6 server without brackets: %s", clashYAML)
	}

	// 2. Sing-box 中 server 必须为纯净 IPv6 无括号
	singBoxJSON, err := sub.ExportSubscription(nodes, "sing-box")
	if err != nil {
		t.Fatalf("sing-box export failed: %v", err)
	}
	if !strings.Contains(singBoxJSON, `"server": "2606:4700:4700::1111"`) {
		t.Errorf("sing-box json should have raw IPv6 server without brackets: %s", singBoxJSON)
	}

	// 3. Raw 分享链接中 Host 必须有括号安全包裹
	raw, err := sub.ExportSubscription(nodes, "raw")
	if err != nil {
		t.Fatalf("raw export failed: %v", err)
	}
	if !strings.Contains(raw, "@[2606:4700:4700::1111]:443") {
		t.Errorf("raw share link should have bracketed IPv6: %s", raw)
	}
}
