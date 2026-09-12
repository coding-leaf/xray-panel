package service

import (
	"context"
	"time"

	"panel/internal/domain"

	"github.com/gin-gonic/gin"
)

type AuditLogService struct {
	repo domain.AuditLogRepository
}

func NewAuditLogService(repo domain.AuditLogRepository) *AuditLogService {
	return &AuditLogService{repo: repo}
}

// RecordFromGin 提供单行安全审计助手方法，具备完整 nil-safe 保护（未注入或测试环境下绝不 panic）
func (s *AuditLogService) RecordFromGin(c *gin.Context, action, target, details, status string) {
	if s == nil || s.repo == nil || c == nil {
		return
	}

	operator := "admin"
	if u, exists := c.Get("username"); exists {
		if str, ok := u.(string); ok && str != "" {
			operator = str
		}
	}

	ip := c.ClientIP()
	if ip == "" {
		ip = "127.0.0.1"
	}

	if status == "" {
		status = "SUCCESS"
	}

	log := &domain.AuditLog{
		CreatedAt: time.Now(),
		Operator:  operator,
		IP:        ip,
		Action:    action,
		Target:    target,
		Details:   details,
		Status:    status,
	}

	// 契合 SQLite SetMaxOpenConns(1) 单连接拓扑：同请求同步安全写入（<0.2ms）
	_ = s.repo.Create(c.Request.Context(), log)
}

func (s *AuditLogService) Record(ctx context.Context, operator, ip, action, target, details, status string) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if status == "" {
		status = "SUCCESS"
	}

	log := &domain.AuditLog{
		CreatedAt: time.Now(),
		Operator:  operator,
		IP:        ip,
		Action:    action,
		Target:    target,
		Details:   details,
		Status:    status,
	}
	return s.repo.Create(ctx, log)
}

func (s *AuditLogService) List(ctx context.Context, page, pageSize int, action, operator, keyword string) ([]domain.AuditLog, int64, error) {
	if s == nil || s.repo == nil {
		return []domain.AuditLog{}, 0, nil
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize, action, operator, keyword)
}

func (s *AuditLogService) Clear(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.Clear(ctx)
}
