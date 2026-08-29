package repository

import (
	"context"
	"errors"
	"time"

	"panel/internal/domain"
	"gorm.io/gorm"
)

type GORMInboundRepository struct {
	db *gorm.DB
}

func NewInboundRepository(db *gorm.DB) *GORMInboundRepository {
	return &GORMInboundRepository{db: db}
}

func (r *GORMInboundRepository) Create(ctx context.Context, inbound *domain.Inbound) error {
	return r.db.WithContext(ctx).Create(inbound).Error
}

func (r *GORMInboundRepository) Update(ctx context.Context, inbound *domain.Inbound) error {
	return r.db.WithContext(ctx).Save(inbound).Error
}

func (r *GORMInboundRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Inbound{}, id).Error
}

func (r *GORMInboundRepository) GetByID(ctx context.Context, id uint) (*domain.Inbound, error) {
	var inbound domain.Inbound
	err := r.db.WithContext(ctx).First(&inbound, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &inbound, err
}

func (r *GORMInboundRepository) GetByTag(ctx context.Context, tag string) (*domain.Inbound, error) {
	var inbound domain.Inbound
	err := r.db.WithContext(ctx).Where("tag = ?", tag).First(&inbound).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &inbound, err
}

func (r *GORMInboundRepository) ListAll(ctx context.Context) ([]domain.Inbound, error) {
	var inbounds []domain.Inbound
	err := r.db.WithContext(ctx).Find(&inbounds).Error
	return inbounds, err
}

func (r *GORMInboundRepository) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return r.db.WithContext(ctx).Model(&domain.Inbound{}).
		Where("tag = ?", tag).
		Updates(map[string]interface{}{
			"up_bytes":   gorm.Expr("up_bytes + ?", upBytes),
			"down_bytes": gorm.Expr("down_bytes + ?", downBytes),
			"updated_at": time.Now(),
		}).Error
}
