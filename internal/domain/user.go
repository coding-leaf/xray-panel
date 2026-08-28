package domain

import "time"

type User struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID        string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"uuid"`
	Email       string    `gorm:"type:varchar(128);uniqueIndex;not null" json:"email"`
	InboundTag  string    `gorm:"type:varchar(64);not null" json:"inboundTag"`
	Flow        string    `gorm:"type:varchar(32);default:''" json:"flow"`
	SubToken    string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"subToken"`
	TotalBytes  int64     `gorm:"default:0" json:"totalBytes"` // 0 for unlimited
	UpBytes     int64     `gorm:"default:0" json:"upBytes"`
	DownBytes   int64     `gorm:"default:0" json:"downBytes"`
	ExpireTime  int64     `gorm:"default:0" json:"expireTime"` // Unix ms, 0 for never
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (u *User) IsExpired() bool {
	if u.ExpireTime <= 0 {
		return false
	}
	return time.Now().UnixMilli() > u.ExpireTime
}

func (u *User) IsTrafficExceeded() bool {
	if u.TotalBytes <= 0 {
		return false
	}
	return (u.UpBytes + u.DownBytes) >= u.TotalBytes
}

func (u *User) IsActive() bool {
	return u.Enabled && !u.IsExpired() && !u.IsTrafficExceeded()
}
