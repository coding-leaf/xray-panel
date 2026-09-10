package domain

import "context"

// Ticket 代表用于带外安全分发的动态高熵凭据实体
type Ticket struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Code          string `gorm:"size:16;uniqueIndex;not null" json:"code"` // 6位 Crockford Base32
	UserID        uint   `gorm:"index;not null" json:"user_id"`
	User          *User  `gorm:"foreignKey:UserID" json:"user,omitempty"`
	RemainingUses int    `gorm:"not null" json:"remaining_uses"`           // 允许兑换次数
	ExpiresAt     int64  `gorm:"index;not null" json:"expires_at"`         // Unix 秒级时间戳
	CreatedAt     int64  `json:"created_at"`
}

// TicketClaimPayload 代表向客户端下发的双轨冷启动交付数据
type TicketClaimPayload struct {
	UserEmail       string   `json:"user_email"`
	EmergencyNodes  []string `json:"emergency_nodes"`  // VLESS 节点链接切片 (用于冷启动一键导入)
	SubscriptionURL string   `json:"subscription_url"` // 隧道内自动更新订阅链接
	RemainingUses   int      `json:"remaining_uses"`
	ExpiresAt       int64    `json:"expires_at"`
}

// TicketRepository 定义 Ticket 持久化仓储接口
type TicketRepository interface {
	Create(ctx context.Context, ticket *Ticket) error
	GetByCode(ctx context.Context, code string) (*Ticket, error)
	ConsumeAtomic(ctx context.Context, code string) (*Ticket, error) // CAS 条件原子扣减并返回票据
	Delete(ctx context.Context, id uint) error
	CleanExpired(ctx context.Context) error
}
