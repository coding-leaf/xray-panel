package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// VmessFormatter VMess 协议订阅链接格式化器
type VmessFormatter struct{}

func init() {
	Register(&VmessFormatter{})
}

// Protocol 返回协议标识
func (f *VmessFormatter) Protocol() string {
	return "vmess"
}

// FormatLink 将 NodeConfig 转换为标准 vmess:// 分享链接 (Base64 JSON)
func (f *VmessFormatter) FormatLink(node *NodeConfig) (string, error) {
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

	network := node.GetParam("type", "tcp")
	security := node.GetParam("security", "none")

	vmessObj := map[string]interface{}{
		"v":    "2",
		"ps":   node.Name,
		"add":  node.RawHost(),
		"port": node.Port,
		"id":   node.UUID,
		"aid":  0,
		"net":  network,
		"type": "none",
		"tls":  security,
	}

	if security == "tls" {
		if sni := node.GetParam("sni"); sni != "" {
			vmessObj["sni"] = sni
		}
		if alpn := node.GetParam("alpn"); alpn != "" {
			vmessObj["alpn"] = alpn
		}
		if fp := node.GetParam("fp"); fp != "" {
			vmessObj["fp"] = fp
		}
	}

	if network == "ws" {
		if p := node.GetParam("path"); p != "" {
			vmessObj["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			vmessObj["host"] = h
		}
	} else if network == "grpc" {
		if sn := node.GetParam("serviceName"); sn != "" {
			vmessObj["path"] = sn
		}
	} else if network == "xhttp" || network == "splithttp" {
		if p := node.GetParam("path"); p != "" {
			vmessObj["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			vmessObj["host"] = h
		}
	}

	rawJSON, err := json.Marshal(vmessObj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal vmess JSON: %w", err)
	}

	return "vmess://" + base64.StdEncoding.EncodeToString(rawJSON), nil
}

// ToClash 将 VMess 节点转为 Clash / Mihomo proxy map
func (f *VmessFormatter) ToClash(node *NodeConfig) (map[string]interface{}, error) {
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
		"type":    "vmess",
		"server":  node.RawHost(),
		"port":    node.Port,
		"uuid":    node.UUID,
		"alterId": 0,
		"cipher":  node.GetParam("cipher", "auto"),
		"udp":     true,
		"network": network,
		"tls":     security == "tls",
	}

	if sni := node.GetParam("sni"); sni != "" {
		proxy["servername"] = sni
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

// ToSingBox 将 VMess 节点转为 Sing-box outbound map
func (f *VmessFormatter) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
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
		"type":        "vmess",
		"server":      node.RawHost(),
		"server_port": node.Port,
		"uuid":        node.UUID,
		"security":    "auto",
	}

	if security == "tls" {
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
	}

	AttachSingBoxTransport(outbound, network, node)
	return outbound, nil
}
