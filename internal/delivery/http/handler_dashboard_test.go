package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDashboardHandler_GetServiceStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewDashboardHandler(nil, "v2.6.6", "abcdef", "2026-10-15_12:00:00")

	router := gin.New()
	router.GET("/api/service/status", handler.GetServiceStatus)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/service/status", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		Active       bool   `json:"active"`
		SubState     string `json:"subState"`
		Version      string `json:"version"`
		PanelVersion string `json:"panelVersion"`
		Commit       string `json:"commit"`
		BuildTime    string `json:"buildTime"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if resp.PanelVersion != "v2.6.6" {
		t.Fatalf("expected panelVersion v2.6.6, got %s", resp.PanelVersion)
	}
	if resp.Commit != "abcdef" {
		t.Fatalf("expected commit abcdef, got %s", resp.Commit)
	}
	if resp.BuildTime != "2026-10-15_12:00:00" {
		t.Fatalf("expected buildTime 2026-10-15_12:00:00, got %s", resp.BuildTime)
	}
}
