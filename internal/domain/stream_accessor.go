package domain

import (
	"encoding/json"
	"strings"

	"panel/internal/pkg/crypto"
)

// StreamSettingsDTO 传输层与安全层配置模型
type StreamSettingsDTO struct {
	Network         string                 `json:"network,omitempty"`  // tcp, xhttp, ws, grpc
	Security        string                 `json:"security,omitempty"` // reality, tls, none
	RealitySettings *RealitySettingsDTO    `json:"realitySettings,omitempty"`
	TLSSettings     *TLSSettingsDTO        `json:"tlsSettings,omitempty"`
	XHTTPSettings   *XHTTPSettingsDTO      `json:"xhttpSettings,omitempty"`
	WSSettings      *WSSettingsDTO         `json:"wsSettings,omitempty"`
	GRPCSettings    *GRPCSettingsDTO       `json:"grpcSettings,omitempty"`
}

type RealitySettingsDTO struct {
	Dest         string   `json:"dest,omitempty"`
	ServerNames  []string `json:"serverNames,omitempty"`
	ServerName   string   `json:"serverName,omitempty"`
	PrivateKey   string   `json:"privateKey,omitempty"`
	PublicKey    string   `json:"publicKey,omitempty"`
	ShortIds     []string `json:"shortIds,omitempty"`
	ShortId      string   `json:"shortId,omitempty"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
	SpiderX      string   `json:"spiderX,omitempty"`
}

type TLSSettingsDTO struct {
	ServerName string   `json:"serverName,omitempty"`
	ALPN       []string `json:"alpn,omitempty"`
}

type XHTTPSettingsDTO struct {
	Path    string            `json:"path,omitempty"`
	Mode    string            `json:"mode,omitempty"`
	Host    string            `json:"host,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type WSSettingsDTO struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Host    string            `json:"host,omitempty"`
}

type GRPCSettingsDTO struct {
	ServiceName string `json:"serviceName,omitempty"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

// InboundStreamAccessor 统一强类型解析与访问器，屏蔽弱类型 map[string]interface{}
type InboundStreamAccessor struct {
	StreamSettings *StreamSettingsDTO
	RawSettings    string
	settingsMap    map[string]interface{}
}

// NewInboundStreamAccessor 创建流配置访问器
func NewInboundStreamAccessor(streamSettingsJSON, settingsJSON string) *InboundStreamAccessor {
	accessor := &InboundStreamAccessor{
		RawSettings: settingsJSON,
	}

	streamStr := strings.TrimSpace(streamSettingsJSON)
	if streamStr != "" && streamStr != "null" && streamStr != "{}" {
		var s StreamSettingsDTO
		if err := json.Unmarshal([]byte(streamStr), &s); err == nil {
			accessor.StreamSettings = &s
		}
	}

	settingsStr := strings.TrimSpace(settingsJSON)
	if settingsStr != "" && settingsStr != "null" && settingsStr != "{}" {
		var sm map[string]interface{}
		if err := json.Unmarshal([]byte(settingsStr), &sm); err == nil {
			accessor.settingsMap = sm
		}
	}

	return accessor
}

// NewInboundStreamAccessorFromInbound 从 domain.Inbound 创建访问器
func NewInboundStreamAccessorFromInbound(inb *Inbound) *InboundStreamAccessor {
	if inb == nil {
		return NewInboundStreamAccessor("", "")
	}
	return NewInboundStreamAccessor(inb.StreamSettings, inb.SettingsJSON)
}

// Network 返回传输层网络类型，默认 "tcp"
func (a *InboundStreamAccessor) Network() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.Network == "" {
		return "tcp"
	}
	return strings.ToLower(strings.TrimSpace(a.StreamSettings.Network))
}

// Security 返回安全层类型，默认 "none"
func (a *InboundStreamAccessor) Security() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.Security == "" {
		return "none"
	}
	return strings.ToLower(strings.TrimSpace(a.StreamSettings.Security))
}

// ResolveVisionFlow 统一解析 Vision Flow (XTLS Vision)
// 规则：严格限定为 VLESS 协议且传输为 TCP (空或"tcp") + REALITY 或 TLS。其余组合一律为空。
func (a *InboundStreamAccessor) ResolveVisionFlow(protocol string) string {
	if a == nil || !strings.EqualFold(protocol, "vless") {
		return ""
	}

	net := a.Network()
	sec := a.Security()
	if (net != "" && net != "tcp") || (sec != "reality" && sec != "tls") {
		return ""
	}

	customFlow := ""
	if a.settingsMap != nil {
		if f, ok := a.settingsMap["flow"].(string); ok {
			customFlow = strings.TrimSpace(f)
		}
	}

	if strings.EqualFold(customFlow, "none") {
		return ""
	}
	if customFlow != "" {
		return customFlow
	}
	return "xtls-rprx-vision"
}

// ResolveShadowsocksMethod 统一提取 Shadowsocks 加密方法，默认 "aes-128-gcm"
func (a *InboundStreamAccessor) ResolveShadowsocksMethod() string {
	if a == nil || a.settingsMap == nil {
		return "aes-128-gcm"
	}
	if m, ok := a.settingsMap["method"].(string); ok && strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	if c, ok := a.settingsMap["cipher"].(string); ok && strings.TrimSpace(c) != "" {
		return strings.TrimSpace(c)
	}
	return "aes-128-gcm"
}

// GetRealityPublicKey 提取 REALITY 公钥，若缺失则尝试从私钥动态推导
func (a *InboundStreamAccessor) GetRealityPublicKey() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.RealitySettings == nil {
		return ""
	}
	r := a.StreamSettings.RealitySettings
	if r.PublicKey != "" {
		return r.PublicKey
	}
	if r.PrivateKey != "" {
		return crypto.DerivePublicKeyFromPrivate(r.PrivateKey)
	}
	return ""
}

// GetRealityServerName 提取 REALITY SNI / ServerName
func (a *InboundStreamAccessor) GetRealityServerName() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.RealitySettings == nil {
		return ""
	}
	r := a.StreamSettings.RealitySettings
	if r.ServerName != "" {
		return r.ServerName
	}
	if len(r.ServerNames) > 0 && r.ServerNames[0] != "" {
		return r.ServerNames[0]
	}
	return ""
}

// GetRealityShortID 提取 REALITY ShortId
func (a *InboundStreamAccessor) GetRealityShortID() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.RealitySettings == nil {
		return ""
	}
	r := a.StreamSettings.RealitySettings
	if r.ShortId != "" {
		return r.ShortId
	}
	if len(r.ShortIds) > 0 && r.ShortIds[0] != "" {
		return r.ShortIds[0]
	}
	return ""
}

// GetRealityFingerprint 提取 REALITY 指纹，默认 "chrome"
func (a *InboundStreamAccessor) GetRealityFingerprint() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.RealitySettings == nil {
		return "chrome"
	}
	r := a.StreamSettings.RealitySettings
	if r.Fingerprint != "" {
		return r.Fingerprint
	}
	return "chrome"
}

// GetRealitySpiderX 提取 REALITY SpiderX
func (a *InboundStreamAccessor) GetRealitySpiderX() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.RealitySettings == nil {
		return ""
	}
	return a.StreamSettings.RealitySettings.SpiderX
}

// GetTLSServerName 提取 TLS ServerName
func (a *InboundStreamAccessor) GetTLSServerName() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.TLSSettings == nil {
		return ""
	}
	return a.StreamSettings.TLSSettings.ServerName
}

// GetTLSALPN 提取 TLS ALPN 列表
func (a *InboundStreamAccessor) GetTLSALPN() []string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.TLSSettings == nil {
		return nil
	}
	return a.StreamSettings.TLSSettings.ALPN
}

// GetWSPath 提取 WebSocket Path
func (a *InboundStreamAccessor) GetWSPath() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.WSSettings == nil {
		return ""
	}
	return a.StreamSettings.WSSettings.Path
}

// GetWSHost 提取 WebSocket Host Header
func (a *InboundStreamAccessor) GetWSHost() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.WSSettings == nil {
		return ""
	}
	ws := a.StreamSettings.WSSettings
	if ws.Headers != nil {
		if h, ok := ws.Headers["Host"]; ok && h != "" {
			return h
		}
		if h, ok := ws.Headers["host"]; ok && h != "" {
			return h
		}
	}
	if ws.Host != "" {
		return ws.Host
	}
	return ""
}

// GetGRPCServiceName 提取 gRPC ServiceName
func (a *InboundStreamAccessor) GetGRPCServiceName() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.GRPCSettings == nil {
		return ""
	}
	return a.StreamSettings.GRPCSettings.ServiceName
}

// GetXHTTPPath 提取 xHTTP Path
func (a *InboundStreamAccessor) GetXHTTPPath() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.XHTTPSettings == nil {
		return ""
	}
	return a.StreamSettings.XHTTPSettings.Path
}

// GetXHTTPMode 提取 xHTTP Mode
func (a *InboundStreamAccessor) GetXHTTPMode() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.XHTTPSettings == nil {
		return ""
	}
	return a.StreamSettings.XHTTPSettings.Mode
}

// GetXHTTPHost 提取 xHTTP Host
func (a *InboundStreamAccessor) GetXHTTPHost() string {
	if a == nil || a.StreamSettings == nil || a.StreamSettings.XHTTPSettings == nil {
		return ""
	}
	if a.StreamSettings.XHTTPSettings.Host != "" {
		return a.StreamSettings.XHTTPSettings.Host
	}
	if a.StreamSettings.XHTTPSettings.Headers != nil {
		if h, ok := a.StreamSettings.XHTTPSettings.Headers["Host"]; ok && h != "" {
			return h
		}
		if h, ok := a.StreamSettings.XHTTPSettings.Headers["host"]; ok && h != "" {
			return h
		}
	}
	return ""
}

// GetSettingsMap 获取反序列化后的设置字典副本
func (a *InboundStreamAccessor) GetSettingsMap() map[string]interface{} {
	if a == nil || a.settingsMap == nil {
		return make(map[string]interface{})
	}
	clone := make(map[string]interface{}, len(a.settingsMap))
	for k, v := range a.settingsMap {
		clone[k] = v
	}
	return clone
}
