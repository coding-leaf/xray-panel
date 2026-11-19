package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type stubSubUserRepo struct {
	user *domain.User
}

func (s *stubSubUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (s *stubSubUserRepo) Update(ctx context.Context, user *domain.User) error { return nil }
func (s *stubSubUserRepo) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	return nil
}
func (s *stubSubUserRepo) Delete(ctx context.Context, id uint) error { return nil }
func (s *stubSubUserRepo) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	return s.user, nil
}
func (s *stubSubUserRepo) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (s *stubSubUserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (s *stubSubUserRepo) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	if s.user != nil && s.user.SubToken == token {
		return s.user, nil
	}
	return nil, domain.ErrNotFound
}
func (s *stubSubUserRepo) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	return nil, nil
}
func (s *stubSubUserRepo) ListAll(ctx context.Context) ([]domain.User, error) { return nil, nil }
func (s *stubSubUserRepo) AddTraffic(ctx context.Context, email string, up, down int64) error {
	return nil
}
func (s *stubSubUserRepo) ResetTraffic(ctx context.Context, id uint) error { return nil }

type stubSubInboundRepo struct {
	inbounds []domain.Inbound
}

func (s *stubSubInboundRepo) Create(ctx context.Context, inbound *domain.Inbound) error { return nil }
func (s *stubSubInboundRepo) Update(ctx context.Context, inbound *domain.Inbound) error { return nil }
func (s *stubSubInboundRepo) Delete(ctx context.Context, id uint) error                 { return nil }
func (s *stubSubInboundRepo) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	return nil, domain.ErrNotFound
}
func (s *stubSubInboundRepo) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	return nil, domain.ErrNotFound
}
func (s *stubSubInboundRepo) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	return s.inbounds, nil
}
func (s *stubSubInboundRepo) AddTraffic(ctx context.Context, tag string, up, down int64) error {
	return nil
}

type stubSubSettingRepo struct {
	settings map[string]string
}

func (s *stubSubSettingRepo) Get(ctx context.Context, key string) (string, error) {
	if v, ok := s.settings[key]; ok {
		return v, nil
	}
	return "", domain.ErrNotFound
}
func (s *stubSubSettingRepo) Set(ctx context.Context, key, value string) error { return nil }
func (s *stubSubSettingRepo) GetAll(ctx context.Context) (map[string]string, error) {
	return s.settings, nil
}

func TestSubHandler_UADetectionAndExplicitOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &domain.User{
		ID:          1,
		Email:       "ua@example.com",
		UUID:        "11111111-1111-4111-8111-111111111111",
		SubToken:    "ua-token-123",
		InboundTags: "vless-tcp",
		Enabled:     true,
	}
	inbounds := []domain.Inbound{
		{
			ID:             1,
			Tag:            "vless-tcp",
			Remark:         "Direct Node",
			Protocol:       "vless",
			Port:           443,
			Enabled:        true,
			StreamSettings: `{"network":"tcp","security":"none"}`,
		},
	}
	settings := map[string]string{
		"sub_domain":          "sub.example.com",
		"sub_show_info_nodes": "false",
	}

	subSvc := service.NewSubService(
		&stubSubUserRepo{user: user},
		&stubSubInboundRepo{inbounds: inbounds},
		&stubSubSettingRepo{settings: settings},
	)
	handler := NewSubHandler(subSvc)

	router := gin.New()
	router.GET("/sub/:token", handler.GetSubscription)

	cases := []struct {
		name        string
		query       string
		userAgent   string
		wantCT      string
		assertBody  func(t *testing.T, body string)
	}{
		{
			name:      "clash UA returns yaml",
			userAgent: "ClashVerge/1.7.7",
			wantCT:    "application/yaml; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				var parsed map[string]interface{}
				if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
					t.Fatalf("expected valid clash yaml, got err: %v\nbody:\n%s", err, body)
				}
				if _, ok := parsed["proxies"]; !ok {
					t.Fatalf("expected proxies key in clash yaml")
				}
			},
		},
		{
			name:      "mihomo UA returns yaml",
			userAgent: "Mihomo/1.18.0",
			wantCT:    "application/yaml; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "proxy-groups:") {
					t.Fatalf("expected clash yaml output, got:\n%s", body)
				}
			},
		},
		{
			name:      "stash UA returns yaml",
			userAgent: "Stash/2.5.0",
			wantCT:    "application/yaml; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				if !strings.Contains(body, "proxy-groups:") {
					t.Fatalf("expected clash yaml output, got:\n%s", body)
				}
			},
		},
		{
			name:      "sing-box UA returns json",
			userAgent: "sing-box 1.9.0",
			wantCT:    "application/json; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(body), &parsed); err != nil {
					t.Fatalf("expected valid sing-box json, got err: %v\nbody:\n%s", err, body)
				}
			},
		},
		{
			name:      "v2rayN UA defaults to base64",
			userAgent: "v2rayN/6.45",
			wantCT:    "text/plain; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body))
				if err != nil {
					t.Fatalf("expected base64 payload, got err: %v, body: %s", err, body)
				}
				if !strings.Contains(string(decoded), "vless://") {
					t.Fatalf("expected decoded vless link, got: %s", string(decoded))
				}
			},
		},
		{
			name:      "explicit format overrides clash UA",
			query:     "?format=raw",
			userAgent: "Clash.Meta/1.0",
			wantCT:    "text/plain; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				if !strings.HasPrefix(strings.TrimSpace(body), "vless://") {
					t.Fatalf("expected raw vless:// link, got: %s", body)
				}
			},
		},
		{
			name:      "explicit type overrides UA",
			query:     "?type=singbox",
			userAgent: "ClashVerge/1.7.7",
			wantCT:    "application/json; charset=utf-8",
			assertBody: func(t *testing.T, body string) {
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(body), &parsed); err != nil {
					t.Fatalf("expected valid sing-box json, got err: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/sub/"+user.SubToken+tc.query, nil)
			req.Header.Set("User-Agent", tc.userAgent)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if gotCT := rec.Header().Get("Content-Type"); gotCT != tc.wantCT {
				t.Fatalf("expected Content-Type %q, got %q", tc.wantCT, gotCT)
			}
			tc.assertBody(t, rec.Body.String())
		})
	}
}
