package http

import (
	"fmt"
	"net/http"
	"strconv"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userSvc *service.UserService
	subSvc  *service.SubService
}

func NewUserHandler(userSvc *service.UserService, subSvc *service.SubService) *UserHandler {
	return &UserHandler{
		userSvc: userSvc,
		subSvc:  subSvc,
	}
}

func (h *UserHandler) List(c *gin.Context) {
	users, err := h.userSvc.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.userSvc.CreateUser(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func (h *UserHandler) Update(c *gin.Context) {
	var user domain.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	user.ID = uint(id)

	if err := h.userSvc.UpdateUser(c.Request.Context(), &user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.userSvc.DeleteUser(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

func (h *UserHandler) ResetTraffic(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.userSvc.ResetTraffic(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "traffic reset successfully"})
}

func (h *UserHandler) GetShareLink(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, c.Request.Host)

	shareInfo, err := h.subSvc.GetUserShareInfo(c.Request.Context(), uint(id), baseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, shareInfo)
}

func (h *UserHandler) GetTrafficHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	daysStr := c.DefaultQuery("days", "30")
	days, _ := strconv.Atoi(daysStr)
	if days <= 0 {
		days = 30
	}

	history, err := h.userSvc.GetTrafficHistory(c.Request.Context(), uint(id), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

func (h *UserHandler) ResetToken(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	newToken, err := h.userSvc.ResetSubToken(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"subToken": newToken, "message": "订阅 Token 重置成功"})
}

type BatchRenewRequest struct {
	IDs  []uint `json:"ids" binding:"required"`
	Days int    `json:"days" binding:"required"`
}

func (h *UserHandler) BatchRenew(c *gin.Context) {
	var req BatchRenewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userSvc.BatchRenew(c.Request.Context(), req.IDs, req.Days); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功为 %d 位用户延期 %d 天", len(req.IDs), req.Days)})
}

type BatchIDsRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

func (h *UserHandler) BatchResetTraffic(c *gin.Context) {
	var req BatchIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userSvc.BatchResetTraffic(c.Request.Context(), req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功重置 %d 位用户的已用流量", len(req.IDs))})
}

type BatchStatusRequest struct {
	IDs     []uint `json:"ids" binding:"required"`
	Enabled bool   `json:"enabled"`
}

func (h *UserHandler) BatchStatus(c *gin.Context) {
	var req BatchStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.userSvc.BatchSetStatus(c.Request.Context(), req.IDs, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	action := "启用"
	if !req.Enabled {
		action = "禁用"
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("成功%s %d 位用户", action, len(req.IDs))})
}
