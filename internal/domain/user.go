package domain

import (
	"strings"
	"time"
)

type User struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID        string    `gorm:"type:varchar(64);uniqueIndex" json:"uuid"`
	Email       string    `gorm:"type:varchar(128);uniqueIndex" json:"email"` // 唯一用户名/邮箱
	InboundTag  string    `gorm:"type:varchar(64);index" json:"inboundTag"`   // 兼容旧字段
	InboundTags string    `gorm:"type:text" json:"inboundTags"`              // 授权的 Inbound Tags (逗号隔开)
	Flow        string    `gorm:"type:varchar(32)" json:"flow"`
	SubToken    string    `gorm:"type:varchar(64);uniqueIndex" json:"subToken"`
	TotalBytes  int64     `gorm:"default:0" json:"totalBytes"` // 0 为不限制
	UpBytes     int64     `gorm:"default:0" json:"upBytes"`
	DownBytes   int64     `gorm:"default:0" json:"downBytes"`
	ExpireTime  int64     `gorm:"default:0" json:"expireTime"` // 毫秒时间戳，0 为永不过期
	ResetDay    int       `gorm:"column:reset_day;default:0" json:"resetDay"` // 每月几号重置流量 (0为不重置, 1-31)
	IPLimit     int       `gorm:"column:ip_limit;default:0" json:"ipLimit"`   // 并发连接限制 (0为不限制)
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// 内存运行时实时速率与在线状态 (不入持久化表)
	UpSpeed    int64 `gorm:"-" json:"upSpeed"`    // Bytes/s
	DownSpeed  int64 `gorm:"-" json:"downSpeed"`  // Bytes/s
	LastActive int64 `gorm:"-" json:"lastActive"` // Unix 毫秒
	IsOnline   bool  `gorm:"-" json:"isOnline"`
}

// GetInboundTagList 获取该用户所属的所有节点 Tag 列表
func (u *User) GetInboundTagList() []string {
	if u.InboundTags != "" {
		parts := strings.Split(u.InboundTags, ",")
		var tags []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
		if len(tags) > 0 {
			return tags
		}
	}
	if u.InboundTag != "" {
		return []string{u.InboundTag}
	}
	return nil
}

// HasInbound 检查用户是否拥有指定节点的权限
func (u *User) HasInbound(tag string) bool {
	for _, t := range u.GetInboundTagList() {
		if t == tag {
			return true
		}
	}
	return false
}

// IsTrafficExceeded 检查是否超出流量限额
func (u *User) IsTrafficExceeded() bool {
	return u.TotalBytes > 0 && (u.UpBytes+u.DownBytes) >= u.TotalBytes
}

// IsExpired 检查是否已过期
func (u *User) IsExpired() bool {
	return u.ExpireTime > 0 && time.Now().UnixMilli() > u.ExpireTime
}

// IsActive 检查用户是否处于正常可用状态（未禁用、未超额、未过期）
func (u *User) IsActive() bool {
	if !u.Enabled {
		return false
	}
	if u.IsTrafficExceeded() {
		return false
	}
	if u.IsExpired() {
		return false
	}
	return true
}

type NodeShareInfo struct {
	Tag        string `json:"tag"`
	Protocol   string `json:"protocol"`
	Remark     string `json:"remark"`
	ShareLink  string `json:"shareLink"`
	SingleSub  string `json:"singleSub"`
}

type UserShareResponse struct {
	UserID     uint            `json:"userId"`
	Email      string          `json:"email"`
	SubToken   string          `json:"subToken"`
	AllSubURL  string          `json:"allSubUrl"`
	Nodes      []NodeShareInfo `json:"nodes"`
}
