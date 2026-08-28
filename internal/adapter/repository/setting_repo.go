package repository

import (
	"context"
	"errors"
	"time"

	"panel/internal/domain"
	"gorm.io/gorm"
)

type GORMSettingRepository struct {
	db *gorm.DB
}

func NewSettingRepository(db *gorm.DB) *GORMSettingRepository {
	return &GORMSettingRepository{db: db}
}

func (r *GORMSettingRepository) Get(ctx context.Context, key string) (string, error) {
	var s domain.Setting
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", domain.ErrNotFound
	}
	return s.Value, err
}

func (r *GORMSettingRepository) Set(ctx context.Context, key, value string) error {
	s := domain.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Save(&s).Error
}

func (r *GORMSettingRepository) GetAll(ctx context.Context) (map[string]string, error) {
	var list []domain.Setting
	if err := r.db.WithContext(ctx).Find(&list).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string)
	for _, item := range list {
		m[item.Key] = item.Value
	}
	return m, nil
}

type GORMAdminRepository struct {
	db *gorm.DB
}

func NewAdminRepository(db *gorm.DB) *GORMAdminRepository {
	return &GORMAdminRepository{db: db}
}

func (r *GORMAdminRepository) GetByUsername(ctx context.Context, username string) (*domain.AdminUser, error) {
	var admin domain.AdminUser
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&admin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &admin, err
}

func (r *GORMAdminRepository) Update(ctx context.Context, admin *domain.AdminUser) error {
	return r.db.WithContext(ctx).Save(admin).Error
}

func (r *GORMAdminRepository) Create(ctx context.Context, admin *domain.AdminUser) error {
	return r.db.WithContext(ctx).Create(admin).Error
}
