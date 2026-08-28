package domain

import "time"

type TrafficStatType string

const (
	TrafficStatTypeUser    TrafficStatType = "user"
	TrafficStatTypeInbound TrafficStatType = "inbound"
	TrafficStatTypeOutbound TrafficStatType = "outbound"
)

type TrafficStat struct {
	Type      TrafficStatType `json:"type"`
	Tag       string          `json:"tag"`
	IsUplink  bool            `json:"isUplink"`
	Value     int64           `json:"value"` // byte count in delta query
}

type TrafficLog struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"index" json:"userId"`
	UserEmail string    `gorm:"type:varchar(128);index" json:"userEmail"`
	UpBytes   int64     `json:"upBytes"`
	DownBytes int64     `json:"downBytes"`
	Date      string    `gorm:"type:varchar(10);index" json:"date"` // YYYY-MM-DD
	CreatedAt time.Time `json:"createdAt"`
}

type TrafficAlert struct {
	Email       string  `json:"email"`
	UsedBytes   int64   `json:"usedBytes"`
	TotalBytes  int64   `json:"totalBytes"`
	UsageRatio  float64 `json:"usageRatio"`
}

type SystemAlert struct {
	Metric      string  `json:"metric"`
	CurrentVal  float64 `json:"currentVal"`
	Threshold   float64 `json:"threshold"`
	Description string  `json:"description"`
}
