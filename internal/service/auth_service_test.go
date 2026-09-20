package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"panel/internal/domain"
	"panel/internal/pkg/totp"

	"golang.org/x/crypto/bcrypt"
)

type mockAdminRepo struct {
	admins map[string]*domain.AdminUser
}

func newMockAdminRepo() *mockAdminRepo {
	return &mockAdminRepo{admins: make(map[string]*domain.AdminUser)}
}

func (m *mockAdminRepo) GetByUsername(ctx context.Context, username string) (*domain.AdminUser, error) {
	if a, ok := m.admins[username]; ok {
		return a, nil
	}
	return nil, errors.New("admin not found")
}

func (m *mockAdminRepo) Update(ctx context.Context, admin *domain.AdminUser) error {
	m.admins[admin.Username] = admin
	return nil
}

func (m *mockAdminRepo) Create(ctx context.Context, admin *domain.AdminUser) error {
	m.admins[admin.Username] = admin
	return nil
}

func TestAuthService_Login(t *testing.T) {
	repo := newMockAdminRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	repo.admins["admin"] = &domain.AdminUser{
		Username:     "admin",
		PasswordHash: string(hash),
	}

	authSvc := NewAuthService(repo, "test-secret")

	// 1. Success login without 2FA
	res, err := authSvc.Login(context.Background(), "admin", "correct-password", "", "127.0.0.1")
	if err != nil {
		t.Fatalf("expected login success, got err: %v", err)
	}
	if res.Username != "admin" || res.Token == "" || res.TOTPEnabled {
		t.Errorf("unexpected login result: %+v", res)
	}

	// 2. Failed login with wrong password
	_, err = authSvc.Login(context.Background(), "admin", "wrong-password", "", "127.0.0.1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// 3. Failed login with non-existent user
	_, err = authSvc.Login(context.Background(), "ghost", "any", "", "127.0.0.1")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// 4. User with 2FA enabled
	secret := "JBSWY3DPEHPK3PXP"
	repo.admins["admin2fa"] = &domain.AdminUser{
		Username:     "admin2fa",
		PasswordHash: string(hash),
		TOTPEnabled:  true,
		TOTPSecret:   secret,
	}

	// 4a. Missing or wrong passcode
	_, err = authSvc.Login(context.Background(), "admin2fa", "correct-password", "", "127.0.0.1")
	if !errors.Is(err, ErrRequire2FA) {
		t.Fatalf("expected ErrRequire2FA, got %v", err)
	}

	// 4b. Valid passcode
	validCode, _ := totp.GenerateCode(secret, time.Now())
	res2fa, err := authSvc.Login(context.Background(), "admin2fa", "correct-password", validCode, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected 2fa login success, got err: %v", err)
	}
	if !res2fa.TOTPEnabled {
		t.Errorf("expected TOTPEnabled to be true")
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	repo := newMockAdminRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	repo.admins["admin"] = &domain.AdminUser{
		Username:     "admin",
		PasswordHash: string(hash),
	}

	authSvc := NewAuthService(repo, "test-secret")

	// 1. Wrong old password
	err := authSvc.ChangePassword(context.Background(), "admin", "bad-old", "new-pass", "127.0.0.1")
	if !errors.Is(err, ErrIncorrectOldPassword) {
		t.Fatalf("expected ErrIncorrectOldPassword, got %v", err)
	}

	// 2. Correct old password
	err = authSvc.ChangePassword(context.Background(), "admin", "old-password", "new-pass", "127.0.0.1")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Verify login with new password
	_, err = authSvc.Login(context.Background(), "admin", "new-pass", "", "127.0.0.1")
	if err != nil {
		t.Fatalf("expected login with new password, got err: %v", err)
	}
}

func TestAuthService_2FALifecycle(t *testing.T) {
	repo := newMockAdminRepo()
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo.admins["admin"] = &domain.AdminUser{
		Username:     "admin",
		PasswordHash: string(hash),
	}

	authSvc := NewAuthService(repo, "test-secret")

	// 1. Setup 2FA
	secret, url, err := authSvc.Setup2FA(context.Background(), "admin")
	if err != nil || secret == "" || url == "" {
		t.Fatalf("Setup2FA failed: err=%v, secret=%s, url=%s", err, secret, url)
	}

	// 2. Enable 2FA with invalid code
	err = authSvc.Enable2FA(context.Background(), "admin", secret, "000000", "127.0.0.1")
	if !errors.Is(err, ErrInvalid2FACode) {
		t.Fatalf("expected ErrInvalid2FACode, got %v", err)
	}

	// 3. Enable 2FA with valid code
	validCode, _ := totp.GenerateCode(secret, time.Now())
	err = authSvc.Enable2FA(context.Background(), "admin", secret, validCode, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected Enable2FA success, got %v", err)
	}

	info, err := authSvc.GetAdminInfo(context.Background(), "admin")
	if err != nil || !info.TOTPEnabled {
		t.Fatalf("expected TOTPEnabled to be true, got %+v, err=%v", info, err)
	}

	// 4. Disable 2FA with wrong password
	err = authSvc.Disable2FA(context.Background(), "admin", "wrong-pass", validCode, "127.0.0.1")
	if !errors.Is(err, ErrIncorrectPassword) {
		t.Fatalf("expected ErrIncorrectPassword, got %v", err)
	}

	// 5. Disable 2FA with correct password and valid code
	validCode2, _ := totp.GenerateCode(secret, time.Now())
	err = authSvc.Disable2FA(context.Background(), "admin", "password123", validCode2, "127.0.0.1")
	if err != nil {
		t.Fatalf("expected Disable2FA success, got %v", err)
	}

	info, _ = authSvc.GetAdminInfo(context.Background(), "admin")
	if info.TOTPEnabled {
		t.Fatalf("expected TOTPEnabled to be false after disable")
	}
}
