package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type mockAuditRepo struct {
	logs []domain.AuditLog
}

func (m *mockAuditRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	m.logs = append(m.logs, *log)
	return nil
}

func (m *mockAuditRepo) List(ctx context.Context, offset, limit int, action, operator, keyword string) ([]domain.AuditLog, int64, error) {
	return m.logs, int64(len(m.logs)), nil
}

func (m *mockAuditRepo) Prune(ctx context.Context, maxKeep int) error {
	return nil
}

func (m *mockAuditRepo) Clear(ctx context.Context) error {
	m.logs = nil
	return nil
}

func TestAuditLogHandler_GetAndClear(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &mockAuditRepo{
		logs: []domain.AuditLog{
			{ID: 1, Action: domain.ActionUserCreate, Target: "u1@test.com", Operator: "admin"},
		},
	}
	svc := service.NewAuditLogService(repo)
	handler := NewAuditLogHandler(svc)

	// 1. Get Audit Logs
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/audit-logs", nil)

	handler.GetAuditLogs(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// 2. Clear Audit Logs
	wClear := httptest.NewRecorder()
	cClear, _ := gin.CreateTestContext(wClear)
	cClear.Request = httptest.NewRequest("DELETE", "/api/audit-logs", nil)
	cClear.Set("username", "admin")

	handler.ClearAuditLogs(cClear)
	if wClear.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wClear.Code)
	}
}
