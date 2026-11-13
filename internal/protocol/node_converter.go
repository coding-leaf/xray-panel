package protocol

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"panel/internal/domain"
	"panel/internal/pkg/crypto"
)

var (
	defaultFpMu        sync.RWMutex
	defaultFingerprint = "chrome"
)

// SetDefaultFingerprint 设置全局默认 TLS/REALITY 伪装指纹
func SetDefaultFingerprint(fp string) {
	defaultFpMu.Lock()
	defer defaultFpMu.Unlock()
	if strings.TrimSpace(fp) != "" {
		defaultFingerprint = strings.TrimSpace(fp)
	} else {
		defaultFingerprint = "chrome"
	}
}

// GetDefaultFingerprint 获取全局默认 TLS/REALITY 伪装指纹
func GetDefaultFingerprint() string {
	defaultFpMu.RLock()
	defer defaultFpMu.RUnlock()
	return defaultFingerprint
}

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
		ServerName  string   `json:"serverName"`
		ALPN        []string `json:"alpn"`
		Fingerprint string   `json:"fingerprint"`
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
	HTTPUpgradeSettings *struct {
		Path    string            `json:"path"`
		Host    string            `json:"host"`
		Headers map[string]string `json:"headers"`
	} `json:"httpupgradeSettings"`
	KCPSettings *struct {
		Header *struct {
			Type string `json:"type"`
		} `json:"header"`
		Seed string `json:"seed"`
	} `json:"kcpSettings"`
}

// InboundToNodeConfig 将 domain.Inbound 与 domain.User 转换为通用的 protocol.NodeConfig
func InboundToNodeConfig(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) *NodeConfig {
	return InboundToNodeConfigWithDefaultFP(inbound, user, hostDomain, defaultPort, GetDefaultFingerprint())
}

// InboundToNodeConfigWithDefaultFP 无状态转换 Inbound 与 User 为 NodeConfig，显式传入默认指纹以避免全局状态竞态
func InboundToNodeConfigWithDefaultFP(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int, defaultFP string) *NodeConfig {
	if inbound == nil || strings.EqualFold(inbound.Protocol, "dokodemo-door") {
		return nil
	}
	if user == nil && !strings.EqualFold(inbound.Protocol, "trojan") {
		return nil
	}

	resolvedDefaultFP := strings.TrimSpace(defaultFP)
	if resolvedDefaultFP == "" {
		resolvedDefaultFP = "chrome"
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

	effectiveUUID := ""
	if user != nil {
		effectiveUUID = user.UUID
	}
	if strings.EqualFold(inbound.Protocol, "trojan") {
		if pwd, ok := settingsMap["password"].(string); ok && strings.TrimSpace(pwd) != "" {
			effectiveUUID = strings.TrimSpace(pwd)
		}
	}
	if inbound.RouteID > 0 && (inbound.Protocol == "" || strings.EqualFold(inbound.Protocol, "vless")) && user != nil {
		effectiveUUID = ApplyVlessRouteToUUID(user.UUID, inbound.RouteID)
	}
	if effectiveUUID == "" {
		return nil
	}

	remark := inbound.Remark
	if remark == "" {
		remark = inbound.Tag
	}

	node := NewNodeConfig(remark, inbound.Protocol, targetHost, targetPort, effectiveUUID)
	node.SetParam("tag", inbound.Tag)
	node.SetParam("remark", inbound.Remark)
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
		if fp := strings.TrimSpace(rawStream.TLSSettings.Fingerprint); fp != "" {
			node.SetParam("fp", fp)
		} else {
			node.SetParam("fp", resolvedDefaultFP)
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
		if fp := strings.TrimSpace(r.Fingerprint); fp != "" {
			node.SetParam("fp", fp)
		} else {
			node.SetParam("fp", resolvedDefaultFP)
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
	case "httpupgrade":
		if rawStream.HTTPUpgradeSettings != nil {
			if rawStream.HTTPUpgradeSettings.Path != "" {
				node.SetParam("path", rawStream.HTTPUpgradeSettings.Path)
			}
			host := rawStream.HTTPUpgradeSettings.Host
			if host == "" && rawStream.HTTPUpgradeSettings.Headers != nil {
				if h, ok := rawStream.HTTPUpgradeSettings.Headers["Host"]; ok && h != "" {
					host = h
				} else if h, ok := rawStream.HTTPUpgradeSettings.Headers["host"]; ok && h != "" {
					host = h
				}
			}
			if host != "" {
				node.SetParam("host", host)
			}
		}
	case "kcp", "mkcp":
		if rawStream.KCPSettings != nil {
			if rawStream.KCPSettings.Header != nil && rawStream.KCPSettings.Header.Type != "" {
				node.SetParam("headerType", rawStream.KCPSettings.Header.Type)
			}
			if rawStream.KCPSettings.Seed != "" {
				node.SetParam("seed", rawStream.KCPSettings.Seed)
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
			uFlow := ""
			if user != nil {
				uFlow = strings.TrimSpace(user.Flow)
			}
			if strings.EqualFold(uFlow, "none") {
				flow = ""
			} else if uFlow != "" {
				flow = uFlow
			} else if strings.EqualFold(customFlow, "none") {
				flow = ""
			} else if customFlow != "" {
				flow = customFlow
			} else {
				flow = "xtls-rprx-vision"
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
func InboundsToNodeConfigs(inbounds []domain.Inbound, user *domain.User, hostDomain string, defaultPort int, tagFilter string, routeFilter ...uint16) []*NodeConfig {
	return InboundsToNodeConfigsWithDefaultFP(inbounds, user, hostDomain, defaultPort, tagFilter, GetDefaultFingerprint(), routeFilter...)
}

// InboundsToNodeConfigsWithDefaultFP 无状态转换一组 Inbound 和 User 为 []*NodeConfig，显式传入默认指纹以避免全局状态竞态
func InboundsToNodeConfigsWithDefaultFP(inbounds []domain.Inbound, user *domain.User, hostDomain string, defaultPort int, tagFilter string, defaultFP string, routeFilter ...uint16) []*NodeConfig {
	var targetRoute uint16
	if len(routeFilter) > 0 {
		targetRoute = routeFilter[0]
	}

	sortedInbounds := make([]domain.Inbound, len(inbounds))
	copy(sortedInbounds, inbounds)
	sort.SliceStable(sortedInbounds, func(i, j int) bool {
		if sortedInbounds[i].SortOrder != sortedInbounds[j].SortOrder {
			return sortedInbounds[i].SortOrder < sortedInbounds[j].SortOrder
		}
		return sortedInbounds[i].ID < sortedInbounds[j].ID
	})

	var nodes []*NodeConfig
	for _, in := range sortedInbounds {
		if !in.Enabled || !user.HasInbound(in.Tag) || strings.EqualFold(in.Protocol, "dokodemo-door") {
			continue
		}
		if tagFilter != "" && in.Tag != tagFilter {
			continue
		}

		isVless := in.Protocol == "" || strings.EqualFold(in.Protocol, "vless")
		subRoutes := in.GetSubRoutes()

		// 协议约束：SubRoute 仅限 VLESS 协议
		if len(subRoutes) == 0 || !isVless {
			if targetRoute > 0 {
				continue
			}
			if node := InboundToNodeConfigWithDefaultFP(&in, user, hostDomain, defaultPort, defaultFP); node != nil {
				nodes = append(nodes, node)
			}
			continue
		}

		// 对 SubRoutes 按 SortOrder ASC 确定性排序
		sort.SliceStable(subRoutes, func(i, j int) bool {
			return subRoutes[i].SortOrder < subRoutes[j].SortOrder
		})

		baseName := in.Remark
		if baseName == "" {
			baseName = in.Tag
		}

		for _, sr := range subRoutes {
			if !sr.Enabled {
				continue
			}
			if targetRoute > 0 && sr.RouteID != targetRoute {
				continue
			}
			if user != nil && !sr.CanAccess(user.Email) {
				continue
			}
			tempInbound := in
			if sr.Name != "" {
				tempInbound.Remark = sr.Name
			} else {
				tempInbound.Remark = fmt.Sprintf("%s - 线路 #%d", baseName, sr.RouteID)
			}
			tempInbound.RouteID = sr.RouteID
			if node := InboundToNodeConfigWithDefaultFP(&tempInbound, user, hostDomain, defaultPort, defaultFP); node != nil {
				if sr.Name != "" {
					node.SetParam("remark", sr.Name)
				}
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

// RenderNodeName 根据模板渲染单个节点名称
// 支持占位符: {remark}, {tag}, {protocol}, {port}, {index}
func RenderNodeName(template string, node *NodeConfig, index int) string {
	if template == "" {
		template = "{remark}"
	}
	remark := node.GetParam("remark", node.Name)
	tag := node.GetParam("tag")
	if remark == "" {
		remark = tag
	}
	if tag == "" {
		tag = remark
	}
	protoUpper := strings.ToUpper(node.Protocol)
	portStr := fmt.Sprintf("%d", node.Port)
	indexStr := fmt.Sprintf("%d", index)

	res := template
	res = strings.ReplaceAll(res, "{remark}", remark)
	res = strings.ReplaceAll(res, "{tag}", tag)
	res = strings.ReplaceAll(res, "{protocol}", protoUpper)
	res = strings.ReplaceAll(res, "{port}", portStr)
	res = strings.ReplaceAll(res, "{index}", indexStr)
	return res
}

// ApplyNodeNameTemplate 批量将模板应用到节点列表名称
func ApplyNodeNameTemplate(nodes []*NodeConfig, template string) {
	if template == "" || template == "{remark}" {
		return
	}
	for i, node := range nodes {
		if node == nil {
			continue
		}
		node.Name = RenderNodeName(template, node, i+1)
	}
}

// BuildShareLink 生成标准 Xray 分享链接 (基于 protocol.Registry 策略模式)
func BuildShareLink(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) string {
	return BuildShareLinkWithDefaultFP(inbound, user, hostDomain, defaultPort, GetDefaultFingerprint())
}

// BuildShareLinkWithDefaultFP 无状态生成标准 Xray 分享链接，显式传入默认指纹以避免全局状态竞态
func BuildShareLinkWithDefaultFP(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int, defaultFP string) string {
	node := InboundToNodeConfigWithDefaultFP(inbound, user, hostDomain, defaultPort, defaultFP)
	if node == nil {
		return ""
	}
	link, err := FormatLink(node)
	if err != nil {
		return ""
	}
	return link
}
