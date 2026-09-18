package repository

import (
	"context"
	"errors"
	"time"

	"panel/internal/domain"
	"gorm.io/gorm"
)

type GORMUserRepository struct {
	db *gorm.DB
}

var (
	_ domain.UserRepository        = (*GORMUserRepository)(nil)
	_ domain.TrafficBatchRepository = (*GORMUserRepository)(nil)
)

func NewUserRepository(db *gorm.DB) *GORMUserRepository {
	return &GORMUserRepository{db: db}
}

func (r *GORMUserRepository) Create(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *GORMUserRepository) Update(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Model(user).Omit("up_bytes", "down_bytes").Save(user).Error
}

func (r *GORMUserRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.User{}, id).Error
}

func (r *GORMUserRepository) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func (r *GORMUserRepository) GetByUUID(ctx context.Context, uuid string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func (r *GORMUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func (r *GORMUserRepository) GetBySubToken(ctx context.Context, token string) (*domain.User, error) {
	var user domain.User
	err := r.db.WithContext(ctx).Where("sub_token = ?", token).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func (r *GORMUserRepository) UpdateFields(ctx context.Context, id uint, values map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", id).Updates(values).Error
}

func (r *GORMUserRepository) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	var candidates []domain.User
	err := r.db.WithContext(ctx).Where("inbound_tag = ? OR inbound_tags LIKE ?", tag, "%"+tag+"%").Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	matched := make([]domain.User, 0, len(candidates))
	for _, u := range candidates {
		if u.HasInbound(tag) {
			matched = append(matched, u)
		}
	}
	return matched, nil
}

func (r *GORMUserRepository) ListAll(ctx context.Context) ([]domain.User, error) {
	var users []domain.User
	err := r.db.WithContext(ctx).Find(&users).Error
	return users, err
}

func (r *GORMUserRepository) AddTraffic(ctx context.Context, email string, upBytes, downBytes int64) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("email = ?", email).
		Updates(map[string]interface{}{
			"up_bytes":   gorm.Expr("up_bytes + ?", upBytes),
			"down_bytes": gorm.Expr("down_bytes + ?", downBytes),
			"updated_at": time.Now(),
		}).Error
}

func (r *GORMUserRepository) ResetTraffic(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"up_bytes":   0,
			"down_bytes": 0,
			"updated_at": time.Now(),
		}).Error
}

// BatchSyncTraffic 在单一原子事务中批量累加用户和入站流量增量，并记录每日流量日志
func (r *GORMUserRepository) BatchSyncTraffic(
	ctx context.Context,
	userDeltas []domain.UserTrafficDelta,
	inboundDeltas []domain.InboundTrafficDelta,
	date string,
) ([]domain.User, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	now := time.Now()
	var updatedUsers []domain.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 批量累加用户流量
		emails := make([]string, 0, len(userDeltas))
		deltaMap := make(map[string]domain.UserTrafficDelta, len(userDeltas))
		for _, d := range userDeltas {
			if d.Up <= 0 && d.Down <= 0 {
				continue
			}
			emails = append(emails, d.Email)
			deltaMap[d.Email] = d
			if err := tx.Model(&domain.User{}).
				Where("email = ?", d.Email).
				Updates(map[string]interface{}{
					"up_bytes":   gorm.Expr("up_bytes + ?", d.Up),
					"down_bytes": gorm.Expr("down_bytes + ?", d.Down),
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}

		// 2. 查询受影响的最新用户信息并记录每日 TrafficLog
		if len(emails) > 0 {
			if err := tx.Where("email IN ?", emails).Find(&updatedUsers).Error; err != nil {
				return err
			}

			for _, u := range updatedUsers {
				d := deltaMap[u.Email]
				var log domain.TrafficLog
				err := tx.Where("user_email = ? AND date = ?", u.Email, date).First(&log).Error
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						newLog := domain.TrafficLog{
							UserID:    u.ID,
							UserEmail: u.Email,
							UpBytes:   d.Up,
							DownBytes: d.Down,
							Date:      date,
							CreatedAt: now,
						}
						if createErr := tx.Create(&newLog).Error; createErr != nil {
							if updateErr := tx.Model(&domain.TrafficLog{}).
								Where("user_email = ? AND date = ?", u.Email, date).
								Updates(map[string]interface{}{
									"up_bytes":   gorm.Expr("up_bytes + ?", d.Up),
									"down_bytes": gorm.Expr("down_bytes + ?", d.Down),
									"user_id":    u.ID,
								}).Error; updateErr != nil {
								return updateErr
							}
						}
					} else {
						return err
					}
				} else {
					if updateErr := tx.Model(&log).Updates(map[string]interface{}{
						"up_bytes":   gorm.Expr("up_bytes + ?", d.Up),
						"down_bytes": gorm.Expr("down_bytes + ?", d.Down),
						"user_id":    u.ID,
					}).Error; updateErr != nil {
						return updateErr
					}
				}
			}
		}

		// 3. 批量累加入站流量
		for _, d := range inboundDeltas {
			if d.Up <= 0 && d.Down <= 0 {
				continue
			}
			if err := tx.Model(&domain.Inbound{}).
				Where("tag = ?", d.Tag).
				Updates(map[string]interface{}{
					"up_bytes":   gorm.Expr("up_bytes + ?", d.Up),
					"down_bytes": gorm.Expr("down_bytes + ?", d.Down),
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return updatedUsers, nil
}
