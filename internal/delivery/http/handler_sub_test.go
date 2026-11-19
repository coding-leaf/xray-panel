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
func (s *stubSubInboundRepo) FindByNodeID(ctx context.Context, nodeID uint) ([]domain.Inbound, error) {
	var matched []domain.Inbound
	for _, in := range s.inbounds {
		if in.NodeID == nodeID {
			matched = append(matched, in)
		}
	}
	return matched, nil
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

func TestSubHandler_ClashVergeRealityIncludesMLKEM(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &domain.User{
		ID:          1,
		Email:       "reality@example.com",
		UUID:        "11111111-1111-4111-8111-111111111111",
		SubToken:    "reality-token-123",
		InboundTags: "vless-reality",
		Enabled:     true,
	}
	inbounds := []domain.Inbound{
		{
			ID:       1,
			Tag:      "vless-reality",
			Remark:   "JP REALITY",
			Protocol: "vless",
			Port:     443,
			Enabled:  true,
			StreamSettings: `{
				"network": "tcp",
				"security": "reality",
				"realitySettings": {
					"serverNames": ["www.apple.com"],
					"publicKey": "test-reality-public-key",
					"shortIds": ["0123456789abcdef"],
					"fingerprint": "chrome"
				}
			}`,
		},
	}
	settings := map[string]string{
		"sub_domain":             "sub.example.com",
		"sub_show_info_nodes":    "false",
		"sub_default_fingerprint": "chrome",
	}

	subSvc := service.NewSubService(
		&stubSubUserRepo{user: user},
		&stubSubInboundRepo{inbounds: inbounds},
		&stubSubSettingRepo{settings: settings},
	)
	handler := NewSubHandler(subSvc)

	router := gin.New()
	router.GET("/sub/:token", handler.GetSubscription)

	req := httptest.NewRequest(http.MethodGet, "/sub/"+user.SubToken, nil)
	req.Header.Set("User-Agent", "ClashVerge/2.0.0")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "support-x25519mlkem768: true") {
		t.Fatalf("expected Clash Verge YAML to contain support-x25519mlkem768: true, got:\n%s", body)
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("expected valid clash yaml, got error: %v\n%s", err, body)
	}
	if _, hasPort := parsed["port"]; hasPort {
		t.Fatalf("expected port key to be omitted, got port=%v", parsed["port"])
	}
	if _, hasSocksPort := parsed["socks-port"]; hasSocksPort {
		t.Fatalf("expected socks-port key to be omitted, got socks-port=%v", parsed["socks-port"])
	}
	proxies, ok := parsed["proxies"].([]interface{})
	if !ok || len(proxies) != 1 {
		t.Fatalf("expected 1 proxy in Clash YAML, got: %+v", parsed["proxies"])
	}
	proxy0, ok := proxies[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected proxy map, got: %T", proxies[0])
	}
	realityOpts, ok := proxy0["reality-opts"].(map[string]interface{})
	if !ok || realityOpts["support-x25519mlkem768"] != true {
		t.Fatalf("expected reality-opts.support-x25519mlkem768 == true, got: %+v", proxy0["reality-opts"])
	}

	// Ensure Base64 / vless:// output for non-Clash UA remains untouched
	rawReq := httptest.NewRequest(http.MethodGet, "/sub/"+user.SubToken, nil)
	rawReq.Header.Set("User-Agent", "v2rayN/6.45")
	rawRec := httptest.NewRecorder()
	router.ServeHTTP(rawRec, rawReq)
	if rawRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for v2rayN UA, got %d: %s", rawRec.Code, rawRec.Body.String())
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rawRec.Body.String()))
	if err != nil {
		t.Fatalf("expected valid base64 response, got err: %v", err)
	}
	if strings.Contains(string(decoded), "support-x25519mlkem768") {
		t.Fatalf("expected vless:// URI not to contain support-x25519mlkem768, got: %s", string(decoded))
	}
}
