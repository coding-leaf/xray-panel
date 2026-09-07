package protocol

import (
	"fmt"
	"net"
	"strings"
)

// NodeConfig 通用节点实体定义
type NodeConfig struct {
	Name     string            `json:"name"`     // 节点备注/名称 (Remark / Tag)
	Address  string            `json:"address"`  // 节点地址 (IP 或 域名)
	Port     int               `json:"port"`     // 端口
	Protocol string            `json:"protocol"` // 协议标识: vless, trojan, hysteria2, vmess, etc.
	UUID     string            `json:"uuid"`     // 用户凭据: UUID / Password / Auth Token
	Params   map[string]string `json:"params"`   // 动态扩展字段 (pbk, path, sni, alpn, flow, host 等)
}

// NewNodeConfig 创建通用节点实体
func NewNodeConfig(name, protocol, address string, port int, uuid string) *NodeConfig {
	return &NodeConfig{
		Name:     name,
		Protocol: strings.ToLower(strings.TrimSpace(protocol)),
		Address:  strings.TrimSpace(address),
		Port:     port,
		UUID:     strings.TrimSpace(uuid),
		Params:   make(map[string]string),
	}
}

// GetParam 获取动态参数，若不存在或为空则返回 fallback（若提供）
func (n *NodeConfig) GetParam(key string, fallback ...string) string {
	if n == nil || n.Params == nil {
		if len(fallback) > 0 {
			return fallback[0]
		}
		return ""
	}
	if val, ok := n.Params[key]; ok && val != "" {
		return val
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}

// SetParam 安全设置动态参数
func (n *NodeConfig) SetParam(key, value string) {
	if n == nil {
		return
	}
	if n.Params == nil {
		n.Params = make(map[string]string)
	}
	n.Params[key] = value
}

// Clone 返回节点副本
func (n *NodeConfig) Clone() *NodeConfig {
	if n == nil {
		return nil
	}
	clone := *n
	if n.Params != nil {
		clone.Params = make(map[string]string, len(n.Params))
		for k, v := range n.Params {
			clone.Params[k] = v
		}
	}
	return &clone
}

// RawHost 返回去除括号和端口的纯净 Host/IP (适用于 Clash/Sing-box/VMess 等配置字段)
func (n *NodeConfig) RawHost() string {
	if n == nil {
		return ""
	}
	addr := strings.TrimSpace(n.Address)
	if h, _, err := net.SplitHostPort(addr); err == nil {
		addr = h
	}
	addr = strings.TrimPrefix(addr, "[")
	addr = strings.TrimSuffix(addr, "]")
	return addr
}

// FormattedHost 返回针对 IPv6 自动用括号包裹的安全 Host 字符串 (适用于 URI 字符串连接)
func (n *NodeConfig) FormattedHost() string {
	if n == nil {
		return ""
	}
	host := n.RawHost()
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// FormattedAddressPort 返回包含安全 Host 和 Port 的连接目标
func (n *NodeConfig) FormattedAddressPort() string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", n.FormattedHost(), n.Port)
}
