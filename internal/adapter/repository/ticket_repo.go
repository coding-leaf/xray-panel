package repository

import (
	"context"
	"errors"
	"time"

	"panel/internal/domain"
	"gorm.io/gorm"
)

type GORMTicketRepository struct {
	db *gorm.DB
}

var _ domain.TicketRepository = (*GORMTicketRepository)(nil)

func NewTicketRepository(db *gorm.DB) *GORMTicketRepository {
	return &GORMTicketRepository{db: db}
}

func (r *GORMTicketRepository) Create(ctx context.Context, ticket *domain.Ticket) error {
	return r.db.WithContext(ctx).Create(ticket).Error
}

func (r *GORMTicketRepository) GetByCode(ctx context.Context, code string) (*domain.Ticket, error) {
	var ticket domain.Ticket
	err := r.db.WithContext(ctx).Preload("User").Where("code = ?", code).First(&ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &ticket, err
}

func (r *GORMTicketRepository) ConsumeAtomic(ctx context.Context, code string) (*domain.Ticket, error) {
	now := time.Now().Unix()
	result := r.db.WithContext(ctx).Model(&domain.Ticket{}).
		Where("code = ? AND remaining_uses > 0 AND expires_at > ?", code, now).
		Update("remaining_uses", gorm.Expr("remaining_uses - 1"))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrTicketInvalidOrExpired
	}

	var ticket domain.Ticket
	if err := r.db.WithContext(ctx).Preload("User").Where("code = ?", code).First(&ticket).Error; err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (r *GORMTicketRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&domain.Ticket{}, id).Error
}

func (r *GORMTicketRepository) DeleteByUserID(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.Ticket{}).Error
}

func (r *GORMTicketRepository) CleanExpired(ctx context.Context) error {
	now := time.Now().Unix()
	return r.db.WithContext(ctx).Where("expires_at <= ? OR remaining_uses <= 0", now).Delete(&domain.Ticket{}).Error
}
