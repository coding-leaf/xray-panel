package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"panel/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type mockAdminRepo struct {
	admin *domain.AdminUser
}

func (m *mockAdminRepo) GetByUsername(ctx context.Context, username string) (*domain.AdminUser, error) {
	if m.admin != nil && m.admin.Username == username {
		return m.admin, nil
	}
	return nil, domain.ErrNotFound
}

func (m *mockAdminRepo) Update(ctx context.Context, admin *domain.AdminUser) error {
	m.admin = admin
	return nil
}

func (m *mockAdminRepo) Create(ctx context.Context, admin *domain.AdminUser) error {
	m.admin = admin
	return nil
}

func TestAuthHandler_Disable2FA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	password := "correct-password"
	pwdHash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "XrayPanelTest",
		AccountName: "admin",
	})
	if err != nil {
		t.Fatalf("failed to generate totp key: %v", err)
	}
	secret := key.Secret()

	newTestContext := func(admin *domain.AdminUser, reqBody map[string]interface{}) (*AuthHandler, *httptest.ResponseRecorder, *gin.Context) {
		repo := &mockAdminRepo{admin: admin}
		handler := NewAuthHandler(repo, "test-secret")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("username", "admin")

		bodyBytes, _ := json.Marshal(reqBody)
		c.Request = httptest.NewRequest("POST", "/api/auth/2fa/disable", bytes.NewReader(bodyBytes))
		c.Request.Header.Set("Content-Type", "application/json")
		return handler, w, c
	}

	t.Run("Empty passcode must be rejected (SEC-01)", func(t *testing.T) {
		admin := &domain.AdminUser{
			Username:     "admin",
			PasswordHash: string(pwdHash),
			TOTPEnabled:  true,
			TOTPSecret:   secret,
		}
		handler, w, c := newTestContext(admin, map[string]interface{}{
			"password": password,
			"passcode": "",
		})

		handler.Disable2FA(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 for empty passcode, got %d. Body: %s", w.Code, w.Body.String())
		}
		if !admin.TOTPEnabled {
			t.Fatalf("2FA was disabled despite empty passcode! Vulnerability SEC-01 reproduced!")
		}
	})

	t.Run("Invalid passcode must be rejected", func(t *testing.T) {
		admin := &domain.AdminUser{
			Username:     "admin",
			PasswordHash: string(pwdHash),
			TOTPEnabled:  true,
			TOTPSecret:   secret,
		}
		handler, w, c := newTestContext(admin, map[string]interface{}{
			"password": password,
			"passcode": "000000",
		})

		handler.Disable2FA(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400 for wrong passcode, got %d. Body: %s", w.Code, w.Body.String())
		}
		if !admin.TOTPEnabled {
			t.Fatalf("2FA was disabled with wrong passcode!")
		}
	})

	t.Run("Valid password and valid passcode must successfully disable 2FA", func(t *testing.T) {
		admin := &domain.AdminUser{
			Username:     "admin",
			PasswordHash: string(pwdHash),
			TOTPEnabled:  true,
			TOTPSecret:   secret,
		}
		validPasscode, err := totp.GenerateCode(secret, time.Now())
		if err != nil {
			t.Fatalf("generate code failed: %v", err)
		}

		handler, w, c := newTestContext(admin, map[string]interface{}{
			"password": password,
			"passcode": validPasscode,
		})

		handler.Disable2FA(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
		if admin.TOTPEnabled {
			t.Fatalf("expected TOTPEnabled to be false, but still true")
		}
		if admin.TOTPSecret != "" {
			t.Fatalf("expected TOTPSecret to be cleared, got %s", admin.TOTPSecret)
		}
	})
}
