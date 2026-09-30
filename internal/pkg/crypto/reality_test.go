package crypto

import (
	"testing"
)

func TestGenerateRealityKeyPair(t *testing.T) {
	kp, err := GenerateRealityKeyPair()
	if err != nil {
		t.Fatalf("GenerateRealityKeyPair failed: %v", err)
	}

	if kp.PrivateKey == "" {
		t.Errorf("expected non-empty private key")
	}
	if kp.PublicKey == "" {
		t.Errorf("expected non-empty public key")
	}
	if len(kp.ShortID) != 16 {
		t.Errorf("expected 16-hex shortId, got %d (%s)", len(kp.ShortID), kp.ShortID)
	}

	derivedPub := DerivePublicKeyFromPrivate(kp.PrivateKey)
	if derivedPub != kp.PublicKey {
		t.Errorf("derived public key %s does not match generated %s", derivedPub, kp.PublicKey)
	}
}

func TestDerivePublicKeyFromPrivate_Invalid(t *testing.T) {
	if got := DerivePublicKeyFromPrivate(""); got != "" {
		t.Errorf("expected empty string for empty input, got %s", got)
	}
	if got := DerivePublicKeyFromPrivate("invalid-base64!!"); got != "" {
		t.Errorf("expected empty string for invalid base64, got %s", got)
	}
}

func TestApplyVlessRouteToUUID(t *testing.T) {
	rawUUID := "7117295b-4362-46ef-a133-b969344dfcd5"

	// 0 routeId: should keep original
	if got := ApplyVlessRouteToUUID(rawUUID, 0); got != rawUUID {
		t.Errorf("expected original UUID for routeID 0, got %s", got)
	}

	// routeId 1: third chunk should become 0001
	expected := "7117295b-4362-0001-a133-b969344dfcd5"
	if got := ApplyVlessRouteToUUID(rawUUID, 1); got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}

	// routeId 65535: third chunk should become ffff
	expectedMax := "7117295b-4362-ffff-a133-b969344dfcd5"
	if got := ApplyVlessRouteToUUID(rawUUID, 65535); got != expectedMax {
		t.Errorf("expected %s, got %s", expectedMax, got)
	}

	// invalid uuid format: should return original
	if got := ApplyVlessRouteToUUID("not-a-uuid", 1); got != "not-a-uuid" {
		t.Errorf("expected raw string for invalid uuid, got %s", got)
	}
}
