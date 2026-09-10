package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTestTicketDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test_ticket_repo.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite test db: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.Ticket{}); err != nil {
		t.Fatalf("failed to auto-migrate tables: %v", err)
	}
	return db
}

func TestTicketRepo_CRUD(t *testing.T) {
	db := setupTestTicketDB(t)
	repo := NewTicketRepository(db)
	userRepo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Email: "user@test.com", UUID: "uuid-1", SubToken: "sub-1", Enabled: true}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	now := time.Now().Unix()
	ticket := &domain.Ticket{
		Code:          "7K9X2P",
		UserID:        user.ID,
		RemainingUses: 2,
		ExpiresAt:     now + 900,
		CreatedAt:     now,
	}

	if err := repo.Create(ctx, ticket); err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	fetched, err := repo.GetByCode(ctx, "7K9X2P")
	if err != nil {
		t.Fatalf("failed to get ticket: %v", err)
	}
	if fetched.UserID != user.ID || fetched.User == nil || fetched.User.Email != "user@test.com" {
		t.Fatalf("preloaded user mismatch: %+v", fetched)
	}

	// Delete
	if err := repo.Delete(ctx, ticket.ID); err != nil {
		t.Fatalf("failed to delete ticket: %v", err)
	}

	_, err = repo.GetByCode(ctx, "7K9X2P")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}
}

func TestTicketRepo_ConsumeAtomic(t *testing.T) {
	db := setupTestTicketDB(t)
	repo := NewTicketRepository(db)
	userRepo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Email: "consumer@test.com", UUID: "uuid-c", SubToken: "sub-c", Enabled: true}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	now := time.Now().Unix()
	ticket := &domain.Ticket{
		Code:          "CONSUME1",
		UserID:        user.ID,
		RemainingUses: 2,
		ExpiresAt:     now + 600,
		CreatedAt:     now,
	}
	if err := repo.Create(ctx, ticket); err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	// 1st consumption
	res1, err := repo.ConsumeAtomic(ctx, "CONSUME1")
	if err != nil {
		t.Fatalf("1st consume failed: %v", err)
	}
	if res1.RemainingUses != 1 {
		t.Fatalf("expected remaining uses 1, got %d", res1.RemainingUses)
	}

	// 2nd consumption
	res2, err := repo.ConsumeAtomic(ctx, "CONSUME1")
	if err != nil {
		t.Fatalf("2nd consume failed: %v", err)
	}
	if res2.RemainingUses != 0 {
		t.Fatalf("expected remaining uses 0, got %d", res2.RemainingUses)
	}

	// 3rd consumption (should fail due to RemainingUses == 0)
	_, err = repo.ConsumeAtomic(ctx, "CONSUME1")
	if !errors.Is(err, domain.ErrTicketInvalidOrExpired) {
		t.Fatalf("expected ErrTicketInvalidOrExpired, got %v", err)
	}
}

func TestTicketRepo_ConsumeAtomic_Expired(t *testing.T) {
	db := setupTestTicketDB(t)
	repo := NewTicketRepository(db)
	userRepo := NewUserRepository(db)
	ctx := context.Background()

	user := &domain.User{Email: "exp@test.com", UUID: "uuid-e", SubToken: "sub-e", Enabled: true}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	now := time.Now().Unix()
	ticket := &domain.Ticket{
		Code:          "EXPIRED1",
		UserID:        user.ID,
		RemainingUses: 2,
		ExpiresAt:     now - 10, // already expired
		CreatedAt:     now - 100,
	}
	if err := repo.Create(ctx, ticket); err != nil {
		t.Fatalf("failed to create ticket: %v", err)
	}

	_, err := repo.ConsumeAtomic(ctx, "EXPIRED1")
	if !errors.Is(err, domain.ErrTicketInvalidOrExpired) {
		t.Fatalf("expected ErrTicketInvalidOrExpired on expired ticket, got %v", err)
	}
}

func TestTicketRepo_CleanExpired(t *testing.T) {
	db := setupTestTicketDB(t)
	repo := NewTicketRepository(db)
	ctx := context.Background()

	now := time.Now().Unix()
	active := &domain.Ticket{Code: "ACTIVE", UserID: 1, RemainingUses: 2, ExpiresAt: now + 600}
	expired := &domain.Ticket{Code: "EXP", UserID: 1, RemainingUses: 2, ExpiresAt: now - 60}
	usedUp := &domain.Ticket{Code: "USED", UserID: 1, RemainingUses: 0, ExpiresAt: now + 600}

	_ = repo.Create(ctx, active)
	_ = repo.Create(ctx, expired)
	_ = repo.Create(ctx, usedUp)

	if err := repo.CleanExpired(ctx); err != nil {
		t.Fatalf("clean expired failed: %v", err)
	}

	if _, err := repo.GetByCode(ctx, "ACTIVE"); err != nil {
		t.Fatalf("active ticket should exist: %v", err)
	}
	if _, err := repo.GetByCode(ctx, "EXP"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expired ticket should be cleaned")
	}
	if _, err := repo.GetByCode(ctx, "USED"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("used ticket should be cleaned")
	}
}
