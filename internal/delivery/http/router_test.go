package http_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/service"
)

func TestRouter_StaticCacheControlHeaders(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html": {
			Data: []byte(`<!DOCTYPE html><html><body><div id="app"></div></body></html>`),
		},
		"assets/app-123456.js": {
			Data: []byte(`console.log("ok");`),
		},
		"assets/style-abcdef.css": {
			Data: []byte(`body{margin:0}`),
		},
	}

	handlers := &deliveryHTTP.Handlers{
		Sub:  deliveryHTTP.NewSubHandler(nil),
		Auth: deliveryHTTP.NewAuthHandler(service.NewAuthService(nil, "secret")),
	}
	router := deliveryHTTP.SetupRouter(handlers, "secret", staticFS)

	t.Run("hashed assets get long-lived immutable Cache-Control", func(t *testing.T) {
		for _, target := range []string{"/assets/app-123456.js", "/assets/style-abcdef.css"} {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200 OK, got %d", target, w.Code)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
				t.Fatalf("%s: expected immutable Cache-Control, got %q", target, cc)
			}
		}
	})

	t.Run("root, index.html, and SPA fallback return no-cache", func(t *testing.T) {
		for _, target := range []string{"/", "/topology", "/assets/missing-999.js"} {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200 OK, got %d", target, w.Code)
			}
			if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
				t.Fatalf("%s: expected Cache-Control=no-cache, got %q", target, cc)
			}
			if !strings.Contains(w.Body.String(), `<div id="app"></div>`) {
				t.Fatalf("%s: expected index.html body, got %q", target, w.Body.String())
			}
		}

		req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("/index.html: expected Cache-Control=no-cache, got %q", cc)
		}
	})
}
