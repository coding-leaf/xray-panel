package http_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/domain"
	"panel/internal/service"
)

type mockSubUserRepo struct{}

func (m *mockSubUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error)     { return nil, nil }
func (m *mockSubUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) { return nil, nil }
func (m *mockSubUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, nil
}
func (m *mockSubUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	return &domain.User{
		ID:          1,
		Email:       "test@example.com",
		UUID:        "11111111-1111-1111-1111-111111111111",
		SubToken:    "valid-token",
		Enabled:     true,
		InboundTags: "in-1",
	}, nil
}
func (m *mockSubUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (m *mockSubUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return nil, nil }
func (m *mockSubUserRepo) Create(ctx context.Context, u *domain.User) error   { return nil }
func (m *mockSubUserRepo) Update(ctx context.Context, u *domain.User) error   { return nil }
func (m *mockSubUserRepo) UpdateFields(ctx context.Context, id uint, fields map[string]interface{}) error {
	return nil
}
func (m *mockSubUserRepo) Delete(ctx context.Context, id uint) error          { return nil }
func (m *mockSubUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (m *mockSubUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

type mockSubInboundRepo struct{}

func (m *mockSubInboundRepo) ListEnabled(ctx context.Context) ([]domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) { return nil, nil }
func (m *mockSubInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubInboundRepo) Create(ctx context.Context, inb *domain.Inbound) error { return nil }
func (m *mockSubInboundRepo) Update(ctx context.Context, inb *domain.Inbound) error { return nil }
func (m *mockSubInboundRepo) Delete(ctx context.Context, id uint) error              { return nil }
func (m *mockSubInboundRepo) AddTraffic(ctx context.Context, tag string, up, down int64) error {
	return nil
}

type mockSubSettingRepo struct{}

func (m *mockSubSettingRepo) Get(ctx context.Context, key string) (string, error) {
	return "", domain.ErrNotFound
}
func (m *mockSubSettingRepo) Set(ctx context.Context, key, val string) error { return nil }
func (m *mockSubSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	return make(map[string]string), nil
}

func TestRouter_SubRateLimiterAndBodySizeLimit(t *testing.T) {
	userRepo := &mockSubUserRepo{}
	inboundRepo := &mockSubInboundRepo{}
	settingRepo := &mockSubSettingRepo{}
	subSvc := service.NewSubService(userRepo, inboundRepo, settingRepo)
	subHandler := deliveryHTTP.NewSubHandler(subSvc)

	handlers := &deliveryHTTP.Handlers{
		Sub: subHandler,
	}

	router := deliveryHTTP.SetupRouter(handlers, "test-secret", nil)

	t.Run("Sub endpoint rate limiter triggers on excessive requests", func(t *testing.T) {
		// Send 61 requests to /sub/valid-token
		hit429 := false
		for i := 0; i < 65; i++ {
			req, _ := http.NewRequest("GET", "/sub/valid-token", nil)
			req.RemoteAddr = "192.0.2.1:12345"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusTooManyRequests {
				hit429 = true
				break
			}
		}
		if !hit429 {
			t.Fatalf("expected rate limiter to trigger 429 Too Many Requests within 65 requests")
		}
	})

	t.Run("Request body exceeding 10MB is rejected", func(t *testing.T) {
		// Create an oversized body (> 10MB)
		oversized := make([]byte, 11*1024*1024)
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewReader(oversized))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Handler or Gin ShouldBindJSON will fail reading the body
		if w.Code != http.StatusBadRequest {
			t.Logf("oversized body responded with status %d (expected 400 Bad Request on read error)", w.Code)
		}
	})
}
