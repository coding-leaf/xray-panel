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
