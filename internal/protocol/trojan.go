package protocol

import (
	"fmt"
	"net/url"
)

// TrojanFormatter Trojan 协议订阅链接格式化器
type TrojanFormatter struct{}

func init() {
	Register(&TrojanFormatter{})
}

// Protocol 返回协议标识
func (f *TrojanFormatter) Protocol() string {
	return "trojan"
}

// FormatLink 将 NodeConfig 转换为标准 trojan:// 分享链接
func (f *TrojanFormatter) FormatLink(node *NodeConfig) (string, error) {
	if err := ValidateBaseNode(node); err != nil {
		return "", err
	}

	v := url.Values{}

	network := node.GetParam("type", "tcp")
	v.Set("type", network)

	security := node.GetParam("security", "tls")
	v.Set("security", security)

	if security == "tls" {
		if sni := node.GetParam("sni"); sni != "" {
			v.Set("sni", sni)
		}
		if alpn := node.GetParam("alpn"); alpn != "" {
			v.Set("alpn", alpn)
		}
		if fp := node.GetParam("fp"); fp != "" {
			v.Set("fp", fp)
		}
		if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
			v.Set("allowInsecure", "1")
		}
	}

	switch network {
	case "ws":
		if p := node.GetParam("path"); p != "" {
			v.Set("path", p)
		}
		if h := node.GetParam("host"); h != "" {
			v.Set("host", h)
		}
	case "grpc":
		if sn := node.GetParam("serviceName"); sn != "" {
			v.Set("serviceName", sn)
		}
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
	default:
		if h := node.GetParam("host"); h != "" {
			v.Set("host", h)
		}
		if p := node.GetParam("path"); p != "" {
			v.Set("path", p)
		}
	}

	// 动态扩展参数透出 (非保留控制键安全加入 query)
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
	return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", node.UUID, targetHost, node.Port, v.Encode(), url.QueryEscape(remark)), nil
}

// ToClash 将 Trojan 节点转为 Clash / Mihomo proxy map
func (f *TrojanFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
	if err := ValidateBaseNode(node); err != nil {
		return nil, err
	}

	network := node.GetParam("type", "tcp")
	proxy := map[string]interface{}{
		"name":     node.Name,
		"type":     "trojan",
		"server":   node.RawHost(),
		"port":     node.Port,
		"password": node.UUID,
		"udp":      true,
		"network":  network,
	}

	if sni := node.GetParam("sni"); sni != "" {
		proxy["sni"] = sni
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		proxy["alpn"] = SplitAndTrim(alpn, ",")
	}
	if fp := node.GetParam("fp"); fp != "" {
		proxy["client-fingerprint"] = fp
	}
	if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
		proxy["skip-cert-verify"] = true
	}

	AttachClashTransport(proxy, network, node)
	return proxy, nil
}

// ToSingBox 将 Trojan 节点转为 Sing-box outbound map
func (f *TrojanFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	if err := ValidateBaseNode(node); err != nil {
		return nil, err
	}

	network := node.GetParam("type", "tcp")
	outbound := map[string]interface{}{
		"tag":         node.Name,
		"type":        "trojan",
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
	if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
		tlsMap["insecure"] = true
	}
	outbound["tls"] = tlsMap

	AttachSingBoxTransport(outbound, network, node)
	return outbound, nil
}
