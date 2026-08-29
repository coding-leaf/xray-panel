package http

import (
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type DNSHandler struct {
	configSvc *service.ConfigService
}

func NewDNSHandler(configSvc *service.ConfigService) *DNSHandler {
	return &DNSHandler{configSvc: configSvc}
}

func (h *DNSHandler) Get(c *gin.Context) {
	cfg, err := h.configSvc.GetDNSConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *DNSHandler) Save(c *gin.Context) {
	var cfg domain.DNSConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.SaveDNSConfig(c.Request.Context(), &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}
