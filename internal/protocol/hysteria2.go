package protocol

import (
	"fmt"
	"net/url"
	"strings"
)

// Hysteria2Formatter Hysteria2 协议订阅链接格式化器
type Hysteria2Formatter struct{}

func init() {
	h2 := &Hysteria2Formatter{}
	Register(h2)
	Register(&hysteria2Alias{h2})
}

type hysteria2Alias struct {
	*Hysteria2Formatter
}

func (a *hysteria2Alias) Protocol() string {
	return "hy2"
}

// Protocol 返回协议标识
func (f *Hysteria2Formatter) Protocol() string {
	return "hysteria2"
}

// FormatLink 将 NodeConfig 转换为标准 hysteria2:// 分享链接
func (f *Hysteria2Formatter) FormatLink(node *NodeConfig) (string, error) {
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

	v := url.Values{}

	if sni := node.GetParam("sni"); sni != "" {
		v.Set("sni", sni)
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		v.Set("alpn", alpn)
	}
	if insecure := node.GetParam("insecure", node.GetParam("allowInsecure")); insecure == "1" || insecure == "true" {
		v.Set("insecure", "1")
	}
	if obfs := node.GetParam("obfs"); obfs != "" {
		v.Set("obfs", obfs)
	}
	if obfsPassword := node.GetParam("obfs-password", node.GetParam("obfs_password")); obfsPassword != "" {
		v.Set("obfs-password", obfsPassword)
	}
	if pinSHA256 := node.GetParam("pinSHA256"); pinSHA256 != "" {
		v.Set("pinSHA256", pinSHA256)
	}
	if mport := node.GetParam("mport", node.GetParam("ports")); mport != "" {
		v.Set("mport", mport)
	}

	// 动态扩展参数透出 (非保留控制键安全加入 query)
	reservedKeys := map[string]bool{
		"sni": true, "alpn": true, "insecure": true, "allowInsecure": true,
		"obfs": true, "obfs-password": true, "obfs_password": true,
		"pinSHA256": true, "mport": true, "ports": true,
	}
	for k, val := range node.Params {
		kClean := strings.TrimSpace(k)
		if kClean == "" || val == "" {
			continue
		}
		if !reservedKeys[kClean] && v.Get(kClean) == "" {
			v.Set(kClean, val)
		}
	}

	targetHost := node.FormattedHost()

	queryStr := ""
	if encoded := v.Encode(); encoded != "" {
		queryStr = "?" + encoded
	}

	remark := node.Name
	return fmt.Sprintf("hysteria2://%s@%s:%d%s#%s", node.UUID, targetHost, node.Port, queryStr, url.QueryEscape(remark)), nil
}

// ToClash 将 Hysteria2 节点转为 Clash / Mihomo proxy map
func (f *Hysteria2Formatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
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
		"type":     "hysteria2",
		"server":   node.RawHost(),
		"port":     node.Port,
		"password": node.UUID,
	}
	if sni := node.GetParam("sni"); sni != "" {
		proxy["sni"] = sni
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		proxy["alpn"] = SplitAndTrim(alpn, ",")
	}
	if insecure := node.GetParam("insecure", node.GetParam("allowInsecure")); insecure == "1" || insecure == "true" {
		proxy["skip-cert-verify"] = true
	}
	if mport := node.GetParam("mport", node.GetParam("ports")); mport != "" {
		proxy["ports"] = mport
	}
	if obfs := node.GetParam("obfs"); obfs != "" {
		proxy["obfs"] = obfs
		if pass := node.GetParam("obfs-password", node.GetParam("obfs_password")); pass != "" {
			proxy["obfs-password"] = pass
		}
	}
	return proxy, nil
}

// ToSingBox 将 Hysteria2 节点转为 Sing-box outbound map
func (f *Hysteria2Formatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
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
		"type":        "hysteria2",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"password":    node.UUID,
	}

	tlsMap := map[string]interface{}{
		"enabled": true,
	}
	if sni := node.GetParam("sni"); sni != "" {
		tlsMap["server_name"] = sni
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		tlsMap["alpn"] = SplitAndTrim(alpn, ",")
	}
	if insecure := node.GetParam("insecure", node.GetParam("allowInsecure")); insecure == "1" || insecure == "true" {
		tlsMap["insecure"] = true
	}
	outbound["tls"] = tlsMap

	if obfs := node.GetParam("obfs"); obfs != "" {
		obfsMap := map[string]interface{}{
			"type": obfs,
		}
		if pass := node.GetParam("obfs-password", node.GetParam("obfs_password")); pass != "" {
			obfsMap["password"] = pass
		}
		outbound["obfs"] = obfsMap
	}
	return outbound, nil
}
