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
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	cfg, err := h.configSvc.GetDNSConfigByNode(c.Request.Context(), nodeID)
	if err != nil {
		writePerNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *DNSHandler) Save(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	var cfg domain.DNSConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.SaveDNSConfigByNode(c.Request.Context(), nodeID, &cfg); err != nil {
		writePerNodeConfigError(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}
