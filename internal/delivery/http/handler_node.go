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

const (
	auditActionNodeCreate      = "NODE_CREATE"
	auditActionNodeUpdate      = "NODE_UPDATE"
	auditActionNodeDelete      = "NODE_DELETE"
	auditActionNodeRotateToken = "NODE_ROTATE_TOKEN"
)

type NodeHandler struct {
	nodeSvc  *service.NodeService
	auditSvc *service.AuditLogService
}

func NewNodeHandler(nodeSvc *service.NodeService, auditSvc ...*service.AuditLogService) *NodeHandler {
	h := &NodeHandler{nodeSvc: nodeSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

func parseUintIDParam(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
		return 0, false
	}
	return uint(id), true
}

func (h *NodeHandler) List(c *gin.Context) {
	nodes, err := h.nodeSvc.ListNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

func (h *NodeHandler) Get(c *gin.Context) {
	id, ok := parseUintIDParam(c, "id")
	if !ok {
		return
	}
	node, err := h.nodeSvc.GetNode(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, node)
}

func (h *NodeHandler) Create(c *gin.Context) {
	var dto domain.CreateNodeDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := h.nodeSvc.CreateNode(c.Request.Context(), dto)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	h.auditSvc.RecordFromGin(c, auditActionNodeCreate, res.Name, fmt.Sprintf("创建节点: %s (%s)", res.Name, res.Host), "SUCCESS")
	c.JSON(http.StatusCreated, res)
}

func (h *NodeHandler) Update(c *gin.Context) {
	id, ok := parseUintIDParam(c, "id")
	if !ok {
		return
	}
	var dto domain.UpdateNodeDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node, err := h.nodeSvc.UpdateNode(c.Request.Context(), id, dto)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, auditActionNodeUpdate, node.Name, fmt.Sprintf("更新节点 ID %d: %s", node.ID, node.Name), "SUCCESS")
	c.JSON(http.StatusOK, node)
}

func (h *NodeHandler) Delete(c *gin.Context) {
	id, ok := parseUintIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.nodeSvc.DeleteNode(c.Request.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		if errors.Is(err, domain.ErrNodeHasInbounds) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditSvc.RecordFromGin(c, auditActionNodeDelete, strconv.FormatUint(uint64(id), 10), fmt.Sprintf("删除节点 ID: %d", id), "SUCCESS")
	c.JSON(http.StatusOK, gin.H{"message": "node deleted"})
}

func (h *NodeHandler) RotateToken(c *gin.Context) {
	id, ok := parseUintIDParam(c, "id")
	if !ok {
		return
	}
	res, err := h.nodeSvc.RotateToken(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	h.auditSvc.RecordFromGin(c, auditActionNodeRotateToken, strconv.FormatUint(uint64(id), 10), fmt.Sprintf("轮换节点 ID %d Token", id), "SUCCESS")
	c.JSON(http.StatusOK, res)
}

func (h *NodeHandler) InstallScript(c *gin.Context) {
	id, ok := parseUintIDParam(c, "id")
	if !ok {
		return
	}

	masterURL := c.Query("masterUrl")
	if masterURL == "" && c.Request.Host != "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		reqHost := c.GetHeader("X-Forwarded-Host")
		if reqHost == "" {
			reqHost = c.Request.Host
		}
		if reqHost != "" {
			masterURL = fmt.Sprintf("%s://%s", scheme, reqHost)
		}
	}

	script, err := h.nodeSvc.RenderInstallScript(c.Request.Context(), id, masterURL)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if c.Query("format") == "raw" {
		c.String(http.StatusOK, script)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"nodeId": id,
		"script": script,
	})
}
