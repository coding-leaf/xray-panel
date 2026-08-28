package domain

import "time"

type Setting struct {
	Key       string    `gorm:"primaryKey;type:varchar(64)" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type AdminUser struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	TOTPSecret   string    `gorm:"type:varchar(64);default:''" json:"-"`
	TOTPEnabled  bool      `gorm:"default:false" json:"totpEnabled"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type SubscriptionPayload struct {
	NodesRaw       string `json:"nodesRaw"` // Multi-line vless://... strings
	Base64Data     string `json:"base64Data"`
	UserEmail      string `json:"userEmail"`
	UpBytes        int64  `json:"upBytes"`
	DownBytes      int64  `json:"downBytes"`
	TotalBytes     int64  `json:"totalBytes"`
	RemainingBytes int64  `json:"remainingBytes"`
	ExpireTime     int64  `json:"expireTime"`
}
