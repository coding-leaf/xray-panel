package domain

import (
	"encoding/json"
	"time"
)

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
	ExternalPort   int       `gorm:"column:external_port;default:0" json:"externalPort"` // 外部公开端口 (如 Nginx 443)，0表示使用全局设置
	ExternalHost   string    `gorm:"column:external_host;type:varchar(255);default:''" json:"externalHost"` // 外部连接域名/IP，留空使用全局域名
	RouteID        uint16    `gorm:"column:route_id;default:0" json:"routeId"` // VLESS 协议级 16 位路由编号 (0 为默认不替换)
	SubRoutesJson  string    `gorm:"column:sub_routes_json;type:text" json:"subRoutesJson"` // 分流订阅线路 JSON 列表
	UpBytes        int64     `gorm:"default:0" json:"upBytes"`
	DownBytes      int64     `gorm:"default:0" json:"downBytes"`
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`

	// 内存运行时探测状态 (不入持久化表)
	LatencyMs int64 `gorm:"-" json:"latencyMs"`
	IsAlive   bool  `gorm:"-" json:"isAlive"`
}

// SubRoute 定义入站下的分流订阅线路
type SubRoute struct {
	ID          string `json:"id"`          // 唯一标识 (如 nanoid 或 uuid)
	Name        string `json:"name"`        // 线路名称 (如 "🇯🇵 日本原生直连", "🇺🇸 美国中转落地")
	RouteID     uint16 `json:"routeId"`     // 16 位路由编号 (1 ~ 65535, 0 为默认直出)
	OutboundTag string `json:"outboundTag"` // 目标出站标签 (如 "direct", "us-test", "warp-out")
	Enabled     bool   `json:"enabled"`     // 是否启用
}

// GetSubRoutes 获取反序列化后的分流线路列表
func (in *Inbound) GetSubRoutes() []SubRoute {
	if in.SubRoutesJson == "" {
		return nil
	}
	var routes []SubRoute
	if err := json.Unmarshal([]byte(in.SubRoutesJson), &routes); err != nil {
		return nil
	}
	return routes
}
