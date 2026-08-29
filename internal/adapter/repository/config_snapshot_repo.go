package repository

import (
	"context"
	"time"

	"panel/internal/domain"

	"gorm.io/gorm"
)

type GormConfigSnapshotRepository struct {
	db *gorm.DB
}

func NewGormConfigSnapshotRepository(db *gorm.DB) *GormConfigSnapshotRepository {
	return &GormConfigSnapshotRepository{db: db}
}

func (r *GormConfigSnapshotRepository) Save(ctx context.Context, snapshot *domain.ConfigSnapshot) error {
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(snapshot).Error
}

func (r *GormConfigSnapshotRepository) List(ctx context.Context, limit int) ([]domain.ConfigSnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []domain.ConfigSnapshot
	err := r.db.WithContext(ctx).
		Select("id, remark, created_at").
		Order("id desc").
		Limit(limit).
		Find(&list).Error
	return list, err
}

func (r *GormConfigSnapshotRepository) GetByID(ctx context.Context, id uint) (*domain.ConfigSnapshot, error) {
	var item domain.ConfigSnapshot
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}
