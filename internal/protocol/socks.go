package protocol

import (
	"fmt"
	"net/url"
	"strings"
)

// SocksFormatter Socks (Socks5 / Socks4) 协议订阅格式化器
type SocksFormatter struct{}

func init() {
	s := &SocksFormatter{}
	Register(s)
	Register(&socks5Alias{s})
}

type socks5Alias struct {
	*SocksFormatter
}

func (a *socks5Alias) Protocol() string {
	return "socks5"
}

// Protocol 返回协议标识
func (f *SocksFormatter) Protocol() string {
	return "socks"
}

// FormatLink 将 NodeConfig 转换为 socks5:// 分享链接
func (f *SocksFormatter) FormatLink(node *NodeConfig) (string, error) {
	if node == nil {
		return "", ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return "", ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return "", ErrInvalidPort
	}

	targetHost := node.FormattedHost()
	remark := url.QueryEscape(node.Name)

	username := node.GetParam("user", node.GetParam("username"))
	password := node.GetParam("pass", node.GetParam("password"))

	if username != "" || password != "" {
		userInfo := url.UserPassword(username, password).String()
		return fmt.Sprintf("socks5://%s@%s:%d#%s", userInfo, targetHost, node.Port, remark), nil
	}

	return fmt.Sprintf("socks5://%s:%d#%s", targetHost, node.Port, remark), nil
}

// ToClash 将 Socks 节点转为 Clash / Mihomo proxy map
func (f *SocksFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return nil, ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return nil, ErrInvalidPort
	}

	proxy := map[string]interface{}{
		"name":   node.Name,
		"type":   "socks5",
		"server": node.RawHost(),
		"port":   node.Port,
	}

	username := node.GetParam("user", node.GetParam("username"))
	password := node.GetParam("pass", node.GetParam("password"))
	if username != "" {
		proxy["username"] = username
	}
	if password != "" {
		proxy["password"] = password
	}

	proxy["udp"] = true // socks5 默认启用 udp

	if node.GetParam("tls") == "true" || node.GetParam("security") == "tls" {
		proxy["tls"] = true
		if sni := node.GetParam("sni"); sni != "" {
			proxy["sni"] = sni
		}
		if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
			proxy["skip-cert-verify"] = true
		}
	}

	return proxy, nil
}

// ToSingBox 将 Socks 节点转为 Sing-box outbound map
func (f *SocksFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return nil, ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return nil, ErrInvalidPort
	}

	outbound := map[string]interface{}{
		"tag":         node.Name,
		"type":        "socks",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"version":     "5",
	}

	username := node.GetParam("user", node.GetParam("username"))
	password := node.GetParam("pass", node.GetParam("password"))
	if username != "" {
		outbound["username"] = username
	}
	if password != "" {
		outbound["password"] = password
	}

	return outbound, nil
}
