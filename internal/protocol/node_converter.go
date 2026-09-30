package protocol

import (
	"encoding/json"
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

type inboundRawStream struct {
	Network         string `json:"network"`
	Security        string `json:"security"`
	RealitySettings *struct {
		Dest        string   `json:"dest"`
		ServerNames []string `json:"serverNames"`
		ServerName  string   `json:"serverName"`
		PrivateKey  string   `json:"privateKey"`
		PublicKey   string   `json:"publicKey"`
		ShortIds    []string `json:"shortIds"`
		ShortId     string   `json:"shortId"`
		Fingerprint string   `json:"fingerprint"`
		SpiderX     string   `json:"spiderX"`
	} `json:"realitySettings"`
	TLSSettings *struct {
		ServerName string   `json:"serverName"`
		ALPN       []string `json:"alpn"`
	} `json:"tlsSettings"`
	XHTTPSettings *struct {
		Path    string            `json:"path"`
		Mode    string            `json:"mode"`
		Host    string            `json:"host"`
		Headers map[string]string `json:"headers"`
	} `json:"xhttpSettings"`
	WSSettings *struct {
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
		Host    string            `json:"host"`
	} `json:"wsSettings"`
	GRPCSettings *struct {
		ServiceName string `json:"serviceName"`
		MultiMode   bool   `json:"multiMode"`
	} `json:"grpcSettings"`
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

	var rawStream inboundRawStream
	if s := strings.TrimSpace(inbound.StreamSettings); s != "" && s != "null" && s != "{}" {
		_ = json.Unmarshal([]byte(s), &rawStream)
	}
	var settingsMap map[string]interface{}
	if s := strings.TrimSpace(inbound.SettingsJSON); s != "" && s != "null" && s != "{}" {
		_ = json.Unmarshal([]byte(s), &settingsMap)
	}

	network := strings.ToLower(strings.TrimSpace(rawStream.Network))
	if network == "" {
		network = "tcp"
	}
	security := strings.ToLower(strings.TrimSpace(rawStream.Security))
	if security == "" {
		security = "none"
	}

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
	if security == "tls" && rawStream.TLSSettings != nil {
		if rawStream.TLSSettings.ServerName != "" {
			node.SetParam("sni", rawStream.TLSSettings.ServerName)
		}
		if len(rawStream.TLSSettings.ALPN) > 0 {
			node.SetParam("alpn", strings.Join(rawStream.TLSSettings.ALPN, ","))
		}
	}

	// 2. REALITY 配置
	if security == "reality" && rawStream.RealitySettings != nil {
		r := rawStream.RealitySettings
		pbk := r.PublicKey
		if pbk == "" && r.PrivateKey != "" {
			pbk = crypto.DerivePublicKeyFromPrivate(r.PrivateKey)
		}
		if pbk != "" {
			node.SetParam("pbk", pbk)
		}
		sni := r.ServerName
		if sni == "" && len(r.ServerNames) > 0 {
			sni = r.ServerNames[0]
		}
		if sni != "" {
			node.SetParam("sni", sni)
		}
		sid := r.ShortId
		if sid == "" && len(r.ShortIds) > 0 {
			sid = r.ShortIds[0]
		}
		if sid != "" {
			node.SetParam("sid", sid)
		}
		if r.SpiderX != "" {
			node.SetParam("spx", r.SpiderX)
		}
		if r.Fingerprint != "" {
			node.SetParam("fp", r.Fingerprint)
		} else {
			node.SetParam("fp", "chrome")
		}
	}

	// 3. 传输层参数
	switch network {
	case "ws":
		if rawStream.WSSettings != nil {
			if rawStream.WSSettings.Path != "" {
				node.SetParam("path", rawStream.WSSettings.Path)
			}
			host := ""
			if rawStream.WSSettings.Headers != nil {
				if h, ok := rawStream.WSSettings.Headers["Host"]; ok && h != "" {
					host = h
				} else if h, ok := rawStream.WSSettings.Headers["host"]; ok && h != "" {
					host = h
				}
			}
			if host == "" && rawStream.WSSettings.Host != "" {
				host = rawStream.WSSettings.Host
			}
			if host != "" {
				node.SetParam("host", host)
			}
		}
	case "grpc":
		if rawStream.GRPCSettings != nil && rawStream.GRPCSettings.ServiceName != "" {
			node.SetParam("serviceName", rawStream.GRPCSettings.ServiceName)
		}
	case "xhttp", "splithttp":
		if rawStream.XHTTPSettings != nil {
			if rawStream.XHTTPSettings.Path != "" {
				node.SetParam("path", rawStream.XHTTPSettings.Path)
			}
			if rawStream.XHTTPSettings.Mode != "" {
				node.SetParam("mode", rawStream.XHTTPSettings.Mode)
			}
			host := rawStream.XHTTPSettings.Host
			if host == "" && rawStream.XHTTPSettings.Headers != nil {
				if h, ok := rawStream.XHTTPSettings.Headers["Host"]; ok && h != "" {
					host = h
				} else if h, ok := rawStream.XHTTPSettings.Headers["host"]; ok && h != "" {
					host = h
				}
			}
			if host != "" {
				node.SetParam("host", host)
			}
		}
	}

	// 4. 流控参数 (XTLS Vision)
	// 协议与传输层强约束：flow (XTLS Vision) 仅限 VLESS 协议且传输为 TCP + (REALITY 或 TLS)。
	// 对于非 TCP 传输（如 xhttp, splithttp, ws, grpc）或非 TLS/REALITY，强制剔除 flow 参数杜绝污染。
	if strings.EqualFold(node.Protocol, "vless") {
		flow := ""
		if (network == "" || network == "tcp") && (security == "reality" || security == "tls") {
			customFlow := ""
			if settingsMap != nil {
				if f, ok := settingsMap["flow"].(string); ok {
					customFlow = strings.TrimSpace(f)
				}
			}
			if strings.EqualFold(customFlow, "none") {
				flow = ""
			} else if customFlow != "" {
				flow = customFlow
			} else {
				flow = "xtls-rprx-vision"
			}

			if flow == "" && user.Flow != "" {
				flow = user.Flow
			}
		}
		if flow != "" {
			node.SetParam("flow", flow)
		}
	}

	// 5. Shadowsocks 加密方式
	switch strings.ToLower(strings.TrimSpace(node.Protocol)) {
	case "shadowsocks", "ss", "shadowsocks-2022", "ss-2022", "ss2022":
		method := "aes-128-gcm"
		if strings.Contains(strings.ToLower(node.Protocol), "2022") {
			method = "2022-blake3-aes-128-gcm"
		}
		if settingsMap != nil {
			if m, ok := settingsMap["method"].(string); ok && strings.TrimSpace(m) != "" {
				method = strings.TrimSpace(m)
			} else if c, ok := settingsMap["cipher"].(string); ok && strings.TrimSpace(c) != "" {
				method = strings.TrimSpace(c)
			}
		}
		node.SetParam("method", method)
	}

	// 6. Socks / HTTP 认证
	if strings.EqualFold(node.Protocol, "socks") || strings.EqualFold(node.Protocol, "http") {
		if settingsMap != nil {
			if accounts, ok := settingsMap["accounts"].([]interface{}); ok && len(accounts) > 0 {
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
