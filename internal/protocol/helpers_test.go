package protocol_test

import (
	"errors"
	"testing"

	"panel/internal/protocol"
)

func TestValidateBaseNode(t *testing.T) {
	if err := protocol.ValidateBaseNode(nil); !errors.Is(err, protocol.ErrNilNode) {
		t.Errorf("expected ErrNilNode, got %v", err)
	}

	node := &protocol.NodeConfig{}
	if err := protocol.ValidateBaseNode(node); !errors.Is(err, protocol.ErrMissingAddress) {
		t.Errorf("expected ErrMissingAddress, got %v", err)
	}

	node.Address = "1.2.3.4"
	if err := protocol.ValidateBaseNode(node); !errors.Is(err, protocol.ErrInvalidPort) {
		t.Errorf("expected ErrInvalidPort, got %v", err)
	}

	node.Port = 443
	if err := protocol.ValidateBaseNode(node); !errors.Is(err, protocol.ErrMissingUUID) {
		t.Errorf("expected ErrMissingUUID, got %v", err)
	}

	node.UUID = "uuid-ok"
	if err := protocol.ValidateBaseNode(node); err != nil {
		t.Errorf("expected nil error for valid node, got %v", err)
	}
}

func TestAttachClashTLS_And_AttachSingBoxTLS(t *testing.T) {
	node := protocol.NewNodeConfig("test", "vless", "1.1.1.1", 443, "uuid")
	node.SetParam("security", "reality")
	node.SetParam("sni", "example.com")
	node.SetParam("alpn", "h2,http/1.1")
	node.SetParam("allowInsecure", "1")
	node.SetParam("fp", "chrome")
	node.SetParam("pbk", "test-public-key")
	node.SetParam("sid", "123456")
	node.SetParam("spx", "/test")

	// 1. Clash
	clashProxy := make(map[string]interface{})
	protocol.AttachClashTLS(clashProxy, node)

	if clashProxy["tls"] != true {
		t.Errorf("clashProxy[tls] should be true")
	}
	if clashProxy["servername"] != "example.com" {
		t.Errorf("clashProxy[servername] = %v", clashProxy["servername"])
	}
	if clashProxy["skip-cert-verify"] != true {
		t.Errorf("clashProxy[skip-cert-verify] should be true")
	}
	if clashProxy["client-fingerprint"] != "chrome" {
		t.Errorf("clashProxy[client-fingerprint] = %v", clashProxy["client-fingerprint"])
	}
	realityOpts, ok := clashProxy["reality-opts"].(map[string]interface{})
	if !ok || realityOpts["public-key"] != "test-public-key" || realityOpts["short-id"] != "123456" {
		t.Errorf("invalid clash reality-opts: %+v", realityOpts)
	}

	// 2. SingBox
	singBoxOutbound := make(map[string]interface{})
	protocol.AttachSingBoxTLS(singBoxOutbound, node)

	tlsMap, ok := singBoxOutbound["tls"].(map[string]interface{})
	if !ok || tlsMap["enabled"] != true {
		t.Fatalf("invalid singbox tlsMap: %+v", tlsMap)
	}
	if tlsMap["server_name"] != "example.com" {
		t.Errorf("tlsMap[server_name] = %v", tlsMap["server_name"])
	}
	if tlsMap["insecure"] != true {
		t.Errorf("tlsMap[insecure] should be true")
	}
	realityMap, ok := tlsMap["reality"].(map[string]interface{})
	if !ok || realityMap["public_key"] != "test-public-key" || realityMap["short_id"] != "123456" {
		t.Errorf("invalid singbox realityMap: %+v", realityMap)
	}
}

func TestBuildNodeQueryParams(t *testing.T) {
	node := protocol.NewNodeConfig("test", "vless", "1.1.1.1", 443, "uuid")
	node.SetParam("type", "tcp")
	node.SetParam("security", "tls")
	node.SetParam("custom_field_1", "val1")
	node.SetParam("custom_field_2", "val2")
	node.SetParam("ignored_custom", "val3")

	q := protocol.BuildNodeQueryParams(node, "ignored_custom")

	if q.Get("type") != "" {
		t.Errorf("reserved type should not be in dynamic query")
	}
	if q.Get("security") != "" {
		t.Errorf("reserved security should not be in dynamic query")
	}
	if q.Get("custom_field_1") != "val1" {
		t.Errorf("custom_field_1 = %v", q.Get("custom_field_1"))
	}
	if q.Get("custom_field_2") != "val2" {
		t.Errorf("custom_field_2 = %v", q.Get("custom_field_2"))
	}
	if q.Get("ignored_custom") != "" {
		t.Errorf("ignored_custom should be filtered by extraReserved")
	}
}
