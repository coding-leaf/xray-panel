package http

import (
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type GeoDataHandler struct {
	geoSvc *service.GeoDataService
}

func NewGeoDataHandler(geoSvc *service.GeoDataService) *GeoDataHandler {
	return &GeoDataHandler{geoSvc: geoSvc}
}

func (h *GeoDataHandler) GetStatus(c *gin.Context) {
	status, err := h.geoSvc.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *GeoDataHandler) UpdateGeoData(c *gin.Context) {
	if err := h.geoSvc.UpdateGeoData(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "GeoData 规则库已成功更新并重载生效"})
}
