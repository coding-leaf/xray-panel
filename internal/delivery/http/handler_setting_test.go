package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"panel/internal/domain"

	"github.com/gin-gonic/gin"
)

type mockSettingRepo struct {
	settings map[string]string
}

func newMockSettingRepo() *mockSettingRepo {
	return &mockSettingRepo{settings: make(map[string]string)}
}

func (m *mockSettingRepo) Get(ctx context.Context, key string) (string, error) {
	if val, ok := m.settings[key]; ok {
		return val, nil
	}
	return "", domain.ErrNotFound
}

func (m *mockSettingRepo) Set(ctx context.Context, key, value string) error {
	m.settings[key] = value
	return nil
}

func (m *mockSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range m.settings {
		cp[k] = v
	}
	return cp, nil
}

func TestSettingHandler_JWTSecretProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("GetSettings must mask/omit jwt_secret (SEC-02)", func(t *testing.T) {
		repo := newMockSettingRepo()
		_ = repo.Set(context.Background(), "jwt_secret", "super-sensitive-jwt-key")
		_ = repo.Set(context.Background(), "sub_domain", "sub.example.com")

		handler := NewSettingHandler(repo, nil, nil, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/settings", nil)

		handler.GetSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if _, exists := resp["jwt_secret"]; exists {
			t.Fatalf("jwt_secret leaked in GetSettings response: %v", resp)
		}
		if resp["sub_domain"] != "sub.example.com" {
			t.Fatalf("expected sub_domain to be preserved, got %s", resp["sub_domain"])
		}
	})

	t.Run("SaveSettings must forbid overwriting jwt_secret (SEC-02)", func(t *testing.T) {
		repo := newMockSettingRepo()
		_ = repo.Set(context.Background(), "jwt_secret", "original-secure-key")

		handler := NewSettingHandler(repo, nil, nil, nil)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		body := map[string]interface{}{
			"jwt_secret": "hacked-attacker-key",
			"sub_domain": "new.example.com",
		}
		b, _ := json.Marshal(body)
		c.Request = httptest.NewRequest("POST", "/api/settings", bytes.NewReader(b))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.SaveSettings(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		// Verify jwt_secret remained unchanged in the repository
		currentKey, _ := repo.Get(context.Background(), "jwt_secret")
		if currentKey != "original-secure-key" {
			t.Fatalf("jwt_secret was maliciously overwritten! Got: %s", currentKey)
		}
		// Verify sub_domain was updated
		subDomain, _ := repo.Get(context.Background(), "sub_domain")
		if subDomain != "new.example.com" {
			t.Fatalf("expected sub_domain to be updated, got: %s", subDomain)
		}
	})
}
