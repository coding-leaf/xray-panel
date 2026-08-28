package domain

import "context"

// UserRepository 定义用户持久化接口
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*User, error)
	GetByUUID(ctx context.Context, uuid string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetBySubToken(ctx context.Context, token string) (*User, error)
	ListByInboundTag(ctx context.Context, tag string) ([]User, error)
	ListAll(ctx context.Context) ([]User, error)
	AddTraffic(ctx context.Context, email string, upBytes, downBytes int64) error
	ResetTraffic(ctx context.Context, id uint) error
}

// InboundRepository 定义入站节点持久化接口
type InboundRepository interface {
	Create(ctx context.Context, inbound *Inbound) error
	Update(ctx context.Context, inbound *Inbound) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*Inbound, error)
	GetByTag(ctx context.Context, tag string) (*Inbound, error)
	ListAll(ctx context.Context) ([]Inbound, error)
	AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error
}

// SettingRepository 定义系统设置仓储接口
type SettingRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) (map[string]string, error)
}

// AdminRepository 定义管理员仓储接口
type AdminRepository interface {
	GetByUsername(ctx context.Context, username string) (*AdminUser, error)
	Update(ctx context.Context, admin *AdminUser) error
	Create(ctx context.Context, admin *AdminUser) error
}

// XrayManager 深模块接口：封装 gRPC 与 Xray 进程交互
type XrayManager interface {
	AddUser(ctx context.Context, inboundTag string, user *User) error
	RemoveUser(ctx context.Context, inboundTag string, email string) error
	QueryTrafficStats(ctx context.Context, reset bool) ([]TrafficStat, error)
	ValidateConfig(ctx context.Context, rawJSON []byte) error
	ApplyConfigAndReload(ctx context.Context, rawJSON []byte) error
	GetServiceStatus(ctx context.Context) (ServiceStatus, error)
	RestartService(ctx context.Context) error
	GetVersion(ctx context.Context) (string, error)
}

// HostMonitor 深模块接口：系统与硬件监控
type HostMonitor interface {
	GetSystemMetrics(ctx context.Context) (*SystemMetrics, error)
	GetNetworkSpeed(ctx context.Context) (uint64, uint64, error)
}

// Notifier 深模块接口：多通道通知告警
type Notifier interface {
	SendTrafficAlert(ctx context.Context, alert TrafficAlert) error
	SendSystemAlert(ctx context.Context, alert SystemAlert) error
	SendServiceStatusAlert(ctx context.Context, status ServiceStatus) error
	SendCertAlert(ctx context.Context, alert CertAlert) error
	SendMessage(ctx context.Context, text string) error
}
