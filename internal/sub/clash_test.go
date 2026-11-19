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
