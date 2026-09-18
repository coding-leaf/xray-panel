package totp

import (
	"strings"
	"testing"
	"time"
)

func TestTOTP_RFC6238Vectors(t *testing.T) {
	// RFC 6238 Appendix B test vector:
	// Key: "12345678901234567890" (20 bytes ASCII)
	// Base32: GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	tests := []struct {
		unixTime int64
		expected string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}

	for _, tc := range tests {
		code, err := GenerateCode(secret, time.Unix(tc.unixTime, 0))
		if err != nil {
			t.Fatalf("time %d failed: %v", tc.unixTime, err)
		}
		if code != tc.expected {
			t.Errorf("time %d: got %s, expected %s", tc.unixTime, code, tc.expected)
		}
	}
}

func TestTOTP_GenerateAndValidate(t *testing.T) {
	key, err := Generate(GenerateOpts{
		Issuer:      "XrayPanel",
		AccountName: "admin@example.com",
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if key.Secret() == "" {
		t.Fatal("empty secret")
	}

	if !strings.HasPrefix(key.URL(), "otpauth://totp/XrayPanel:admin@example.com?") {
		t.Fatalf("unexpected URL prefix: %s", key.URL())
	}

	// Validate with correct code
	now := time.Now()
	code, err := GenerateCode(key.Secret(), now)
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	if !Validate(code, key.Secret()) {
		t.Fatal("Validate returned false for valid code")
	}

	// Validate within window: -29 seconds
	pastCode, err := GenerateCode(key.Secret(), now.Add(-29*time.Second))
	if err != nil {
		t.Fatalf("GenerateCode past failed: %v", err)
	}
	if !Validate(pastCode, key.Secret()) {
		t.Fatal("Validate returned false for code within past window")
	}

	// Validate outside window: -90 seconds
	farPastCode, err := GenerateCode(key.Secret(), now.Add(-90*time.Second))
	if err != nil {
		t.Fatalf("GenerateCode far past failed: %v", err)
	}
	if Validate(farPastCode, key.Secret()) {
		t.Fatal("Validate returned true for code far outside window")
	}

	// Validate wrong code
	if Validate("123456", key.Secret()) && code != "123456" {
		t.Fatal("Validate returned true for incorrect code")
	}

	// Validate invalid formats
	if Validate("", key.Secret()) {
		t.Fatal("Validate accepted empty code")
	}
	if Validate("123", key.Secret()) {
		t.Fatal("Validate accepted short code")
	}
	if Validate(code, "invalid-base32-!@#$") {
		t.Fatal("Validate accepted invalid secret")
	}
}

func TestTOTP_GenerateErrors(t *testing.T) {
	_, err := Generate(GenerateOpts{Issuer: "", AccountName: "admin"})
	if err == nil {
		t.Fatal("expected error on empty issuer")
	}

	_, err = Generate(GenerateOpts{Issuer: "Panel", AccountName: ""})
	if err == nil {
		t.Fatal("expected error on empty account name")
	}
}
