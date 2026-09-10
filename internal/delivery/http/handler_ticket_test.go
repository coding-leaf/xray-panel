package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func setupTestDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test_handler_ticket.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.Ticket{}, &domain.Inbound{}, &domain.Setting{}, &domain.AdminUser{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}

func generateAdminToken(secret string) string {
	claims := jwt.MapClaims{
		"sub":      "admin",
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(secret))
	return tokenStr
}

func TestTicketHandler_CreateTicket(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	subSvc := service.NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := service.NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ticketHandler := deliveryHTTP.NewTicketHandler(ticketSvc)

	handlers := &deliveryHTTP.Handlers{
		Ticket:  ticketHandler,
		Auth:    deliveryHTTP.NewAuthHandler(adminRepo, "test-secret"),
		Setting: deliveryHTTP.NewSettingHandler(settingRepo, nil, nil, nil),
	}
	router := deliveryHTTP.SetupRouter(handlers, "test-secret", nil)
	adminToken := generateAdminToken("test-secret")

	// 1. Create a test active user
	user := &domain.User{
		Email:       "alice@example.com",
		UUID:        "11111111-2222-3333-4444-555555555555",
		SubToken:    "alice-token",
		Enabled:     true,
		InboundTags: "in-1",
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	t.Run("Create ticket successfully", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"ttl_minutes": 20,
			"max_uses":    3,
		})
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/users/%d/tickets", user.ID), bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
		}

		var res map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to parse json response: %v", err)
		}
		code, ok := res["code"].(string)
		if !ok || len(code) != 6 {
			t.Fatalf("expected 6-char code, got %v", res["code"])
		}
		if int(res["remaining_uses"].(float64)) != 3 {
			t.Fatalf("expected remaining_uses 3, got %v", res["remaining_uses"])
		}
		if res["share_text"] == "" {
			t.Fatal("expected non-empty share_text")
		}
	})

	t.Run("Create ticket for non-existent user returns 404", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"ttl_minutes": 15,
			"max_uses":    2,
		})
		req := httptest.NewRequest("POST", "/api/users/9999/tickets", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("Create ticket without auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/users/%d/tickets", user.ID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})
}

func TestTicketHandler_ClaimTicket(t *testing.T) {
	db := setupTestDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	adminRepo := repository.NewAdminRepository(db)

	subSvc := service.NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := service.NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ticketHandler := deliveryHTTP.NewTicketHandler(ticketSvc)

	handlers := &deliveryHTTP.Handlers{
		Ticket:  ticketHandler,
		Auth:    deliveryHTTP.NewAuthHandler(adminRepo, "test-secret"),
		Setting: deliveryHTTP.NewSettingHandler(settingRepo, nil, nil, nil),
	}
	router := deliveryHTTP.SetupRouter(handlers, "test-secret", nil)

	// Create user
	user := &domain.User{
		Email:       "bob@example.com",
		UUID:        "22222222-3333-4444-5555-666666666666",
		SubToken:    "bob-sub-token",
		Enabled:     true,
		InboundTags: "in-bob",
	}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user bob failed: %v", err)
	}

	// Generate ticket
	ticket, _, err := ticketSvc.GenerateTicket(context.Background(), user.ID, 15, 2)
	if err != nil {
		t.Fatalf("generate ticket failed: %v", err)
	}

	t.Run("Claim ticket successfully", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"code": ticket.Code,
		})
		req := httptest.NewRequest("POST", "/api/portal/claim", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
		}

		var payload domain.TicketClaimPayload
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode response payload: %v", err)
		}
		if payload.UserEmail != "bob@example.com" {
			t.Fatalf("expected user bob@example.com, got %s", payload.UserEmail)
		}
		if payload.SubscriptionURL == "" {
			t.Fatal("expected non-empty subscription_url")
		}
	})

	t.Run("Claim ticket with invalid code returns 400 and uniform error message", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"code": "WRONG1",
		})
		req := httptest.NewRequest("POST", "/api/portal/claim", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.2:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", w.Code)
		}
		var res map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res["error"] != "凭据无效、已过期或已被销毁" {
			t.Fatalf("expected uniform error message, got %s", res["error"])
		}
	})

	t.Run("Consecutive failures trigger IP ban", func(t *testing.T) {
		testIP := "198.51.100.99"
		for i := 0; i < 5; i++ {
			body, _ := json.Marshal(map[string]string{"code": fmt.Sprintf("ERR%03d", i)})
			req := httptest.NewRequest("POST", "/api/portal/claim", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.RemoteAddr = testIP + ":54321"
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		// 6th attempt should be blocked with 429
		body, _ := json.Marshal(map[string]string{"code": "ERR999"})
		req := httptest.NewRequest("POST", "/api/portal/claim", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = testIP + ":54321"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Fatalf("expected 429 Too Many Requests on IP ban, got %d", w.Code)
		}
	})
}
