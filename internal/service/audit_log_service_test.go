package service

import (
	"context"
	"testing"

	"panel/internal/domain"
)

type mockAuditLogRepo struct {
	logs []domain.AuditLog
}

func (m *mockAuditLogRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	log.ID = uint(len(m.logs) + 1)
	m.logs = append(m.logs, *log)
	return nil
}

func (m *mockAuditLogRepo) List(ctx context.Context, offset, limit int, action, operator, keyword string) ([]domain.AuditLog, int64, error) {
	return m.logs, int64(len(m.logs)), nil
}

func (m *mockAuditLogRepo) Prune(ctx context.Context, maxKeep int) error {
	if len(m.logs) > maxKeep {
		m.logs = m.logs[len(m.logs)-maxKeep:]
	}
	return nil
}

func (m *mockAuditLogRepo) Clear(ctx context.Context) error {
	m.logs = nil
	return nil
}

func TestAuditLogService_RecordAndList(t *testing.T) {
	repo := &mockAuditLogRepo{}
	svc := NewAuditLogService(repo)

	ctx := context.Background()
	err := svc.Record(ctx, "admin", "127.0.0.1", domain.ActionUserCreate, "test@example.com", "create user", "SUCCESS")
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}

	items, total, err := svc.List(ctx, 1, 10, "", "", "")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 log, got total=%d, len=%d", total, len(items))
	}
	if items[0].Action != domain.ActionUserCreate || items[0].Target != "test@example.com" {
		t.Fatalf("unexpected log item: %+v", items[0])
	}

	// Test nil receiver safety
	var nilSvc *AuditLogService
	nilSvc.RecordFromGin(nil, "TEST", "target", "details", "SUCCESS")
	_ = nilSvc.Record(ctx, "admin", "127.0.0.1", "TEST", "target", "details", "SUCCESS")
	logs, count, err := nilSvc.List(ctx, 1, 10, "", "", "")
	if err != nil || count != 0 || len(logs) != 0 {
		t.Fatal("nil receiver should return empty results without error")
	}

	// Test Clear
	if err := svc.Clear(ctx); err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	items, total, _ = svc.List(ctx, 1, 10, "", "", "")
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected 0 logs after clear, got %d", total)
	}
}
