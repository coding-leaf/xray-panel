package jwt_test

import (
	"testing"
	"time"

	"panel/internal/pkg/jwt"
)

func TestJWT_GenerateAndParse(t *testing.T) {
	secret := "my-secret-key"
	username := "admin"

	token, err := jwt.GenerateToken(username, secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := jwt.ParseToken(token, secret)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}

	if claims.Username != username {
		t.Errorf("expected username %s, got %s", username, claims.Username)
	}

	// Test invalid secret
	_, err = jwt.ParseToken(token, "wrong-secret")
	if err == nil {
		t.Errorf("expected error with wrong secret, got nil")
	}

	// Test expired token
	expiredToken, err := jwt.GenerateToken(username, secret, -1*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken expired failed: %v", err)
	}
	_, err = jwt.ParseToken(expiredToken, secret)
	if err == nil {
		t.Errorf("expected error with expired token, got nil")
	}
}
