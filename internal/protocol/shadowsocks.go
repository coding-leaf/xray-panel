package protocol

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// ShadowsocksFormatter Shadowsocks 协议订阅链接格式化器
type ShadowsocksFormatter struct{}

func init() {
	ss := &ShadowsocksFormatter{}
	Register(ss)
	Register(&shadowsocksAlias{ss})
}

type shadowsocksAlias struct {
	*ShadowsocksFormatter
}

func (a *shadowsocksAlias) Protocol() string {
	return "ss"
}

// Protocol 返回协议标识
func (f *ShadowsocksFormatter) Protocol() string {
	return "shadowsocks"
}

// FormatLink 将 NodeConfig 转换为标准 ss:// 分享链接
func (f *ShadowsocksFormatter) FormatLink(node *NodeConfig) (string, error) {
	if node == nil {
		return "", ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return "", ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return "", ErrInvalidPort
	}
	if strings.TrimSpace(node.UUID) == "" {
		return "", ErrMissingUUID
	}

	method := node.GetParam("method", "aes-128-gcm")
	auth := fmt.Sprintf("%s:%s", method, node.UUID)
	encodedAuth := base64.URLEncoding.EncodeToString([]byte(auth))

	targetHost := node.FormattedHost()
	remark := url.QueryEscape(node.Name)
	return fmt.Sprintf("ss://%s@%s:%d#%s", encodedAuth, targetHost, node.Port, remark), nil
}

// ToClash 将 Shadowsocks 节点转为 Clash / Mihomo proxy map
func (f *ShadowsocksFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return nil, ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return nil, ErrInvalidPort
	}
	if strings.TrimSpace(node.UUID) == "" {
		return nil, ErrMissingUUID
	}

	proxy := map[string]interface{}{
		"name":     node.Name,
		"type":     "ss",
		"server":   node.RawHost(),
		"port":     node.Port,
		"cipher":   node.GetParam("method", "aes-128-gcm"),
		"password": node.UUID,
		"udp":      true,
	}
	return proxy, nil
}

// ToSingBox 将 Shadowsocks 节点转为 Sing-box outbound map
func (f *ShadowsocksFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return nil, ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return nil, ErrInvalidPort
	}
	if strings.TrimSpace(node.UUID) == "" {
		return nil, ErrMissingUUID
	}

	outbound := map[string]interface{}{
		"tag":         node.Name,
		"type":        "shadowsocks",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"method":      node.GetParam("method", "aes-128-gcm"),
		"password":    node.UUID,
	}
	return outbound, nil
}
