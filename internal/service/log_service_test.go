package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockLogReader struct {
	accessPath string
	errorPath  string
	lines      []string
}

func (m *mockLogReader) GetLogPaths() (string, string) {
	return m.accessPath, m.errorPath
}

func (m *mockLogReader) ReadLastLinesFiltered(filePath string, maxLines int, filter domain.LogFilter) ([]string, error) {
	return m.lines, nil
}

func (m *mockLogReader) ParseAccessLog(line string) *domain.AccessLogEntry {
	return &domain.AccessLogEntry{
		Time: "2026/09/20 12:00:00",
		Raw:  line,
	}
}

func (m *mockLogReader) ParseErrorLog(line string) *domain.ErrorLogEntry {
	return &domain.ErrorLogEntry{
		Time: "2026/09/20 12:00:00",
		Raw:  line,
	}
}

func TestLogService_GetRecentLogs(t *testing.T) {
	reader := &mockLogReader{
		accessPath: "/var/log/xray/access.log",
		errorPath:  "/var/log/xray/error.log",
		lines:      []string{"log line 1", "log line 2"},
	}

	svc := NewLogService(reader)

	resp, err := svc.GetRecentLogs(context.Background(), "access", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Type != "access" {
		t.Errorf("expected type 'access', got %s", resp.Type)
	}
	if len(resp.Lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(resp.Lines))
	}
	if len(resp.Access) != 2 {
		t.Errorf("expected 2 access entries, got %d", len(resp.Access))
	}
}
