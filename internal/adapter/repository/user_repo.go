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

func NewUserRepository(db *gorm.DB) *GORMUserRepository {
	return &GORMUserRepository{db: db}
}

func (r *GORMUserRepository) Create(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *GORMUserRepository) Update(ctx context.Context, user *domain.User) error {
	return r.db.WithContext(ctx).Save(user).Error
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

func (r *GORMUserRepository) ListByInboundTag(ctx context.Context, tag string) ([]domain.User, error) {
	var users []domain.User
	err := r.db.WithContext(ctx).Where("inbound_tag = ? OR inbound_tags LIKE ?", tag, "%"+tag+"%").Find(&users).Error
	return users, err
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
