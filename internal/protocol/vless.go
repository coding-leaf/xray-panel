package protocol

import (
	"fmt"
	"net/url"
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
	if err := ValidateBaseNode(node); err != nil {
		return "", err
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
		if flow != "none" && flow != "" {
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
	dynamicQuery := BuildNodeQueryParams(node)
	for k, vals := range dynamicQuery {
		for _, val := range vals {
			if v.Get(k) == "" {
				v.Set(k, val)
			}
		}
	}

	targetHost := node.FormattedHost()
	remark := node.Name
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", node.UUID, targetHost, node.Port, v.Encode(), url.QueryEscape(remark)), nil
}

// ToClash 将 VLESS 节点转为 Clash / Mihomo proxy map
func (f *VlessFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
	if err := ValidateBaseNode(node); err != nil {
		return nil, err
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

	AttachClashTLS(proxy, node)

	isTLS := security == "tls" || security == "reality"
	if (network == "tcp" || network == "") && isTLS {
		if flow := node.GetParam("flow"); flow != "" && flow != "none" {
			proxy["flow"] = flow
		}
	}

	AttachClashTransport(proxy, network, node)
	return proxy, nil
}

// ToSingBox 将 VLESS 节点转为 Sing-box outbound map
func (f *VlessFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	if err := ValidateBaseNode(node); err != nil {
		return nil, err
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
		}
	}

	AttachSingBoxTLS(outbound, node)
	AttachSingBoxTransport(outbound, network, node)
	return outbound, nil
}
