package domain

import (
	"context"
	"time"
)

type ConfigSnapshot struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RawConfig string    `gorm:"type:text;not null" json:"rawConfig"`
	Remark    string    `gorm:"type:varchar(255)" json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
}

type ConfigSnapshotRepository interface {
	Save(ctx context.Context, snapshot *ConfigSnapshot) error
	List(ctx context.Context, limit int) ([]ConfigSnapshot, error)
	GetByID(ctx context.Context, id uint) (*ConfigSnapshot, error)
}
