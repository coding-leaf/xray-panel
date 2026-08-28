package http

import (
	"net/http"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type RoutingHandler struct {
	configSvc *service.ConfigService
}

func NewRoutingHandler(configSvc *service.ConfigService) *RoutingHandler {
	return &RoutingHandler{configSvc: configSvc}
}

func (h *RoutingHandler) Get(c *gin.Context) {
	cfg, err := h.configSvc.GetRoutingConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func (h *RoutingHandler) Save(c *gin.Context) {
	var cfg domain.RoutingConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.SaveRoutingConfig(c.Request.Context(), &cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}
