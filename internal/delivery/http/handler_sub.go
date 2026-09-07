package http

import (
	"fmt"
	"net/http"
	"strings"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type SubHandler struct {
	subSvc *service.SubService
}

func NewSubHandler(subSvc *service.SubService) *SubHandler {
	return &SubHandler{subSvc: subSvc}
}

func (h *SubHandler) GetSubscription(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		token = c.Query("token")
	}
	if token == "" {
		c.String(http.StatusBadRequest, "token is required")
		return
	}

	tagFilter := c.Query("tag")
	reqHost := c.Request.Host
	format := c.Query("format")
	if format == "" {
		format = c.Query("type")
	}

	payload, output, err := h.subSvc.ExportUserSubscription(c.Request.Context(), token, tagFilter, reqHost, format)
	if err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}

	// 注入 Subscription-Userinfo 标准响应头: upload=X; download=Y; total=Z; expire=E
	var expireSec int64 = 0
	if payload.ExpireTime > 0 {
		expireSec = payload.ExpireTime / 1000
	}
	c.Header("Subscription-Userinfo", fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", payload.UpBytes, payload.DownBytes, payload.TotalBytes, expireSec))

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "clash", "clash-meta", "mihomo":
		c.Header("Content-Type", "application/yaml; charset=utf-8")
	case "sing-box", "singbox":
		c.Header("Content-Type", "application/json; charset=utf-8")
	default:
		c.Header("Content-Type", "text/plain; charset=utf-8")
	}

	c.Header("Profile-Update-Interval", "24")
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	// 返回对应格式的订阅内容
	c.String(http.StatusOK, output)
}
