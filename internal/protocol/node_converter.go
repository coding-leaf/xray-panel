package protocol

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"panel/internal/domain"

	"golang.org/x/crypto/curve25519"
)

type RealityKeyPair struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	ShortID    string `json:"shortId"`
}

// GenerateRealityKeyPair 生成符合 Xray/VLESS 规范的 x25519 Reality 密钥对与 ShortId
func GenerateRealityKeyPair() (*RealityKeyPair, error) {
	var privKey [32]byte
	if _, err := rand.Read(privKey[:]); err != nil {
		return nil, fmt.Errorf("read random bytes failed: %w", err)
	}

	// 限制私钥位以符合 Curve25519 规范
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64

	var pubKey [32]byte
	curve25519.ScalarBaseMult(&pubKey, &privKey)

	privStr := base64.RawURLEncoding.EncodeToString(privKey[:])
	pubStr := base64.RawURLEncoding.EncodeToString(pubKey[:])

	var shortBytes [8]byte
	_, _ = rand.Read(shortBytes[:])
	shortID := hex.EncodeToString(shortBytes[:])

	return &RealityKeyPair{
		PrivateKey: privStr,
		PublicKey:  pubStr,
		ShortID:    shortID,
	}, nil
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

// ApplyVlessRouteToUUID 将 routeID 编码进 UUID 的第 3 组字段 (例如 7117295b-4362-0001-a133-b969344dfcd5)
func ApplyVlessRouteToUUID(rawUUID string, routeID uint16) string {
	if routeID == 0 || rawUUID == "" {
		return rawUUID
	}
	parts := strings.Split(rawUUID, "-")
	if len(parts) != 5 {
		return rawUUID
	}
	parts[2] = fmt.Sprintf("%04x", routeID)
	return strings.Join(parts, "-")
}

// DerivePublicKeyFromPrivate 从 Reality base64 私钥自动推导 x25519 对应公钥
func DerivePublicKeyFromPrivate(privStr string) string {
	if privStr == "" {
		return ""
	}
	privBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(privStr, "="))
	if err != nil {
		privBytes, err = base64.StdEncoding.DecodeString(privStr)
	}
	if err != nil || len(privBytes) != 32 {
		return ""
	}
	var privKey, pubKey [32]byte
	copy(privKey[:], privBytes)
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64
	curve25519.ScalarBaseMult(&pubKey, &privKey)
	return base64.RawURLEncoding.EncodeToString(pubKey[:])
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

	var streamMap map[string]interface{}
	_ = json.Unmarshal([]byte(inbound.StreamSettings), &streamMap)

	network := "tcp"
	if netVal, ok := streamMap["network"].(string); ok && netVal != "" {
		network = netVal
	}

	security := "none"
	if secVal, ok := streamMap["security"].(string); ok && secVal != "" {
		security = secVal
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

	// 提取 TLS / REALITY 关键配置
	if tlsSettings, ok := streamMap["tlsSettings"].(map[string]interface{}); ok {
		if serverName, ok := tlsSettings["serverName"].(string); ok && serverName != "" {
			node.SetParam("sni", serverName)
		}
		if alpnArr, ok := tlsSettings["alpn"].([]interface{}); ok && len(alpnArr) > 0 {
			var alpns []string
			for _, a := range alpnArr {
				alpns = append(alpns, fmt.Sprintf("%v", a))
			}
			node.SetParam("alpn", strings.Join(alpns, ","))
		}
	}

	if realitySettings, ok := streamMap["realitySettings"].(map[string]interface{}); ok {
		if pbk, ok := realitySettings["publicKey"].(string); ok && pbk != "" {
			node.SetParam("pbk", pbk)
		} else if privKey, ok := realitySettings["privateKey"].(string); ok && privKey != "" {
			node.SetParam("pbk", DerivePublicKeyFromPrivate(privKey))
		}

		if sn, ok := realitySettings["serverName"].(string); ok && sn != "" {
			node.SetParam("sni", sn)
		} else if serverNames, ok := realitySettings["serverNames"].([]interface{}); ok && len(serverNames) > 0 {
			node.SetParam("sni", fmt.Sprintf("%v", serverNames[0]))
		}

		if sid, ok := realitySettings["shortId"].(string); ok && sid != "" {
			node.SetParam("sid", sid)
		} else if shortIds, ok := realitySettings["shortIds"].([]interface{}); ok && len(shortIds) > 0 {
			for _, sidRaw := range shortIds {
				sidStr := fmt.Sprintf("%v", sidRaw)
				if sidStr != "" {
					node.SetParam("sid", sidStr)
					break
				}
			}
		}

		if spx, ok := realitySettings["spiderX"].(string); ok && spx != "" {
			node.SetParam("spx", spx)
		}
		if fp, ok := realitySettings["fingerprint"].(string); ok && fp != "" {
			node.SetParam("fp", fp)
		} else {
			node.SetParam("fp", "chrome")
		}
	}

	// 提取 WebSocket / gRPC / xHTTP 传输层参数
	if wsSettings, ok := streamMap["wsSettings"].(map[string]interface{}); ok {
		if path, ok := wsSettings["path"].(string); ok && path != "" {
			node.SetParam("path", path)
		}
		if headers, ok := wsSettings["headers"].(map[string]interface{}); ok {
			if h, ok := headers["Host"].(string); ok && h != "" {
				node.SetParam("host", h)
			} else if h, ok := headers["host"].(string); ok && h != "" {
				node.SetParam("host", h)
			}
		}
		if node.GetParam("host") == "" {
			if h, ok := wsSettings["host"].(string); ok && h != "" {
				node.SetParam("host", h)
			}
		}
	}

	if grpcSettings, ok := streamMap["grpcSettings"].(map[string]interface{}); ok {
		if serviceName, ok := grpcSettings["serviceName"].(string); ok && serviceName != "" {
			node.SetParam("serviceName", serviceName)
		}
	}

	if xhttpSettings, ok := streamMap["xhttpSettings"].(map[string]interface{}); ok {
		if p, ok := xhttpSettings["path"].(string); ok && p != "" {
			node.SetParam("path", p)
		}
		if m, ok := xhttpSettings["mode"].(string); ok && m != "" {
			node.SetParam("mode", m)
		}
		if h, ok := xhttpSettings["host"].(string); ok && h != "" {
			node.SetParam("host", h)
		} else if headers, ok := xhttpSettings["headers"].(map[string]interface{}); ok {
			if h, ok := headers["Host"].(string); ok && h != "" {
				node.SetParam("host", h)
			} else if h, ok := headers["host"].(string); ok && h != "" {
				node.SetParam("host", h)
			}
		}
	}

	// 提取 SettingsJSON (flow / method)
	var settingsMap map[string]interface{}
	_ = json.Unmarshal([]byte(inbound.SettingsJSON), &settingsMap)
	if f, ok := settingsMap["flow"].(string); ok {
		node.SetParam("flow", f)
	} else if clients, ok := settingsMap["clients"].([]interface{}); ok && len(clients) > 0 {
		if cMap, ok := clients[0].(map[string]interface{}); ok {
			if f, ok := cMap["flow"].(string); ok {
				node.SetParam("flow", f)
			}
		}
	}
	if node.GetParam("flow") == "" && user.Flow != "" {
		node.SetParam("flow", user.Flow)
	}

	// 协议与传输层强约束：flow (XTLS Vision) 仅限 VLESS 协议且传输为 TCP + (REALITY 或 TLS)。
	// 对于非 TCP 传输（如 xhttp, splithttp, ws, grpc）或非 TLS/REALITY，强制剔除 flow 参数杜绝污染。
	netLower := strings.ToLower(node.GetParam("type"))
	secLower := strings.ToLower(node.GetParam("security"))
	if strings.ToLower(node.Protocol) == "vless" {
		if (netLower != "tcp" && netLower != "") || (secLower != "reality" && secLower != "tls") {
			delete(node.Params, "flow")
		}
	} else {
		delete(node.Params, "flow")
	}

	if method, ok := settingsMap["method"].(string); ok && method != "" {
		node.SetParam("method", method)
	}

	// 提取 Socks / HTTP 认证信息
	if strings.ToLower(node.Protocol) == "socks" || strings.ToLower(node.Protocol) == "http" {
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
