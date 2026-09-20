package http

import (
	"errors"
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authSvc: authSvc,
	}
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

	res, err := h.authSvc.Login(c.Request.Context(), req.Username, req.Password, req.Passcode, c.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrRequire2FA) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2fa passcode", "require2fa": true})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":       res.Token,
		"username":    res.Username,
		"totpEnabled": res.TOTPEnabled,
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
	err := h.authSvc.ChangePassword(c.Request.Context(), username.(string), req.OldPassword, req.NewPassword, c.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if errors.Is(err, service.ErrIncorrectOldPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect old password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update password failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}

func (h *AuthHandler) GetAdminInfo(c *gin.Context) {
	username, _ := c.Get("username")
	admin, err := h.authSvc.GetAdminInfo(c.Request.Context(), username.(string))
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
	secret, url, err := h.authSvc.Setup2FA(c.Request.Context(), username.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate 2fa key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"secret":     secret,
		"otpauthUrl": url,
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

	username, _ := c.Get("username")
	err := h.authSvc.Enable2FA(c.Request.Context(), username.(string), req.Secret, req.Passcode, c.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrInvalid2FACode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 2fa verification code"})
			return
		}
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable 2fa"})
		return
	}

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
	err := h.authSvc.Disable2FA(c.Request.Context(), username.(string), req.Password, req.Passcode, c.ClientIP())
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if errors.Is(err, service.ErrIncorrectPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "incorrect password"})
			return
		}
		if errors.Is(err, service.Err2FANotEnabled) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "2fa is not enabled"})
			return
		}
		if errors.Is(err, service.ErrInvalid2FACode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 2fa passcode"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable 2fa"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "2fa disabled successfully", "totpEnabled": false})
}
