package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type mockSubHandlerUserRepo struct {
	user *domain.User
}

func (m *mockSubHandlerUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubHandlerUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (m *mockSubHandlerUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	return nil
}
func (m *mockSubHandlerUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (m *mockSubHandlerUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	return m.user, nil
}
func (m *mockSubHandlerUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	return m.user, nil
}
func (m *mockSubHandlerUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.user, nil
}
func (m *mockSubHandlerUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	if m.user != nil && m.user.SubToken == token {
		return m.user, nil
	}
	return nil, domain.ErrNotFound
}
func (m *mockSubHandlerUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (m *mockSubHandlerUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return nil, nil }
func (m *mockSubHandlerUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (m *mockSubHandlerUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

type mockSubHandlerInboundRepo struct {
	inbounds []domain.Inbound
}

func (m *mockSubHandlerInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockSubHandlerInboundRepo) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return nil
}
func (m *mockSubHandlerInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubHandlerInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockSubHandlerInboundRepo) Create(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockSubHandlerInboundRepo) Update(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockSubHandlerInboundRepo) Delete(ctx context.Context, id uint) error { return nil }

func TestSubHandler_GetSubscription_WithRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &domain.User{
		ID:          1,
		Email:       "user@example.com",
		UUID:        "7117295b-4362-4260-a133-b969344dfcd5",
		SubToken:    "test-sub-token",
		InboundTags: "vless-reality",
		Enabled:     true,
	}
	inbound := domain.Inbound{
		Tag:            "vless-reality",
		Protocol:       "vless",
		Port:           443,
		Listen:         "0.0.0.0",
		ExternalHost:   "198.51.100.1",
		Enabled:        true,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		SubRoutesJson:  `[{"id":"1","name":"日本节点","routeId":1,"outboundTag":"direct","enabled":true},{"id":"2","name":"美国节点","routeId":2,"outboundTag":"us-out","enabled":true}]`,
	}

	userRepo := &mockSubHandlerUserRepo{user: user}
	inboundRepo := &mockSubHandlerInboundRepo{inbounds: []domain.Inbound{inbound}}
	subSvc := service.NewSubService(userRepo, inboundRepo, nil)
	handler := NewSubHandler(subSvc)

	r := gin.New()
	r.GET("/sub/:token", handler.GetSubscription)

	t.Run("query without route returns all virtual nodes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sub/test-sub-token?format=raw", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		lines := strings.Split(strings.TrimSpace(body), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines when route omitted, got %d: %v", len(lines), lines)
		}
	})

	t.Run("query with route=2 filters specific virtual node", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sub/test-sub-token?format=raw&route=2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
		body := w.Body.String()
		lines := strings.Split(strings.TrimSpace(body), "\n")
		if len(lines) != 1 {
			t.Fatalf("expected 1 line for route=2, got %d: %v", len(lines), lines)
		}
		if !strings.Contains(lines[0], "7117295b-4362-0002-a133-b969344dfcd5") {
			t.Fatalf("expected routeId 2 encoded in UUID, got %s", lines[0])
		}
	})

	t.Run("missing token returns bad request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sub/?format=raw", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 or 404, got %d", w.Code)
		}
	})
}
