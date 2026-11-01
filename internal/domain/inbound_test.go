package domain_test

import (
	"testing"

	"panel/internal/domain"
)

func TestSubRoute_CanAccess(t *testing.T) {
	t.Run("empty or nil AllowedUsers allows everyone", func(t *testing.T) {
		srNil := domain.SubRoute{
			ID:           "sr-1",
			AllowedUsers: nil,
		}
		if !srNil.CanAccess("user1@example.com") {
			t.Errorf("expected true for nil AllowedUsers")
		}

		srEmpty := domain.SubRoute{
			ID:           "sr-2",
			AllowedUsers: []string{},
		}
		if !srEmpty.CanAccess("user2@example.com") {
			t.Errorf("expected true for empty AllowedUsers")
		}
	})

	t.Run("whitelisted users match exactly", func(t *testing.T) {
		sr := domain.SubRoute{
			ID:           "sr-3",
			AllowedUsers: []string{"alice@test.com", "bob@test.com"},
		}

		if !sr.CanAccess("alice@test.com") {
			t.Errorf("expected true for alice@test.com")
		}
		if !sr.CanAccess("bob@test.com") {
			t.Errorf("expected true for bob@test.com")
		}
		if sr.CanAccess("charlie@test.com") {
			t.Errorf("expected false for charlie@test.com")
		}
		if sr.CanAccess("") {
			t.Errorf("expected false for empty email")
		}
	})
}

func TestSupportsDynamicUser(t *testing.T) {
	supported := []string{"vless", "VLESS", "vmess", "VMess", "trojan", "Trojan", "shadowsocks", "shadowsocks-2022", "ss", "ss-2022", "ss2022"}
	for _, p := range supported {
		if !domain.SupportsDynamicUser(p) {
			t.Errorf("expected SupportsDynamicUser(%q) to be true", p)
		}
	}

	unsupported := []string{"socks", "http", "dokodemo-door", "freedom", "blackhole", "wireguard", ""}
	for _, p := range unsupported {
		if domain.SupportsDynamicUser(p) {
			t.Errorf("expected SupportsDynamicUser(%q) to be false", p)
		}
	}
}
