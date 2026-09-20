package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"panel/internal/adapter/reality"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/delivery/http/middleware"
	"panel/internal/domain"
	"panel/internal/service"
)

type mockRealityInboundRepo struct {
	inbounds []domain.Inbound
}

func (m *mockRealityInboundRepo) Create(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockRealityInboundRepo) Update(ctx context.Context, inbound *domain.Inbound) error {
	return nil
}
func (m *mockRealityInboundRepo) Delete(ctx context.Context, id uint) error {
	return nil
}
func (m *mockRealityInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockRealityInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, nil
}
func (m *mockRealityInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockRealityInboundRepo) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return nil
}

type mockRealityProber struct{}

func (p *mockRealityProber) Probe(ctx context.Context, dest string, port int, serverName string) (*reality.ProbeResult, error) {
	return &reality.ProbeResult{
		TLSVersion: 0x0304, // TLS 1.3
		ALPN:       "h2",
		LatencyMs:  25,
		Headers:    make(http.Header),
	}, nil
}

func setupRealityTestRouter() (http.Handler, string) {
	jwtSecret := "test-secret-key-123456"

	rawStream := `{
		"security": "reality",
		"realitySettings": {
			"dest": "example.com:443",
			"serverNames": ["example.com"]
		}
	}`
	repo := &mockRealityInboundRepo{
		inbounds: []domain.Inbound{
			{
				ID:             1,
				Tag:            "reality-in",
				Protocol:       "vless",
				StreamSettings: rawStream,
				Enabled:        true,
			},
		},
	}
	prober := &mockRealityProber{}
	realitySvc := service.NewRealityMonitorService(repo, prober, nil)

	inboundHandler := deliveryHTTP.NewInboundHandler(nil, nil, realitySvc)

	handlers := &deliveryHTTP.Handlers{
		Inbound: inboundHandler,
	}

	router := deliveryHTTP.SetupRouter(handlers, jwtSecret, nil)
	token, _ := middleware.GenerateToken("admin", jwtSecret, time.Hour)
	return router, token
}

func TestRealityHandler_AuthProtection(t *testing.T) {
	router, _ := setupRealityTestRouter()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/inbounds/reality-status"},
		{"GET", "/api/v1/inbounds/reality-status"},
		{"POST", "/api/inbounds/reality-status/check"},
		{"POST", "/api/v1/inbounds/reality-status/check"},
	}

	for _, ep := range endpoints {
		t.Run("unauthenticated_"+ep.method+"_"+ep.path, func(t *testing.T) {
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 Unauthorized for %s %s, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

func TestRealityHandler_GetRealityStatus(t *testing.T) {
	router, token := setupRealityTestRouter()

	paths := []string{"/api/inbounds/reality-status", "/api/v1/inbounds/reality-status"}
	for _, p := range paths {
		t.Run("path_"+p, func(t *testing.T) {
			req, _ := http.NewRequest("GET", p, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
			}

			var resp struct {
				Code int                        `json:"code"`
				Msg  string                     `json:"msg"`
				Data domain.RealitySummaryStatus `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Code != 0 {
				t.Errorf("expected code 0, got %d", resp.Code)
			}
			if resp.Msg != "success" {
				t.Errorf("expected msg 'success', got '%s'", resp.Msg)
			}
		})
	}
}

func TestRealityHandler_TriggerRealityCheck(t *testing.T) {
	router, token := setupRealityTestRouter()

	paths := []string{"/api/inbounds/reality-status/check", "/api/v1/inbounds/reality-status/check"}
	for _, p := range paths {
		t.Run("path_"+p, func(t *testing.T) {
			req, _ := http.NewRequest("POST", p, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
			}

			var resp struct {
				Code int                        `json:"code"`
				Msg  string                     `json:"msg"`
				Data domain.RealitySummaryStatus `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Code != 0 {
				t.Errorf("expected code 0, got %d", resp.Code)
			}
			if resp.Msg != "reality check completed" {
				t.Errorf("expected msg 'reality check completed', got '%s'", resp.Msg)
			}
			if resp.Data.TotalChecked != 1 {
				t.Errorf("expected TotalChecked == 1, got %d", resp.Data.TotalChecked)
			}
		})
	}
}
