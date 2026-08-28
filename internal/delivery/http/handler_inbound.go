package http

import (
	"net/http"
	"strconv"

	"panel/internal/domain"
	"panel/internal/service"

	"github.com/gin-gonic/gin"
)

type InboundHandler struct {
	configSvc *service.ConfigService
}

func NewInboundHandler(configSvc *service.ConfigService) *InboundHandler {
	return &InboundHandler{configSvc: configSvc}
}

func (h *InboundHandler) List(c *gin.Context) {
	inbounds, err := h.configSvc.ListInbounds(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, inbounds)
}

func (h *InboundHandler) Create(c *gin.Context) {
	var in domain.Inbound
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.configSvc.CreateInbound(c.Request.Context(), &in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, in)
}

func (h *InboundHandler) Update(c *gin.Context) {
	var in domain.Inbound
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	in.ID = uint(id)

	if err := h.configSvc.UpdateInbound(c.Request.Context(), &in); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, in)
}

func (h *InboundHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid inbound id"})
		return
	}

	if err := h.configSvc.DeleteInbound(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "inbound deleted"})
}
