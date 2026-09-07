package protocol

import (
	"fmt"
	"net/url"
	"strings"
)

// VlessFormatter VLESS 协议订阅链接格式化器
type VlessFormatter struct{}

func init() {
	Register(&VlessFormatter{})
}

// Protocol 返回协议标识
func (f *VlessFormatter) Protocol() string {
	return "vless"
}

// FormatLink 将 NodeConfig 转换为标准 vless:// 分享链接
func (f *VlessFormatter) FormatLink(node *NodeConfig) (string, error) {
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

	// 1. 基础传输与加密
	network := node.GetParam("type", "tcp")
	v.Set("type", network)

	security := node.GetParam("security", "none")
	v.Set("security", security)

	encryption := node.GetParam("encryption", "none")
	v.Set("encryption", encryption)

	// 2. TLS 与 REALITY 安全层参数
	if security == "reality" {
		fp := node.GetParam("fp", "chrome")
		v.Set("fp", fp)
		if pbk := node.GetParam("pbk"); pbk != "" {
			v.Set("pbk", pbk)
		}
		if sni := node.GetParam("sni"); sni != "" {
			v.Set("sni", sni)
		}
		if sid := node.GetParam("sid"); sid != "" {
			v.Set("sid", sid)
		}
		if spx := node.GetParam("spx"); spx != "" {
			v.Set("spx", spx)
		}
	} else if security == "tls" {
		fp := node.GetParam("fp", "chrome")
		v.Set("fp", fp)
		if sni := node.GetParam("sni"); sni != "" {
			v.Set("sni", sni)
		}
		if alpn := node.GetParam("alpn"); alpn != "" {
			v.Set("alpn", alpn)
		}
		if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
			v.Set("allowInsecure", "1")
		}
	}

	// 3. 流控参数 (Flow 控制，如 Vision)
	// XTLS Vision 严格仅限 TCP + (REALITY 或 TLS) 传输层，其余协议（如 xhttp、ws、grpc）严禁携带 flow
	if (network == "tcp" || network == "") && (security == "reality" || security == "tls") {
		flow := node.GetParam("flow")
		if flow == "" {
			flow = "xtls-rprx-vision"
		}
		if flow != "none" {
			v.Set("flow", flow)
		}
	}

	// 4. 传输协议特定参数 (XHTTP / WS / gRPC / HTTPUpgrade / CDN 回源)
	switch network {
	case "xhttp", "splithttp":
		if p := node.GetParam("path"); p != "" {
			v.Set("path", p)
		}
		if m := node.GetParam("mode"); m != "" {
			v.Set("mode", m)
		}
		if h := node.GetParam("host"); h != "" {
			v.Set("host", h)
		}
		if extra := node.GetParam("extra"); extra != "" {
			v.Set("extra", extra)
		}
	case "ws":
		if p := node.GetParam("path"); p != "" {
			v.Set("path", p)
		}
		if h := node.GetParam("host"); h != "" {
			v.Set("host", h)
		}
		if ed := node.GetParam("ed"); ed != "" {
			v.Set("ed", ed)
		}
	case "grpc":
		if sn := node.GetParam("serviceName"); sn != "" {
			v.Set("serviceName", sn)
		}
		if m := node.GetParam("mode"); m != "" {
			v.Set("mode", m)
		}
	case "httpupgrade":
		if p := node.GetParam("path"); p != "" {
			v.Set("path", p)
		}
		if h := node.GetParam("host"); h != "" {
			v.Set("host", h)
		}
	default:
		// CDN 回源或自定义参数兜底
		if h := node.GetParam("host"); h != "" && v.Get("host") == "" {
			v.Set("host", h)
		}
		if p := node.GetParam("path"); p != "" && v.Get("path") == "" {
			v.Set("path", p)
		}
	}

	// 5. 动态扩展参数透出 (非保留控制键安全加入 query)
	reservedKeys := map[string]bool{
		"type": true, "security": true, "encryption": true,
		"fp": true, "pbk": true, "sni": true, "sid": true, "spx": true,
		"alpn": true, "allowInsecure": true, "insecure": true,
		"flow": true, "path": true, "mode": true, "host": true,
		"extra": true, "serviceName": true, "ed": true,
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
	remark := node.Name
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", node.UUID, targetHost, node.Port, v.Encode(), url.QueryEscape(remark)), nil
}

// ToClash 将 VLESS 节点转为 Clash / Mihomo proxy map
func (f *VlessFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
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

	network := node.GetParam("type", "tcp")
	security := node.GetParam("security", "none")

	proxy := map[string]interface{}{
		"name":    node.Name,
		"type":    "vless",
		"server":  node.RawHost(),
		"port":    node.Port,
		"uuid":    node.UUID,
		"udp":     true,
		"network": network,
	}

	isTLS := security == "tls" || security == "reality"
	proxy["tls"] = isTLS
	if sni := node.GetParam("sni"); sni != "" {
		proxy["servername"] = sni
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		proxy["alpn"] = SplitAndTrim(alpn, ",")
	}
	if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
		proxy["skip-cert-verify"] = true
	}
	if (network == "tcp" || network == "") && isTLS {
		if flow := node.GetParam("flow"); flow != "" && flow != "none" {
			proxy["flow"] = flow
		} else {
			proxy["flow"] = "xtls-rprx-vision"
		}
	}
	if fp := node.GetParam("fp"); fp != "" {
		proxy["client-fingerprint"] = fp
	}

	if security == "reality" {
		realityOpts := map[string]interface{}{}
		if pbk := node.GetParam("pbk"); pbk != "" {
			realityOpts["public-key"] = pbk
		}
		if sid := node.GetParam("sid"); sid != "" {
			realityOpts["short-id"] = sid
		}
		if spx := node.GetParam("spx"); spx != "" {
			realityOpts["spider-x"] = spx
		}
		proxy["reality-opts"] = realityOpts
	}

	AttachClashTransport(proxy, network, node)
	return proxy, nil
}

// ToSingBox 将 VLESS 节点转为 Sing-box outbound map
func (f *VlessFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
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

	network := node.GetParam("type", "tcp")
	security := node.GetParam("security", "none")

	outbound := map[string]interface{}{
		"tag":         node.Name,
		"type":        "vless",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"uuid":        node.UUID,
	}

	isTLS := security == "tls" || security == "reality"
	if (network == "tcp" || network == "") && isTLS {
		if flow := node.GetParam("flow"); flow != "" && flow != "none" {
			outbound["flow"] = flow
		} else {
			outbound["flow"] = "xtls-rprx-vision"
		}
	}

	if isTLS {
		tlsMap := map[string]interface{}{
			"enabled": true,
		}
		if sni := node.GetParam("sni"); sni != "" {
			tlsMap["server_name"] = sni
		}
		if alpn := node.GetParam("alpn"); alpn != "" {
			tlsMap["alpn"] = SplitAndTrim(alpn, ",")
		}
		if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
			tlsMap["insecure"] = true
		}
		if fp := node.GetParam("fp", "chrome"); fp != "" {
			tlsMap["utls"] = map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			}
		}
		if security == "reality" {
			realityMap := map[string]interface{}{
				"enabled": true,
			}
			if pbk := node.GetParam("pbk"); pbk != "" {
				realityMap["public_key"] = pbk
			}
			if sid := node.GetParam("sid"); sid != "" {
				realityMap["short_id"] = sid
			}
			tlsMap["reality"] = realityMap
		}
		outbound["tls"] = tlsMap
	}

	AttachSingBoxTransport(outbound, network, node)
	return outbound, nil
}
