package protocol

import (
	"net/url"
	"strings"
)

// ValidateBaseNode 统一验证基础节点配置完整性
func ValidateBaseNode(node *NodeConfig) error {
	if node == nil {
		return ErrNilNode
	}
	if strings.TrimSpace(node.Address) == "" {
		return ErrMissingAddress
	}
	if node.Port <= 0 || node.Port > 65535 {
		return ErrInvalidPort
	}
	if strings.TrimSpace(node.UUID) == "" {
		return ErrMissingUUID
	}
	return nil
}

// SplitAndTrim 按分隔符切分字符串并去除各项首尾空格、过滤空项
func SplitAndTrim(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// AttachClashTLS 为 Clash proxy 统一注入 TLS 与 REALITY 安全层配置
func AttachClashTLS(proxy map[string]interface{}, node *NodeConfig) {
	if proxy == nil || node == nil {
		return
	}
	security := strings.ToLower(node.GetParam("security", "none"))
	isTLS := security == "tls" || security == "reality"
	proxy["tls"] = isTLS

	if isTLS {
		if sni := node.GetParam("sni"); sni != "" {
			proxy["servername"] = sni
		}
		if alpn := node.GetParam("alpn"); alpn != "" {
			proxy["alpn"] = SplitAndTrim(alpn, ",")
		}
		if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
			proxy["skip-cert-verify"] = true
		}
		if fp := node.GetParam("fp"); fp != "" {
			proxy["client-fingerprint"] = fp
		}
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
		if len(realityOpts) > 0 {
			proxy["reality-opts"] = realityOpts
		}
	}
}

// AttachSingBoxTLS 为 Sing-box outbound 统一注入 TLS 与 REALITY 安全层配置
func AttachSingBoxTLS(outbound map[string]interface{}, node *NodeConfig) {
	if outbound == nil || node == nil {
		return
	}
	security := strings.ToLower(node.GetParam("security", "none"))
	if security != "tls" && security != "reality" {
		return
	}

	tls := map[string]interface{}{
		"enabled": true,
	}

	if sni := node.GetParam("sni"); sni != "" {
		tls["server_name"] = sni
	}
	if alpn := node.GetParam("alpn"); alpn != "" {
		tls["alpn"] = SplitAndTrim(alpn, ",")
	}
	if allowInsecure := node.GetParam("allowInsecure", node.GetParam("insecure")); allowInsecure == "1" || allowInsecure == "true" {
		tls["insecure"] = true
	}
	if fp := node.GetParam("fp"); fp != "" {
		tls["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}

	if security == "reality" {
		reality := map[string]interface{}{
			"enabled": true,
		}
		if pbk := node.GetParam("pbk"); pbk != "" {
			reality["public_key"] = pbk
		}
		if sid := node.GetParam("sid"); sid != "" {
			reality["short_id"] = sid
		}
		tls["reality"] = reality
	}

	outbound["tls"] = tls
}

// BuildNodeQueryParams 统一构建 URI Query 参数，并安全透出未被保留的额外动态参数
func BuildNodeQueryParams(node *NodeConfig, extraReserved ...string) url.Values {
	v := url.Values{}
	if node == nil {
		return v
	}

	reservedKeys := map[string]bool{
		"type": true, "security": true, "sni": true, "alpn": true,
		"fp": true, "allowInsecure": true, "insecure": true,
		"flow": true, "path": true, "mode": true, "host": true,
		"extra": true, "serviceName": true, "pbk": true, "sid": true,
		"spx": true, "encryption": true,
	}
	for _, k := range extraReserved {
		reservedKeys[strings.ToLower(strings.TrimSpace(k))] = true
	}

	for k, val := range node.Params {
		kClean := strings.TrimSpace(k)
		if kClean == "" || val == "" {
			continue
		}
		if !reservedKeys[strings.ToLower(kClean)] && v.Get(kClean) == "" {
			v.Set(kClean, val)
		}
	}
	return v
}

// AttachClashTransport 为 Clash proxy 注入传输层配置 (ws, grpc, xhttp, httpupgrade)
func AttachClashTransport(proxy map[string]interface{}, network string, node *NodeConfig) {
	if node == nil || proxy == nil {
		return
	}
	switch network {
	case "ws":
		wsOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			wsOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			wsOpts["headers"] = map[string]string{"Host": h}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc":
		grpcOpts := map[string]interface{}{}
		if sn := node.GetParam("serviceName"); sn != "" {
			grpcOpts["grpc-service-name"] = sn
		}
		if len(grpcOpts) > 0 {
			proxy["grpc-opts"] = grpcOpts
		}
	case "xhttp", "splithttp":
		xhttpOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			xhttpOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			xhttpOpts["headers"] = map[string]string{"Host": h}
		}
		if m := node.GetParam("mode"); m != "" {
			xhttpOpts["mode"] = m
		}
		if len(xhttpOpts) > 0 {
			proxy["xhttp-opts"] = xhttpOpts
		}
	case "httpupgrade":
		huOpts := map[string]interface{}{}
		if p := node.GetParam("path"); p != "" {
			huOpts["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			huOpts["headers"] = map[string]string{"Host": h}
		}
		if len(huOpts) > 0 {
			proxy["httpupgrade-opts"] = huOpts
		}
	}
}

// AttachSingBoxTransport 为 Sing-box outbound 注入传输层配置 (ws, grpc, xhttp, httpupgrade)
func AttachSingBoxTransport(outbound map[string]interface{}, network string, node *NodeConfig) {
	if node == nil || outbound == nil {
		return
	}
	switch network {
	case "ws":
		transport := map[string]interface{}{
			"type": "ws",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		outbound["transport"] = transport
	case "grpc":
		transport := map[string]interface{}{
			"type": "grpc",
		}
		if sn := node.GetParam("serviceName"); sn != "" {
			transport["service_name"] = sn
		}
		outbound["transport"] = transport
	case "xhttp", "splithttp":
		transport := map[string]interface{}{
			"type": "xhttp",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		if m := node.GetParam("mode"); m != "" {
			transport["mode"] = m
		}
		outbound["transport"] = transport
	case "httpupgrade":
		transport := map[string]interface{}{
			"type": "httpupgrade",
		}
		if p := node.GetParam("path"); p != "" {
			transport["path"] = p
		}
		if h := node.GetParam("host"); h != "" {
			transport["headers"] = map[string]string{"Host": h}
		}
		outbound["transport"] = transport
	}
}
