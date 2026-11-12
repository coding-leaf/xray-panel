package repository

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"panel/internal/domain"

	"gorm.io/gorm"
)

const (
	snapshotMaxKeep    = 100
	snapshotPruneEvery = 20
)

type GormConfigSnapshotRepository struct {
	db         *gorm.DB
	writeCount atomic.Uint64
}

func NewGormConfigSnapshotRepository(db *gorm.DB) *GormConfigSnapshotRepository {
	return &GormConfigSnapshotRepository{db: db}
}

func (r *GormConfigSnapshotRepository) Save(ctx context.Context, snapshot *domain.ConfigSnapshot) error {
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(snapshot).Error; err != nil {
		return err
	}

	// 原子写计数器：每累积 snapshotPruneEvery 次写入轻量触发一次裁剪，保留最新 snapshotMaxKeep 条快照
	if r.writeCount.Add(1)%snapshotPruneEvery == 0 {
		if err := r.Prune(ctx, snapshotMaxKeep); err != nil {
			slog.Warn("failed to auto-prune config snapshots", slog.Any("error", err))
		}
	}

	return nil
}

// Prune 按主键倒序保留最新 maxKeep 条快照，清理其余旧快照
func (r *GormConfigSnapshotRepository) Prune(ctx context.Context, maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}

	subQuery := r.db.Model(&domain.ConfigSnapshot{}).Select("id").Order("id DESC").Offset(maxKeep).Limit(1)
	return r.db.WithContext(ctx).Where("id <= (?)", subQuery).Delete(&domain.ConfigSnapshot{}).Error
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
