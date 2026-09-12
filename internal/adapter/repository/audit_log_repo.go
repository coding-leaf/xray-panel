package repository

import (
	"context"
	"sync/atomic"

	"panel/internal/domain"

	"gorm.io/gorm"
)

type GormAuditLogRepository struct {
	db           *gorm.DB
	writeCounter atomic.Int64
}

func NewGormAuditLogRepository(db *gorm.DB) *GormAuditLogRepository {
	return &GormAuditLogRepository{db: db}
}

func (r *GormAuditLogRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return err
	}

	// 内存原子计数器：每累积 100 次写入轻量触发一次修剪，杜绝每次写都 COUNT/DELETE 造成的 I/O 放大
	if r.writeCounter.Add(1)%100 == 0 {
		_ = r.Prune(ctx, 3000)
	}

	return nil
}

func (r *GormAuditLogRepository) Prune(ctx context.Context, maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}

	// 利用主键索引裁剪超出 3000 条的旧日志
	subQuery := r.db.Model(&domain.AuditLog{}).Select("id").Order("id DESC").Offset(maxKeep).Limit(1)
	return r.db.WithContext(ctx).Where("id <= (?)", subQuery).Delete(&domain.AuditLog{}).Error
}

func (r *GormAuditLogRepository) List(ctx context.Context, offset, limit int, action, operator, keyword string) ([]domain.AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&domain.AuditLog{})
	if action != "" {
		q = q.Where("action = ?", action)
	}
	if operator != "" {
		q = q.Where("operator = ?", operator)
	}
	if keyword != "" {
		likeKw := "%" + keyword + "%"
		q = q.Where("target LIKE ? OR details LIKE ? OR ip LIKE ?", likeKw, likeKw, likeKw)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var logs []domain.AuditLog
	err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

func (r *GormAuditLogRepository) Clear(ctx context.Context) error {
	return r.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&domain.AuditLog{}).Error
}
