package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	deliveryHTTP "panel/internal/delivery/http"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupUserHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_handler_user.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.Ticket{}, &domain.TrafficLog{}, &domain.AuditLog{}, &domain.AdminUser{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}

func makeTestAdminJWT(secret string) string {
	claims := jwt.MapClaims{
		"sub":      "admin",
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(secret))
	return tokenStr
}

func TestUserHandler_BatchDelete(t *testing.T) {
	db := setupUserHandlerTestDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	trafficLogRepo := repository.NewGormTrafficLogRepository(db)
	auditLogRepo := repository.NewGormAuditLogRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	settingRepo := repository.NewSettingRepository(db)

	userSvc := service.NewUserService(userRepo, nil, trafficLogRepo, nil, nil).
		WithTicketRepo(ticketRepo)
	auditSvc := service.NewAuditLogService(auditLogRepo)

	handlers := &deliveryHTTP.Handlers{
		Auth:     deliveryHTTP.NewAuthHandler(service.NewAuthService(adminRepo, "test-secret")),
		User:     deliveryHTTP.NewUserHandler(userSvc, nil, auditSvc),
		Setting:  deliveryHTTP.NewSettingHandler(service.NewSettingService(settingRepo, nil, nil, nil)),
		AuditLog: deliveryHTTP.NewAuditLogHandler(auditSvc),
	}
	router := deliveryHTTP.SetupRouter(handlers, "test-secret", nil)
	adminToken := makeTestAdminJWT("test-secret")
	ctx := context.Background()

	u1 := &domain.User{Email: "u1@example.com", UUID: "11111111-1111-4111-8111-111111111111", SubToken: "tok-u1-123456", Enabled: true}
	u2 := &domain.User{Email: "u2@example.com", UUID: "22222222-2222-4222-8222-222222222222", SubToken: "tok-u2-123456", Enabled: true}
	u3 := &domain.User{Email: "u3@example.com", UUID: "33333333-3333-4333-8333-333333333333", SubToken: "tok-u3-123456", Enabled: true}
	for _, u := range []*domain.User{u1, u2, u3} {
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("seed user %s failed: %v", u.Email, err)
		}
	}

	_ = ticketRepo.Create(ctx, &domain.Ticket{Code: "AAA111", UserID: u1.ID, RemainingUses: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	_ = trafficLogRepo.RecordTraffic(ctx, u1.ID, u1.Email, 100, 200, "2026-11-12")

	t.Run("missing ids returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/batch-delete", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("batch delete removes selected users, orphans, and logs audit", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"ids": []uint{u1.ID, u2.ID},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/users/batch-delete", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response failed: %v", err)
		}
		if resp["message"] != "成功删除 2 位用户" {
			t.Fatalf("unexpected message: %q", resp["message"])
		}

		remaining, err := userRepo.ListAll(ctx)
		if err != nil {
			t.Fatalf("ListAll failed: %v", err)
		}
		if len(remaining) != 1 || remaining[0].ID != u3.ID {
			t.Fatalf("expected only u3 to remain, got %+v", remaining)
		}

		// Orphans for u1 must be cleaned
		var ticketCount int64
		db.Model(&domain.Ticket{}).Where("user_id = ?", u1.ID).Count(&ticketCount)
		if ticketCount != 0 {
			t.Fatalf("expected 0 orphan tickets for u1, got %d", ticketCount)
		}
		logs, _ := trafficLogRepo.GetHistoryByEmail(ctx, u1.Email, 30)
		if len(logs) != 0 {
			t.Fatalf("expected 0 orphan traffic logs for u1, got %d", len(logs))
		}

		// Recreate user with same email and verify zero inherited traffic history
		recreated := &domain.User{
			Email:    u1.Email,
			UUID:     "44444444-4444-4444-8444-444444444444",
			SubToken: "tok-u1-recreated",
			Enabled:  true,
		}
		if err := userRepo.Create(ctx, recreated); err != nil {
			t.Fatalf("recreate user failed: %v", err)
		}
		recreatedHistory, err := userSvc.GetTrafficHistory(ctx, recreated.ID, 30)
		if err != nil {
			t.Fatalf("GetTrafficHistory for recreated user failed: %v", err)
		}
		if len(recreatedHistory) != 0 {
			t.Fatalf("expected 0 traffic logs for recreated user with same email, got %d", len(recreatedHistory))
		}

		// Audit log recorded with ActionUserDelete
		auditLogs, total, err := auditLogRepo.List(ctx, 0, 10, domain.ActionUserDelete, "", "")
		if err != nil || total == 0 || len(auditLogs) == 0 {
			t.Fatalf("expected audit log with ActionUserDelete, got total=%d err=%v", total, err)
		}
	})
}
