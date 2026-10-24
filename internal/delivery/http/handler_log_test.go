package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type mockLogReader struct {
	lastFilter domain.LogFilter
	lastLines  int
	lastPath   string
}

func (m *mockLogReader) GetLogPaths() (accessLog, errorLog string) {
	return "/var/log/xray/access.log", "/var/log/xray/error.log"
}

func (m *mockLogReader) ReadLastLinesFiltered(filePath string, maxLines int, filter domain.LogFilter) ([]string, error) {
	m.lastPath = filePath
	m.lastLines = maxLines
	m.lastFilter = filter
	return []string{
		"2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: user@test.com",
	}, nil
}

func (m *mockLogReader) ParseAccessLog(line string) *domain.AccessLogEntry {
	return &domain.AccessLogEntry{
		Time:        "2026/09/12 10:00:01",
		FromIP:      "127.0.0.1:1000",
		Protocol:    "TCP",
		Target:      "example.com:443",
		Route:       "vless-in -> direct",
		InboundTag:  "vless-in",
		OutboundTag: "direct",
		Email:       "user@test.com",
		Action:      "accepted",
		Raw:         line,
	}
}

func (m *mockLogReader) ParseErrorLog(line string) *domain.ErrorLogEntry {
	return &domain.ErrorLogEntry{
		Time:    "2026/09/12 10:00:01",
		Level:   "WARN",
		Module:  "Core",
		Message: "test message",
		Raw:     line,
	}
}

func TestLogHandler_GetLogs_FilterParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reader := &mockLogReader{}
	logSvc := service.NewLogService(reader)
	handler := NewLogHandler(logSvc)

	t.Run("GetLogs access mode with inbound and outbound filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/logs?type=access&lines=50&inbound=vless-in&outbound=warp-out&keyword=example", nil)

		handler.GetLogs(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		if reader.lastLines != 50 {
			t.Errorf("expected maxLines 50, got %d", reader.lastLines)
		}
		if reader.lastFilter.InboundTag != "vless-in" {
			t.Errorf("expected inbound vless-in, got %s", reader.lastFilter.InboundTag)
		}
		if reader.lastFilter.OutboundTag != "warp-out" {
			t.Errorf("expected outbound warp-out, got %s", reader.lastFilter.OutboundTag)
		}
		if reader.lastFilter.Keyword != "example" {
			t.Errorf("expected keyword example, got %s", reader.lastFilter.Keyword)
		}

		var resp service.LogResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if len(resp.Access) != 1 || resp.Access[0].OutboundTag != "direct" {
			t.Errorf("unexpected access entries in response: %+v", resp.Access)
		}
	})

	t.Run("GetLogs error mode with level filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/api/logs?type=error&lines=20&level=ERROR", nil)

		handler.GetLogs(c)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}

		if reader.lastLines != 20 {
			t.Errorf("expected maxLines 20, got %d", reader.lastLines)
		}
		if reader.lastFilter.Level != "ERROR" {
			t.Errorf("expected level ERROR, got %s", reader.lastFilter.Level)
		}
	})
}
