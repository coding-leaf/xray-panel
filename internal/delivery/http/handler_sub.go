package http

import (
	"fmt"
	"net/http"

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
		c.String(http.StatusBadRequest, "token is required")
		return
	}

	tagFilter := c.Query("tag")

	payload, err := h.subSvc.GetSubscriptionByToken(c.Request.Context(), token, tagFilter)
	if err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}

	// 注入 Subscription-Userinfo 标准响应头
	if payload.RemainingBytes >= 0 {
		c.Header("Subscription-Userinfo", fmt.Sprintf("total=%d; expire=%d", payload.RemainingBytes, payload.ExpireTime/1000))
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Profile-Update-Interval", "24")

	// 返回 Base64 编码的标准节点订阅
	c.String(http.StatusOK, payload.Base64Data)
}
