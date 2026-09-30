package protocol

import (
	"net"
	"strings"

	"panel/internal/domain"
	"panel/internal/pkg/crypto"
)

type RealityKeyPair = crypto.RealityKeyPair

// GenerateRealityKeyPair 生成符合 Xray/VLESS 规范的 x25519 Reality 密钥对与 ShortId
func GenerateRealityKeyPair() (*RealityKeyPair, error) {
	return crypto.GenerateRealityKeyPair()
}

// DerivePublicKeyFromPrivate 从 Reality base64 私钥自动推导 x25519 对应公钥
func DerivePublicKeyFromPrivate(privStr string) string {
	return crypto.DerivePublicKeyFromPrivate(privStr)
}

// ApplyVlessRouteToUUID 将 routeID 编码进 UUID 的第 3 组字段
func ApplyVlessRouteToUUID(rawUUID string, routeID uint16) string {
	return crypto.ApplyVlessRouteToUUID(rawUUID, routeID)
}

func cleanHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "[") && strings.Contains(s, "]") {
		idx := strings.Index(s, "]")
		return s[1:idx]
	}
	if strings.Count(s, ":") >= 2 {
		return s
	}
	if host, _, err := net.SplitHostPort(s); err == nil && host != "" {
		return host
	}
	if idx := strings.Index(s, ":"); idx != -1 {
		return s[:idx]
	}
	return s
}

// InboundToNodeConfig 将 domain.Inbound 与 domain.User 转换为通用的 protocol.NodeConfig
func InboundToNodeConfig(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) *NodeConfig {
	if inbound == nil || user == nil {
		return nil
	}

	targetHost := cleanHost(inbound.ExternalHost)
	if targetHost == "" {
		targetHost = cleanHost(hostDomain)
	}
	if targetHost == "" {
		targetHost = inbound.Listen
		if targetHost == "0.0.0.0" || targetHost == "" {
			targetHost = "127.0.0.1"
		}
	}

	targetPort := inbound.Port
	if inbound.ExternalPort > 0 {
		targetPort = inbound.ExternalPort
	} else if defaultPort > 0 {
		targetPort = defaultPort
	}

	accessor := inbound.GetStreamAccessor()
	network := accessor.Network()
	security := accessor.Security()

	effectiveUUID := user.UUID
	if inbound.RouteID > 0 && (inbound.Protocol == "" || strings.EqualFold(inbound.Protocol, "vless")) {
		effectiveUUID = ApplyVlessRouteToUUID(user.UUID, inbound.RouteID)
	}

	remark := inbound.Remark
	if remark == "" {
		remark = inbound.Tag
	}

	node := NewNodeConfig(remark, inbound.Protocol, targetHost, targetPort, effectiveUUID)
	node.SetParam("type", network)
	node.SetParam("security", security)

	// 1. TLS 配置
	if security == "tls" {
		if sni := accessor.GetTLSServerName(); sni != "" {
			node.SetParam("sni", sni)
		}
		if alpns := accessor.GetTLSALPN(); len(alpns) > 0 {
			node.SetParam("alpn", strings.Join(alpns, ","))
		}
	}

	// 2. REALITY 配置
	if security == "reality" {
		if pbk := accessor.GetRealityPublicKey(); pbk != "" {
			node.SetParam("pbk", pbk)
		}
		if sni := accessor.GetRealityServerName(); sni != "" {
			node.SetParam("sni", sni)
		}
		if sid := accessor.GetRealityShortID(); sid != "" {
			node.SetParam("sid", sid)
		}
		if spx := accessor.GetRealitySpiderX(); spx != "" {
			node.SetParam("spx", spx)
		}
		if fp := accessor.GetRealityFingerprint(); fp != "" {
			node.SetParam("fp", fp)
		} else {
			node.SetParam("fp", "chrome")
		}
	}

	// 3. 传输层参数
	switch network {
	case "ws":
		if path := accessor.GetWSPath(); path != "" {
			node.SetParam("path", path)
		}
		if host := accessor.GetWSHost(); host != "" {
			node.SetParam("host", host)
		}
	case "grpc":
		if sn := accessor.GetGRPCServiceName(); sn != "" {
			node.SetParam("serviceName", sn)
		}
	case "xhttp", "splithttp":
		if path := accessor.GetXHTTPPath(); path != "" {
			node.SetParam("path", path)
		}
		if mode := accessor.GetXHTTPMode(); mode != "" {
			node.SetParam("mode", mode)
		}
		if host := accessor.GetXHTTPHost(); host != "" {
			node.SetParam("host", host)
		}
	}

	// 4. 流控参数 (XTLS Vision)
	// 协议与传输层强约束：flow (XTLS Vision) 仅限 VLESS 协议且传输为 TCP + (REALITY 或 TLS)。
	// 对于非 TCP 传输（如 xhttp, splithttp, ws, grpc）或非 TLS/REALITY，强制剔除 flow 参数杜绝污染。
	if strings.EqualFold(node.Protocol, "vless") {
		flow := accessor.ResolveVisionFlow(inbound.Protocol)
		if flow == "" && user.Flow != "" {
			// 若流配置未显式指定，且用户实体配有流控，再次严格验证传输层约束
			if (network == "" || network == "tcp") && (security == "reality" || security == "tls") {
				flow = user.Flow
			}
		}
		if flow != "" {
			node.SetParam("flow", flow)
		}
	}

	// 5. Shadowsocks 加密方式
	if strings.EqualFold(node.Protocol, "shadowsocks") {
		node.SetParam("method", accessor.ResolveShadowsocksMethod())
	}

	// 6. Socks / HTTP 认证
	if strings.EqualFold(node.Protocol, "socks") || strings.EqualFold(node.Protocol, "http") {
		sm := accessor.GetSettingsMap()
		if accounts, ok := sm["accounts"].([]interface{}); ok && len(accounts) > 0 {
			if acc, ok := accounts[0].(map[string]interface{}); ok {
				if u, ok := acc["user"].(string); ok && u != "" {
					node.SetParam("user", u)
				}
				if p, ok := acc["pass"].(string); ok && p != "" {
					node.SetParam("pass", p)
				}
			}
		}
	}

	return node
}

// InboundsToNodeConfigs 将一组 Inbound 和 User 转换为 []*NodeConfig
func InboundsToNodeConfigs(inbounds []domain.Inbound, user *domain.User, hostDomain string, defaultPort int, tagFilter string) []*NodeConfig {
	var nodes []*NodeConfig
	for _, in := range inbounds {
		if !in.Enabled || !user.HasInbound(in.Tag) {
			continue
		}
		if tagFilter != "" && in.Tag != tagFilter {
			continue
		}
		subRoutes := in.GetSubRoutes()
		if len(subRoutes) == 0 {
			if node := InboundToNodeConfig(&in, user, hostDomain, defaultPort); node != nil {
				nodes = append(nodes, node)
			}
			continue
		}
		for _, sr := range subRoutes {
			if !sr.Enabled {
				continue
			}
			if user != nil && !sr.CanAccess(user.Email) {
				continue
			}
			tempInbound := in
			if sr.Name != "" {
				tempInbound.Remark = sr.Name
			}
			tempInbound.RouteID = sr.RouteID
			if node := InboundToNodeConfig(&tempInbound, user, hostDomain, defaultPort); node != nil {
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

// BuildShareLink 生成标准 Xray 分享链接 (基于 protocol.Registry 策略模式)
func BuildShareLink(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) string {
	node := InboundToNodeConfig(inbound, user, hostDomain, defaultPort)
	if node == nil {
		return ""
	}
	link, err := FormatLink(node)
	if err != nil {
		return ""
	}
	return link
}
