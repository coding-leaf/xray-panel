package repository

import (
	"context"
	"time"

	"panel/internal/domain"

	"gorm.io/gorm"
)

type GormTrafficLogRepository struct {
	db *gorm.DB
}

func NewGormTrafficLogRepository(db *gorm.DB) *GormTrafficLogRepository {
	return &GormTrafficLogRepository{db: db}
}

func (r *GormTrafficLogRepository) RecordTraffic(ctx context.Context, userID uint, email string, upBytes, downBytes int64, date string) error {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	var log domain.TrafficLog
	err := r.db.WithContext(ctx).
		Where("user_email = ? AND date = ?", email, date).
		First(&log).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			newLog := domain.TrafficLog{
				UserID:    userID,
				UserEmail: email,
				UpBytes:   upBytes,
				DownBytes: downBytes,
				Date:      date,
				CreatedAt: time.Now(),
			}
			createErr := r.db.WithContext(ctx).Create(&newLog).Error
			if createErr == nil {
				return nil
			}
			// 若并发写入命中唯一索引冲突，安全回退为累加更新
			return r.db.WithContext(ctx).Model(&domain.TrafficLog{}).
				Where("user_email = ? AND date = ?", email, date).
				Updates(map[string]interface{}{
					"up_bytes":   gorm.Expr("up_bytes + ?", upBytes),
					"down_bytes": gorm.Expr("down_bytes + ?", downBytes),
					"user_id":    userID,
				}).Error
		}
		return err
	}

	return r.db.WithContext(ctx).Model(&log).Updates(map[string]interface{}{
		"up_bytes":   gorm.Expr("up_bytes + ?", upBytes),
		"down_bytes": gorm.Expr("down_bytes + ?", downBytes),
		"user_id":    userID,
	}).Error
}

func (r *GormTrafficLogRepository) GetHistoryByUserID(ctx context.Context, userID uint, days int) ([]domain.TrafficLog, error) {
	if days <= 0 {
		days = 30
	}
	var logs []domain.TrafficLog
	err := r.db.WithContext(ctx).
		Where("user_id = ? OR user_email = (SELECT email FROM users WHERE id = ?)", userID, userID).
		Order("date DESC").
		Limit(days).
		Find(&logs).Error
	return logs, err
}

func (r *GormTrafficLogRepository) GetHistoryByEmail(ctx context.Context, email string, days int) ([]domain.TrafficLog, error) {
	if days <= 0 {
		days = 30
	}
	var logs []domain.TrafficLog
	err := r.db.WithContext(ctx).
		Where("user_email = ?", email).
		Order("date DESC").
		Limit(days).
		Find(&logs).Error
	return logs, err
}
