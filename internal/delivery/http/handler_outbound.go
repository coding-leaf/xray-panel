package http

import (
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type OutboundHandler struct {
	configSvc *service.ConfigService
}

func NewOutboundHandler(configSvc *service.ConfigService) *OutboundHandler {
	return &OutboundHandler{configSvc: configSvc}
}

func (h *OutboundHandler) List(c *gin.Context) {
	outbounds, err := h.configSvc.ListOutbounds(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, outbounds)
}

func (h *OutboundHandler) Save(c *gin.Context) {
	var ob domain.Outbound
	if err := c.ShouldBindJSON(&ob); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ob.Tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.SaveOutbound(c.Request.Context(), ob); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ob)
}

func (h *OutboundHandler) Delete(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tag is required"})
		return
	}

	if err := h.configSvc.DeleteOutbound(c.Request.Context(), tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "outbound deleted"})
}
