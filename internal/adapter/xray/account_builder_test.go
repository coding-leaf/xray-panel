package xray

import (
	"errors"
	"testing"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"
)

func TestAccountRegistry_BuildAccountMessage(t *testing.T) {
	testUUID := "7117295b-4362-0000-a133-b969344dfcd5"
	user := &domain.User{
		UUID:  testUUID,
		Email: "test@example.com",
	}

	t.Run("VLESS with TCP and REALITY", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol:       "vless",
			StreamSettings: `{"network":"tcp","security":"reality"}`,
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != "xray.proxy.vless.Account" {
			t.Errorf("expected type xray.proxy.vless.Account, got %s", msg.Type)
		}
	})

	t.Run("VMess", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol: "vmess",
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != "xray.proxy.vmess.Account" {
			t.Errorf("expected type xray.proxy.vmess.Account, got %s", msg.Type)
		}
	})

	t.Run("Trojan", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol: "trojan",
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != "xray.proxy.trojan.Account" {
			t.Errorf("expected type xray.proxy.trojan.Account, got %s", msg.Type)
		}
	})

	t.Run("Shadowsocks chacha20", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"chacha20-poly1305"}`,
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != "xray.proxy.shadowsocks.Account" {
			t.Errorf("expected type xray.proxy.shadowsocks.Account, got %s", msg.Type)
		}
	})

	t.Run("Shadowsocks 2022 blake3-aes-128-gcm", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"2022-blake3-aes-128-gcm"}`,
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != proto.TypeShadowsocks2022Account {
			t.Errorf("expected type %s, got %s", proto.TypeShadowsocks2022Account, msg.Type)
		}
		raw, err := msg.GetInstance()
		if err != nil {
			t.Fatalf("failed to get instance: %v", err)
		}
		acc, ok := raw.(*proto.Shadowsocks2022Account)
		if !ok {
			t.Fatalf("expected *proto.Shadowsocks2022Account, got %T", raw)
		}
		if acc.Key != testUUID {
			t.Errorf("expected key %s, got %s", testUUID, acc.Key)
		}
	})

	t.Run("Shadowsocks 2022 protocol tag ss2022", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol:     "ss2022",
			SettingsJSON: `{"method":"2022-blake3-aes-256-gcm"}`,
		}
		msg, err := BuildAccountMessage(inb, user)
		if err != nil {
			t.Fatalf("BuildAccountMessage failed: %v", err)
		}
		if msg.Type != proto.TypeShadowsocks2022Account {
			t.Errorf("expected type %s, got %s", proto.TypeShadowsocks2022Account, msg.Type)
		}
	})

	t.Run("Unsupported protocol", func(t *testing.T) {
		inb := &domain.Inbound{
			Protocol: "unknown-proto",
		}
		_, err := BuildAccountMessage(inb, user)
		if err == nil {
			t.Errorf("expected error for unsupported protocol, got nil")
		}
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})
}

// 确保动态注册新协议策略的能力
type mockCustomAccountBuilder struct{}

func (m *mockCustomAccountBuilder) Protocol() string {
	return "custom-mock"
}

func (m *mockCustomAccountBuilder) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return &proto.TypedMessage{
		Type: "custom.mock.Account",
	}, nil
}

func TestAccountRegistry_DynamicRegistration(t *testing.T) {
	reg := NewAccountRegistry()
	reg.Register(&mockCustomAccountBuilder{})

	user := &domain.User{UUID: "123"}
	inb := &domain.Inbound{Protocol: "custom-mock"}

	msg, err := reg.BuildAccountMessage(inb, user)
	if err != nil {
		t.Fatalf("custom protocol build failed: %v", err)
	}
	if msg.Type != "custom.mock.Account" {
		t.Errorf("expected custom.mock.Account, got %s", msg.Type)
	}
}

func TestProtocolRegistry_DirectAndResolve(t *testing.T) {
	reg := NewProtocolRegistry()

	// 1. Verify standard resolution
	adVless, ok := reg.ResolveAdapter(&domain.Inbound{Protocol: "vless"})
	if !ok || adVless.Protocol() != "vless" {
		t.Fatalf("expected vless adapter, got %v", adVless)
	}

	// 2. Verify SS AEAD resolution
	adSS, ok := reg.ResolveAdapter(&domain.Inbound{
		Protocol:     "shadowsocks",
		SettingsJSON: `{"method":"aes-128-gcm"}`,
	})
	if !ok || adSS.Protocol() != "shadowsocks" {
		t.Fatalf("expected shadowsocks adapter, got %v", adSS)
	}

	// 3. Verify SS2022 auto-resolution via method 2022-blake3-*
	adSS2022, ok := reg.ResolveAdapter(&domain.Inbound{
		Protocol:     "shadowsocks",
		SettingsJSON: `{"method":"2022-blake3-chacha20-poly1305"}`,
	})
	if !ok || adSS2022.Protocol() != "shadowsocks-2022" {
		t.Fatalf("expected shadowsocks-2022 adapter, got %v", adSS2022)
	}
}

func TestShadowsocks2022Adapter_CompileClients(t *testing.T) {
	adapter := &Shadowsocks2022Adapter{}
	users := []domain.User{
		{Email: "user@test.com", UUID: "7117295b-4362-0000-a133-b969344dfcd5", Enabled: true, InboundTags: "ss-in"},
	}

	t.Run("valid aes-128-gcm", func(t *testing.T) {
		inb := &domain.Inbound{
			Tag:          "ss-in",
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"2022-blake3-aes-128-gcm"}`,
		}
		clients, err := adapter.CompileClients(inb, users, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %d", len(clients))
		}
	})

	t.Run("valid aes-256-gcm", func(t *testing.T) {
		inb := &domain.Inbound{
			Tag:          "ss-in",
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"2022-blake3-aes-256-gcm"}`,
		}
		clients, err := adapter.CompileClients(inb, users, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clients) != 1 {
			t.Fatalf("expected 1 client, got %d", len(clients))
		}
	})

	t.Run("invalid chacha20 in 2022 multi-user", func(t *testing.T) {
		inb := &domain.Inbound{
			Tag:          "ss-in",
			Protocol:     "shadowsocks",
			SettingsJSON: `{"method":"2022-blake3-chacha20-poly1305"}`,
		}
		_, err := adapter.CompileClients(inb, users, nil)
		if err == nil {
			t.Fatalf("expected error for chacha20 in ss2022 multi-user, got nil")
		}
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})
}

func TestTrojanAdapter_CompileClients_FixedPassword(t *testing.T) {
	adapter := &TrojanAdapter{}
	users := []domain.User{
		{
			Email:       "u1@example.com",
			UUID:        "11111111-2222-3333-4444-555555555555",
			InboundTags: "trojan-in",
			Enabled:     true,
		},
	}

	t.Run("with active users and fixed password", func(t *testing.T) {
		inb := &domain.Inbound{
			Tag:          "trojan-in",
			Protocol:     "trojan",
			SettingsJSON: `{"password":"fixed-secret-pwd"}`,
		}
		clients, err := adapter.CompileClients(inb, users, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clients) != 2 {
			t.Fatalf("expected 2 clients (1 user + 1 fixed), got %d", len(clients))
		}
		hasFixed := false
		for _, c := range clients {
			if c.Password == "fixed-secret-pwd" {
				hasFixed = true
			}
		}
		if !hasFixed {
			t.Errorf("expected client with fixed password")
		}
	})

	t.Run("without active users but has fixed password", func(t *testing.T) {
		inb := &domain.Inbound{
			Tag:          "trojan-in",
			Protocol:     "trojan",
			SettingsJSON: `{"password":"fixed-secret-pwd"}`,
		}
		clients, err := adapter.CompileClients(inb, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(clients) != 1 {
			t.Fatalf("expected 1 client (fixed password), got %d", len(clients))
		}
		if clients[0].Password != "fixed-secret-pwd" {
			t.Errorf("expected password 'fixed-secret-pwd', got %s", clients[0].Password)
		}
	})
}

func TestHTTPAndDokodemoDoorAdapters(t *testing.T) {
	reg := DefaultProtocolRegistry()

	t.Run("HTTPAdapter registered and decorates", func(t *testing.T) {
		a, ok := reg.Get("http")
		if !ok || a == nil {
			t.Fatalf("expected HTTP adapter to be registered")
		}
		settings := make(map[string]interface{})
		a.DecorateSettings(&domain.Inbound{Protocol: "http"}, settings)
		if settings["allowTransparent"] != false {
			t.Errorf("expected allowTransparent=false, got %v", settings["allowTransparent"])
		}
		if settings["timeout"] != 300 {
			t.Errorf("expected timeout=300, got %v", settings["timeout"])
		}
		_, err := a.BuildAccount(&domain.Inbound{Protocol: "http"}, &domain.User{}, nil)
		if err == nil {
			t.Errorf("expected error from BuildAccount on HTTP")
		}
	})

	t.Run("DokodemoDoorAdapter registered and decorates", func(t *testing.T) {
		a, ok := reg.Get("dokodemo-door")
		if !ok || a == nil {
			t.Fatalf("expected DokodemoDoor adapter to be registered")
		}
		settings := make(map[string]interface{})
		a.DecorateSettings(&domain.Inbound{Protocol: "dokodemo-door"}, settings)
		if settings["followRedirect"] != false {
			t.Errorf("expected followRedirect=false, got %v", settings["followRedirect"])
		}
		if settings["timeout"] != 300 {
			t.Errorf("expected timeout=300, got %v", settings["timeout"])
		}
		_, err := a.BuildAccount(&domain.Inbound{Protocol: "dokodemo-door"}, &domain.User{}, nil)
		if err == nil {
			t.Errorf("expected error from BuildAccount on dokodemo-door")
		}
	})

	t.Run("SupportsDynamicUserManagement helper", func(t *testing.T) {
		if !SupportsDynamicUserManagement("vless") {
			t.Errorf("expected vless supported")
		}
		if SupportsDynamicUserManagement("socks") {
			t.Errorf("expected socks not supported")
		}
		if SupportsDynamicUserManagement("dokodemo-door") {
			t.Errorf("expected dokodemo-door not supported")
		}
	})
}
