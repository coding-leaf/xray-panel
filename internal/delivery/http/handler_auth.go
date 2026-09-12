package http

import (
	"net/http"
	"time"

	"panel/internal/delivery/http/middleware"
	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	adminRepo domain.AdminRepository
	jwtSecret string
	auditSvc  *service.AuditLogService
}

func NewAuthHandler(adminRepo domain.AdminRepository, jwtSecret string, auditSvc ...*service.AuditLogService) *AuthHandler {
	h := &AuthHandler{
		adminRepo: adminRepo,
		jwtSecret: jwtSecret,
	}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Passcode string `json:"passcode"` // TOTP code if enabled
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	admin, err := h.adminRepo.GetByUsername(c.Request.Context(), req.Username)
	if err != nil {
		_ = h.auditSvc.Record(c.Request.Context(), req.Username, c.ClientIP(), domain.ActionAuthLoginFailed, req.Username, "登录失败: 用户名不存在", "FAILED")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		_ = h.auditSvc.Record(c.Request.Context(), req.Username, c.ClientIP(), domain.ActionAuthLoginFailed, req.Username, "登录失败: 密码错误", "FAILED")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	// 2FA 校验
	if admin.TOTPEnabled {
		if req.Passcode == "" || !totp.Validate(req.Passcode, admin.TOTPSecret) {
			_ = h.auditSvc.Record(c.Request.Context(), req.Username, c.ClientIP(), domain.ActionAuthLoginFailed, req.Username, "登录失败: 2FA 动态码错误", "FAILED")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2fa passcode", "require2fa": true})
			return
		}
	}

	token, err := middleware.GenerateToken(admin.Username, h.jwtSecret, 7*24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate token failed"})
		return
	}

	_ = h.auditSvc.Record(c.Request.Context(), admin.Username, c.ClientIP(), domain.ActionAuthLogin, admin.Username, "管理员登录成功", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{
		"token":       token,
		"username":    admin.Username,
		"totpEnabled": admin.TOTPEnabled,
	})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, _ := c.Get("username")
	admin, err := h.adminRepo.GetByUsername(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect old password"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password failed"})
		return
	}

	admin.PasswordHash = string(newHash)
	admin.UpdatedAt = time.Now()
	if err := h.adminRepo.Update(c.Request.Context(), admin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update password failed"})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionAuthPassword, admin.Username, "修改管理员登录密码", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}

func (h *AuthHandler) GetAdminInfo(c *gin.Context) {
	username, _ := c.Get("username")
	admin, err := h.adminRepo.GetByUsername(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"username":    admin.Username,
		"totpEnabled": admin.TOTPEnabled,
	})
}

func (h *AuthHandler) Setup2FA(c *gin.Context) {
	username, _ := c.Get("username")
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "XrayPanel",
		AccountName: username.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate 2fa key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"secret":     key.Secret(),
		"otpauthUrl": key.URL(),
	})
}

type Enable2FARequest struct {
	Secret   string `json:"secret" binding:"required"`
	Passcode string `json:"passcode" binding:"required"`
}

func (h *AuthHandler) Enable2FA(c *gin.Context) {
	var req Enable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !totp.Validate(req.Passcode, req.Secret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 2fa verification code"})
		return
	}

	username, _ := c.Get("username")
	admin, err := h.adminRepo.GetByUsername(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	admin.TOTPSecret = req.Secret
	admin.TOTPEnabled = true
	admin.UpdatedAt = time.Now()
	if err := h.adminRepo.Update(c.Request.Context(), admin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable 2fa"})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionAuth2FAEnable, admin.Username, "启用二次验证 (2FA)", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "2fa enabled successfully", "totpEnabled": true})
}

type Disable2FARequest struct {
	Password string `json:"password" binding:"required"`
	Passcode string `json:"passcode" binding:"required,len=6"`
}

func (h *AuthHandler) Disable2FA(c *gin.Context) {
	var req Disable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	username, _ := c.Get("username")
	admin, err := h.adminRepo.GetByUsername(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect password"})
		return
	}

	if !admin.TOTPEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2fa is not enabled"})
		return
	}

	if admin.TOTPSecret == "" || !totp.Validate(req.Passcode, admin.TOTPSecret) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 2fa passcode"})
		return
	}

	admin.TOTPEnabled = false
	admin.TOTPSecret = ""
	admin.UpdatedAt = time.Now()
	if err := h.adminRepo.Update(c.Request.Context(), admin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable 2fa"})
		return
	}

	h.auditSvc.RecordFromGin(c, domain.ActionAuth2FADisable, admin.Username, "禁用二次验证 (2FA)", "SUCCESS")

	c.JSON(http.StatusOK, gin.H{"message": "2fa disabled successfully", "totpEnabled": false})
}
