package domain

import "time"

type Inbound struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Tag            string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"tag"`
	Port           int       `gorm:"not null" json:"port"`
	Listen         string    `gorm:"type:varchar(64);default:'0.0.0.0'" json:"listen"`
	Protocol       string    `gorm:"type:varchar(32);not null" json:"protocol"` // vless, vmess, trojan, shadowsocks
	SettingsJSON   string    `gorm:"type:text" json:"settingsJson"`
	StreamSettings string    `gorm:"type:text" json:"streamSettings"`
	SniffingJSON   string    `gorm:"type:text" json:"sniffingJson"`
	Remark         string    `gorm:"type:varchar(128)" json:"remark"`
	UpBytes        int64     `gorm:"default:0" json:"upBytes"`
	DownBytes      int64     `gorm:"default:0" json:"downBytes"`
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`

	// 内存运行时探测状态 (不入持久化表)
	LatencyMs int64 `gorm:"-" json:"latencyMs"`
	IsAlive   bool  `gorm:"-" json:"isAlive"`
}
