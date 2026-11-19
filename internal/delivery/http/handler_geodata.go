package http

import (
	"net/http"

	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type GeoDataHandler struct {
	geoSvc  *service.GeoDataService
	nodeSvc *service.NodeService
}

func NewGeoDataHandler(geoSvc *service.GeoDataService) *GeoDataHandler {
	return &GeoDataHandler{geoSvc: geoSvc}
}

// WithNodeService attaches NodeService for on-demand remote node GeoData updates.
func (h *GeoDataHandler) WithNodeService(nodeSvc *service.NodeService) *GeoDataHandler {
	h.nodeSvc = nodeSvc
	return h
}

func (h *GeoDataHandler) GetStatus(c *gin.Context) {
	status, err := h.geoSvc.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *GeoDataHandler) GetProgress(c *gin.Context) {
	progress := h.geoSvc.GetProgress()
	c.JSON(http.StatusOK, progress)
}

func (h *GeoDataHandler) UpdateGeoData(c *gin.Context) {
	nodeID, ok := parseOptionalNodeID(c)
	if !ok {
		return
	}
	if nodeID > 0 {
		if h.nodeSvc == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "node service not configured"})
			return
		}
		resp, err := h.nodeSvc.RequestNodeGeoUpdate(c.Request.Context(), nodeID)
		if err != nil {
			writePerNodeConfigError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, resp)
		return
	}

	if err := h.geoSvc.TriggerUpdate(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "GeoData 规则库更新任务已在后台启动"})
}
