package http_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	deliveryHTTP "panel/internal/delivery/http"
)

func TestRouter_StaticFilesAndSPAFallback(t *testing.T) {
	staticFS := os.DirFS("../../../web/dist")
	handlers := &deliveryHTTP.Handlers{
		Sub:  deliveryHTTP.NewSubHandler(nil),
		Auth: deliveryHTTP.NewAuthHandler(nil, "secret"),
	}

	router := deliveryHTTP.SetupRouter(handlers, "secret", staticFS)

	t.Run("Serves favicon.svg directly", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/favicon.svg", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /favicon.svg, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "<svg") {
			t.Fatalf("expected svg content, got %s", body)
		}
	})

	t.Run("Serves index.html on SPA routes", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/topology", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for SPA route /topology, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "<!DOCTYPE html>") && !strings.Contains(body, "<div id=\"app\">") {
			t.Fatalf("expected index.html content, got %s", body)
		}
	})

	t.Run("API unknown route returns 404 JSON", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/unknown-endpoint", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for unknown api endpoint, got %d", w.Code)
		}
	})
}
