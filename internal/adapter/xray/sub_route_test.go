package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"panel/internal/domain"
)

func TestBuildShareLinksForInbound_SubRoutes(t *testing.T) {
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

	streamSettings := map[string]interface{}{
		"network":  "xhttp",
		"security": "reality",
		"realitySettings": map[string]interface{}{
			"serverNames": []string{"www.titech.ac.jp"},
			"publicKey":   "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww",
			"shortIds":    []string{"0123456789abcdef"},
		},
		"xhttpSettings": map[string]interface{}{
			"path": "/mbqyfa4grswh5ntz",
			"mode": "auto",
		},
	}
	streamBytes, _ := json.Marshal(streamSettings)

	inbound := &domain.Inbound{
		Tag:            "vless-reality",
		Port:           4434,
		ExternalPort:   443,
		ExternalHost:   "198.51.100.1",
		Protocol:       "vless",
		StreamSettings: string(streamBytes),
		SubRoutesJson:  string(subRoutesBytes),
		Enabled:        true,
	}

	user := &domain.User{
		Email: "user@example.com",
		UUID:  "7117295b-4362-4260-a133-b969344dfcd5",
	}

	links := BuildShareLinksForInbound(inbound, user, "198.51.100.1", 443)

	if len(links) != 2 {
		t.Fatalf("expected 2 links for enabled subroutes, got %d", len(links))
	}

	// 验证第 1 个链接 (日本直连)
	link1 := links[0]
	if !strings.Contains(link1, "7117295b-4362-0001-a133-b969344dfcd5") {
		t.Errorf("link1 does not contain route 1 UUID: %s", link1)
	}
	if !strings.Contains(link1, "@198.51.100.1:443") {
		t.Errorf("link1 does not connect to port 443: %s", link1)
	}
	if !strings.Contains(link1, "%2Fmbqyfa4grswh5ntz") {
		t.Errorf("link1 does not contain shared path: %s", link1)
	}

	// 验证第 2 个链接 (美国中转)
	link2 := links[1]
	if !strings.Contains(link2, "7117295b-4362-0002-a133-b969344dfcd5") {
		t.Errorf("link2 does not contain route 2 UUID: %s", link2)
	}
	if !strings.Contains(link2, "@198.51.100.1:443") {
		t.Errorf("link2 does not connect to port 443: %s", link2)
	}
	if !strings.Contains(link2, "FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww") {
		t.Errorf("link2 does not contain shared reality key: %s", link2)
	}
}
