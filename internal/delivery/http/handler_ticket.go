package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type TicketHandler struct {
	ticketSvc *service.TicketService
}

func NewTicketHandler(ticketSvc *service.TicketService) *TicketHandler {
	return &TicketHandler{ticketSvc: ticketSvc}
}

type CreateTicketRequest struct {
	TTLMinutes int `json:"ttl_minutes"` // 默认 15 分钟
	MaxUses    int `json:"max_uses"`    // 默认 2 次
}

type ClaimTicketRequest struct {
	Code string `json:"code" binding:"required"`
}

func (h *TicketHandler) CreateTicket(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.TTLMinutes = 15
		req.MaxUses = 2
	}

	// 动态感知管理员访问所用的 Host 与协议 (支持反代/Cloudflare Worker 标头)
	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	reqHost := c.Request.Header.Get("X-Forwarded-Host")
	if reqHost == "" {
		reqHost = c.Request.Host
	}
	var reqBaseURL string
	if reqHost != "" {
		reqBaseURL = fmt.Sprintf("%s://%s", scheme, reqHost)
	}

	ticket, shareText, err := h.ticketSvc.GenerateTicket(c.Request.Context(), uint(id), req.TTLMinutes, req.MaxUses, reqBaseURL)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if errors.Is(err, domain.ErrUserDisabled) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user is disabled"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":           ticket.Code,
		"user_id":        ticket.UserID,
		"remaining_uses": ticket.RemainingUses,
		"expires_at":     ticket.ExpiresAt,
		"share_text":     shareText,
	})
}

func (h *TicketHandler) ClaimTicket(c *gin.Context) {
	var req ClaimTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "提取码不能为空"})
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || c.Request.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	reqHost := c.Request.Header.Get("X-Forwarded-Host")
	if reqHost == "" {
		reqHost = c.Request.Host
	}
	var reqBaseURL string
	if reqHost != "" {
		reqBaseURL = fmt.Sprintf("%s://%s", scheme, reqHost)
	}

	clientIP := c.ClientIP()
	payload, err := h.ticketSvc.ClaimTicket(c.Request.Context(), req.Code, clientIP, reqBaseURL)
	if err != nil {
		if errors.Is(err, domain.ErrIPRateLimited) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "请求异常频繁，该IP已被临时封禁，请30分钟后再试",
			})
			return
		}
		// 统一模糊错误信息，防止侧信道推断
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "凭据无效、已过期或已被销毁",
		})
		return
	}

	// 关键安全防线：强制禁止浏览器及中间代理缓存节点明文与凭证响应
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	c.JSON(http.StatusOK, payload)
}
