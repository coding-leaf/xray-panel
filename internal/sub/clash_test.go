package sub

import (
	"strings"
	"testing"

	"panel/internal/protocol"

	"gopkg.in/yaml.v3"
)

func TestExportClashWithParams_SafeSkipAndFallback(t *testing.T) {
	t.Run("skips xhttp, splithttp, and unsupported protocol while keeping compatible nodes with AUTO first", func(t *testing.T) {
		nodes := []*protocol.NodeConfig{
			{
				Name:     "XHTTP-Node",
				Protocol: "vless",
				Address:  "127.0.0.1",
				Port:     443,
				UUID:     "11111111-1111-4111-8111-111111111111",
				Params: map[string]string{
					"type":     "xhttp",
					"security": "reality",
					"pbk":      "test-pbk",
					"sni":      "example.com",
				},
			},
			{
				Name:     "SplitHTTP-Node",
				Protocol: "vless",
				Address:  "127.0.0.1",
				Port:     8443,
				UUID:     "11111111-1111-4111-8111-111111111111",
				Params: map[string]string{
					"type":     "splithttp",
					"security": "tls",
				},
			},
			{
				Name:     "Unsupported-Proto",
				Protocol: "unknown-proto",
				Address:  "127.0.0.1",
				Port:     9000,
			},
			{
				Name:     "Compatible-Trojan",
				Protocol: "trojan",
				Address:  "127.0.0.1",
				Port:     443,
				UUID:     "secret-pass",
				Params: map[string]string{
					"type":     "tcp",
					"security": "tls",
					"sni":      "example.com",
				},
			},
		}

		out, err := ExportClash(nodes)
		if err != nil {
			t.Fatalf("ExportClash returned unexpected error: %v", err)
		}

		var cfg ClashConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("expected valid yaml, got error: %v\n%s", err, out)
		}
		if len(cfg.Proxies) != 1 || cfg.Proxies[0]["name"] != "Compatible-Trojan" {
			t.Fatalf("expected only Compatible-Trojan in proxies, got: %+v", cfg.Proxies)
		}

		proxiesGroup := cfg.ProxyGroups[0]["proxies"].([]interface{})
		if len(proxiesGroup) != 2 || proxiesGroup[0] != "AUTO" || proxiesGroup[1] != "Compatible-Trojan" {
			t.Fatalf("expected PROXIES group [AUTO, Compatible-Trojan], got: %+v", proxiesGroup)
		}
	})

	t.Run("all nodes incompatible returns valid DIRECT yaml with notice comment", func(t *testing.T) {
		nodes := []*protocol.NodeConfig{
			{
				Name:     "Only-XHTTP",
				Protocol: "vless",
				Address:  "127.0.0.1",
				Port:     443,
				UUID:     "11111111-1111-4111-8111-111111111111",
				Params: map[string]string{
					"type":     "xhttp",
					"security": "reality",
				},
			},
		}

		out, err := ExportClash(nodes)
		if err != nil {
			t.Fatalf("ExportClash returned unexpected error: %v", err)
		}
		if !strings.HasPrefix(out, "# Notice:") {
			t.Fatalf("expected top notice comment in fallback YAML, got:\n%s", out)
		}
		if !strings.Contains(out, "v2rayN") || !strings.Contains(out, "sing-box") {
			t.Fatalf("expected client recommendation in notice comment, got:\n%s", out)
		}

		var cfg ClashConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("expected valid fallback yaml, got error: %v\n%s", err, out)
		}
		if len(cfg.Proxies) != 0 {
			t.Fatalf("expected 0 proxies, got %d", len(cfg.Proxies))
		}
		proxiesGroup := cfg.ProxyGroups[0]["proxies"].([]interface{})
		if len(proxiesGroup) != 1 || proxiesGroup[0] != "DIRECT" {
			t.Fatalf("expected PROXIES fallback [DIRECT], got: %+v", proxiesGroup)
		}
	})
}

func TestExportClashWithParams_RealityMLKEMAndTemplateHygiene(t *testing.T) {
	virtualInfo1 := &protocol.NodeConfig{
		Name:     "📊 剩余流量: 90.00 GB / 100.00 GB",
		Protocol: "trojan",
		Address:  "127.0.0.1",
		Port:     10000,
		UUID:     "00000000-0000-0000-0000-000000000000",
		Params: map[string]string{
			"type":     "tcp",
			"security": "none",
		},
	}
	virtualInfo2 := &protocol.NodeConfig{
		Name:     "⏳ 到期时间: 长期有效",
		Protocol: "trojan",
		Address:  "127.0.0.1",
		Port:     10000,
		UUID:     "00000000-0000-0000-0000-000000000000",
		Params: map[string]string{
			"type":     "tcp",
			"security": "none",
		},
	}
	realRealityNode := &protocol.NodeConfig{
		Name:     "JP-REALITY",
		Protocol: "vless",
		Address:  "198.51.100.1",
		Port:     443,
		UUID:     "11111111-1111-4111-8111-111111111111",
		Params: map[string]string{
			"type":     "tcp",
			"security": "reality",
			"flow":     "xtls-rprx-vision",
			"pbk":      "test-pbk-123",
			"sni":      "www.apple.com",
			"sid":      "0123456789abcdef",
			"fp":       "chrome",
		},
	}

	t.Run("emits support-x25519mlkem768, excludes virtual info nodes from AUTO while keeping in PROXIES, and omits duplicate port", func(t *testing.T) {
		out, err := ExportClash([]*protocol.NodeConfig{virtualInfo1, virtualInfo2, realRealityNode})
		if err != nil {
			t.Fatalf("ExportClash returned unexpected error: %v", err)
		}

		if !strings.Contains(out, "support-x25519mlkem768: true") {
			t.Fatalf("expected Clash YAML to contain support-x25519mlkem768: true, got:\n%s", out)
		}

		var rawMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(out), &rawMap); err != nil {
			t.Fatalf("expected valid yaml, got error: %v", err)
		}
		if _, hasPort := rawMap["port"]; hasPort {
			t.Fatalf("expected duplicate port key to be omitted when mixed-port is used, got port=%v", rawMap["port"])
		}
		if _, hasSocksPort := rawMap["socks-port"]; hasSocksPort {
			t.Fatalf("expected socks-port key to be omitted when mixed-port is used, got socks-port=%v", rawMap["socks-port"])
		}
		if rawMap["mixed-port"] != 7890 {
			t.Fatalf("expected mixed-port: 7890, got: %v", rawMap["mixed-port"])
		}

		var cfg ClashConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("failed to unmarshal ClashConfig: %v", err)
		}
		if len(cfg.ProxyGroups) != 2 {
			t.Fatalf("expected 2 proxy groups (PROXIES and AUTO), got %d", len(cfg.ProxyGroups))
		}
		if cfg.ProxyGroups[0]["name"] != "PROXIES" || cfg.ProxyGroups[0]["type"] != "select" {
			t.Fatalf("expected group 0 to be PROXIES (select), got: %+v", cfg.ProxyGroups[0])
		}
		if cfg.ProxyGroups[1]["name"] != "AUTO" || cfg.ProxyGroups[1]["type"] != "url-test" {
			t.Fatalf("expected group 1 to be AUTO (url-test), got: %+v", cfg.ProxyGroups[1])
		}

		proxiesGroup := cfg.ProxyGroups[0]["proxies"].([]interface{})
		if len(proxiesGroup) != 4 || proxiesGroup[0] != "AUTO" || proxiesGroup[1] != virtualInfo1.Name || proxiesGroup[2] != virtualInfo2.Name || proxiesGroup[3] != realRealityNode.Name {
			t.Fatalf("expected PROXIES to include AUTO + virtual info nodes + real node, got: %+v", proxiesGroup)
		}

		autoGroup := cfg.ProxyGroups[1]["proxies"].([]interface{})
		if len(autoGroup) != 1 || autoGroup[0] != realRealityNode.Name {
			t.Fatalf("expected AUTO to include ONLY real proxy node [%s], got: %+v", realRealityNode.Name, autoGroup)
		}

		if len(cfg.Rules) != 1 || cfg.Rules[0] != "MATCH,PROXIES" {
			t.Fatalf("expected rules [MATCH,PROXIES], got: %+v", cfg.Rules)
		}
	})

	t.Run("only virtual info nodes falls back AUTO safely to DIRECT instead of empty list", func(t *testing.T) {
		out, err := ExportClash([]*protocol.NodeConfig{virtualInfo1, virtualInfo2})
		if err != nil {
			t.Fatalf("ExportClash returned unexpected error: %v", err)
		}
		if !strings.HasPrefix(out, "# Notice:") {
			t.Fatalf("expected top notice comment when no real proxies exist, got:\n%s", out)
		}

		var rawMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(out), &rawMap); err != nil {
			t.Fatalf("expected valid yaml, got error: %v\n%s", err, out)
		}
		if _, hasPort := rawMap["port"]; hasPort {
			t.Fatalf("expected port key to be omitted, got port=%v", rawMap["port"])
		}
		if _, hasSocksPort := rawMap["socks-port"]; hasSocksPort {
			t.Fatalf("expected socks-port key to be omitted, got socks-port=%v", rawMap["socks-port"])
		}
		if rawMap["mixed-port"] != 7890 {
			t.Fatalf("expected mixed-port: 7890, got: %v", rawMap["mixed-port"])
		}

		var cfg ClashConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("failed to unmarshal ClashConfig: %v\n%s", err, out)
		}
		if len(cfg.ProxyGroups) != 2 {
			t.Fatalf("expected 2 proxy groups, got %d", len(cfg.ProxyGroups))
		}
		if cfg.ProxyGroups[0]["name"] != "PROXIES" || cfg.ProxyGroups[0]["type"] != "select" {
			t.Fatalf("expected group 0 to be PROXIES (select), got: %+v", cfg.ProxyGroups[0])
		}
		if cfg.ProxyGroups[1]["name"] != "AUTO" || cfg.ProxyGroups[1]["type"] != "url-test" {
			t.Fatalf("expected group 1 to be AUTO (url-test), got: %+v", cfg.ProxyGroups[1])
		}

		autoGroup, ok := cfg.ProxyGroups[1]["proxies"].([]interface{})
		if !ok || len(autoGroup) == 0 {
			t.Fatalf("expected non-empty AUTO proxies fallback, got: %+v", cfg.ProxyGroups[1]["proxies"])
		}
		if len(autoGroup) != 1 || autoGroup[0] != "DIRECT" {
			t.Fatalf("expected AUTO fallback [DIRECT] when only virtual info nodes exist, got: %+v", autoGroup)
		}

		proxiesGroup := cfg.ProxyGroups[0]["proxies"].([]interface{})
		if len(proxiesGroup) != 3 || proxiesGroup[0] != "DIRECT" || proxiesGroup[1] != virtualInfo1.Name || proxiesGroup[2] != virtualInfo2.Name {
			t.Fatalf("expected PROXIES to retain DIRECT + virtual info nodes, got: %+v", proxiesGroup)
		}

		if len(cfg.Rules) != 1 || cfg.Rules[0] != "MATCH,PROXIES" {
			t.Fatalf("expected rules [MATCH,PROXIES], got: %+v", cfg.Rules)
		}
	})

	t.Run("empty nodes produces valid Mihomo YAML with DIRECT fallback and mixed-port only", func(t *testing.T) {
		out, err := ExportClash(nil)
		if err != nil {
			t.Fatalf("ExportClash returned unexpected error: %v", err)
		}
		if !strings.HasPrefix(out, "# Notice:") {
			t.Fatalf("expected top notice comment for empty nodes, got:\n%s", out)
		}

		var rawMap map[string]interface{}
		if err := yaml.Unmarshal([]byte(out), &rawMap); err != nil {
			t.Fatalf("expected valid yaml, got error: %v\n%s", err, out)
		}
		if _, hasPort := rawMap["port"]; hasPort {
			t.Fatalf("expected port key to be omitted, got port=%v", rawMap["port"])
		}
		if _, hasSocksPort := rawMap["socks-port"]; hasSocksPort {
			t.Fatalf("expected socks-port key to be omitted, got socks-port=%v", rawMap["socks-port"])
		}
		if rawMap["mixed-port"] != 7890 {
			t.Fatalf("expected mixed-port: 7890, got: %v", rawMap["mixed-port"])
		}

		var cfg ClashConfig
		if err := yaml.Unmarshal([]byte(out), &cfg); err != nil {
			t.Fatalf("failed to unmarshal ClashConfig: %v\n%s", err, out)
		}
		if len(cfg.Proxies) != 0 {
			t.Fatalf("expected 0 proxies, got %d", len(cfg.Proxies))
		}
		if len(cfg.ProxyGroups) != 2 {
			t.Fatalf("expected 2 proxy groups, got %d", len(cfg.ProxyGroups))
		}
		if cfg.ProxyGroups[0]["name"] != "PROXIES" || cfg.ProxyGroups[0]["type"] != "select" {
			t.Fatalf("expected group 0 to be PROXIES (select), got: %+v", cfg.ProxyGroups[0])
		}
		if cfg.ProxyGroups[1]["name"] != "AUTO" || cfg.ProxyGroups[1]["type"] != "url-test" {
			t.Fatalf("expected group 1 to be AUTO (url-test), got: %+v", cfg.ProxyGroups[1])
		}
		proxiesGroup := cfg.ProxyGroups[0]["proxies"].([]interface{})
		if len(proxiesGroup) != 1 || proxiesGroup[0] != "DIRECT" {
			t.Fatalf("expected PROXIES fallback [DIRECT], got: %+v", proxiesGroup)
		}
		autoGroup := cfg.ProxyGroups[1]["proxies"].([]interface{})
		if len(autoGroup) != 1 || autoGroup[0] != "DIRECT" {
			t.Fatalf("expected AUTO fallback [DIRECT], got: %+v", autoGroup)
		}
		if len(cfg.Rules) != 1 || cfg.Rules[0] != "MATCH,PROXIES" {
			t.Fatalf("expected rules [MATCH,PROXIES], got: %+v", cfg.Rules)
		}
	})
}
