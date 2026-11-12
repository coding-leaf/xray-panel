package repository

import (
	"context"
	"time"

	"panel/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormTrafficLogRepository struct {
	db *gorm.DB
}

func NewGormTrafficLogRepository(db *gorm.DB) *GormTrafficLogRepository {
	return &GormTrafficLogRepository{db: db}
}

func trafficLogOnConflictClause() clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{{Name: "user_email"}, {Name: "date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"up_bytes":   gorm.Expr("traffic_logs.up_bytes + excluded.up_bytes"),
			"down_bytes": gorm.Expr("traffic_logs.down_bytes + excluded.down_bytes"),
			"user_id":    gorm.Expr("excluded.user_id"),
		}),
	}
}

func (r *GormTrafficLogRepository) RecordTraffic(ctx context.Context, userID uint, email string, upBytes, downBytes int64, date string) error {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	newLog := domain.TrafficLog{
		UserID:    userID,
		UserEmail: email,
		UpBytes:   upBytes,
		DownBytes: downBytes,
		Date:      date,
		CreatedAt: time.Now(),
	}
	return r.db.WithContext(ctx).
		Clauses(trafficLogOnConflictClause()).
		Create(&newLog).Error
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

// DeleteByUser 删除指定用户关联的全部每日流量日志（按 user_id 或 user_email 匹配）
func (r *GormTrafficLogRepository) DeleteByUser(ctx context.Context, userID uint, email string) error {
	if userID == 0 && email == "" {
		return nil
	}
	q := r.db.WithContext(ctx)
	switch {
	case userID != 0 && email != "":
		q = q.Where("user_id = ? OR user_email = ?", userID, email)
	case userID != 0:
		q = q.Where("user_id = ?", userID)
	default:
		q = q.Where("user_email = ?", email)
	}
	return q.Delete(&domain.TrafficLog{}).Error
}

// PruneBefore 删除早于指定日期 (YYYY-MM-DD) 的历史流量日志
func (r *GormTrafficLogRepository) PruneBefore(ctx context.Context, beforeDate string) error {
	if beforeDate == "" {
		return nil
	}
	return r.db.WithContext(ctx).Where("date < ?", beforeDate).Delete(&domain.TrafficLog{}).Error
}
