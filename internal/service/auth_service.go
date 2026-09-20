package service

import (
	"context"
	"errors"
	"time"

	"panel/internal/domain"
	paneljwt "panel/internal/pkg/jwt"
	"panel/internal/pkg/totp"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials   = errors.New("invalid username or password")
	ErrRequire2FA           = errors.New("invalid 2fa passcode")
	ErrUserNotFound         = errors.New("user not found")
	ErrIncorrectPassword    = errors.New("incorrect password")
	ErrIncorrectOldPassword = errors.New("incorrect old password")
	ErrInvalid2FACode       = errors.New("invalid 2fa verification code")
	Err2FANotEnabled        = errors.New("2fa is not enabled")
)

type LoginResult struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	TOTPEnabled bool   `json:"totpEnabled"`
}

type AuthService struct {
	adminRepo domain.AdminRepository
	jwtSecret string
	auditSvc  *AuditLogService
}

func NewAuthService(adminRepo domain.AdminRepository, jwtSecret string, auditSvc ...*AuditLogService) *AuthService {
	s := &AuthService{
		adminRepo: adminRepo,
		jwtSecret: jwtSecret,
	}
	if len(auditSvc) > 0 {
		s.auditSvc = auditSvc[0]
	}
	return s
}

func (s *AuthService) Login(ctx context.Context, username, password, passcode, clientIP string) (*LoginResult, error) {
	admin, err := s.adminRepo.GetByUsername(ctx, username)
	if err != nil {
		if s.auditSvc != nil {
			_ = s.auditSvc.Record(ctx, username, clientIP, domain.ActionAuthLoginFailed, username, "登录失败: 用户名不存在", "FAILED")
		}
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		if s.auditSvc != nil {
			_ = s.auditSvc.Record(ctx, username, clientIP, domain.ActionAuthLoginFailed, username, "登录失败: 密码错误", "FAILED")
		}
		return nil, ErrInvalidCredentials
	}

	// 2FA 校验
	if admin.TOTPEnabled {
		if passcode == "" || !totp.Validate(passcode, admin.TOTPSecret) {
			if s.auditSvc != nil {
				_ = s.auditSvc.Record(ctx, username, clientIP, domain.ActionAuthLoginFailed, username, "登录失败: 2FA 动态码错误", "FAILED")
			}
			return nil, ErrRequire2FA
		}
	}

	token, err := paneljwt.GenerateToken(admin.Username, s.jwtSecret, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, admin.Username, clientIP, domain.ActionAuthLogin, admin.Username, "管理员登录成功", "SUCCESS")
	}

	return &LoginResult{
		Token:       token,
		Username:    admin.Username,
		TOTPEnabled: admin.TOTPEnabled,
	}, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, username, oldPassword, newPassword, clientIP string) error {
	admin, err := s.adminRepo.GetByUsername(ctx, username)
	if err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrIncorrectOldPassword
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	admin.PasswordHash = string(newHash)
	admin.UpdatedAt = time.Now()
	if err := s.adminRepo.Update(ctx, admin); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, admin.Username, clientIP, domain.ActionAuthPassword, admin.Username, "修改管理员登录密码", "SUCCESS")
	}

	return nil
}

func (s *AuthService) GetAdminInfo(ctx context.Context, username string) (*domain.AdminUser, error) {
	admin, err := s.adminRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return admin, nil
}

func (s *AuthService) Setup2FA(ctx context.Context, username string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "XrayPanel",
		AccountName: username,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *AuthService) Enable2FA(ctx context.Context, username, secret, passcode, clientIP string) error {
	if !totp.Validate(passcode, secret) {
		return ErrInvalid2FACode
	}

	admin, err := s.adminRepo.GetByUsername(ctx, username)
	if err != nil {
		return ErrUserNotFound
	}

	admin.TOTPSecret = secret
	admin.TOTPEnabled = true
	admin.UpdatedAt = time.Now()
	if err := s.adminRepo.Update(ctx, admin); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, admin.Username, clientIP, domain.ActionAuth2FAEnable, admin.Username, "启用二次验证 (2FA)", "SUCCESS")
	}

	return nil
}

func (s *AuthService) Disable2FA(ctx context.Context, username, password, passcode, clientIP string) error {
	admin, err := s.adminRepo.GetByUsername(ctx, username)
	if err != nil {
		return ErrUserNotFound
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		return ErrIncorrectPassword
	}

	if !admin.TOTPEnabled {
		return Err2FANotEnabled
	}

	if admin.TOTPSecret == "" || !totp.Validate(passcode, admin.TOTPSecret) {
		return ErrInvalid2FACode
	}

	admin.TOTPEnabled = false
	admin.TOTPSecret = ""
	admin.UpdatedAt = time.Now()
	if err := s.adminRepo.Update(ctx, admin); err != nil {
		return err
	}

	if s.auditSvc != nil {
		_ = s.auditSvc.Record(ctx, admin.Username, clientIP, domain.ActionAuth2FADisable, admin.Username, "禁用二次验证 (2FA)", "SUCCESS")
	}

	return nil
}
