package xray

import "encoding/json"

// XrayConfigFile 代表 Xray-core 官方根配置
type XrayConfigFile struct {
	Log       *XrayLogConfig      `json:"log,omitempty"`
	API       *XrayAPIConfig      `json:"api,omitempty"`
	Stats     *XrayStatsConfig    `json:"stats,omitempty"`
	Policy    *XrayPolicyConfig   `json:"policy,omitempty"`
	DNS       *XrayDNSConfig      `json:"dns,omitempty"`
	Routing   *XrayRoutingConfig  `json:"routing,omitempty"`
	Inbounds  []XrayInbound       `json:"inbounds"`
	Outbounds []XrayOutbound      `json:"outbounds"`
}

type XrayLogConfig struct {
	Access   string `json:"access,omitempty"`
	Error    string `json:"error,omitempty"`
	LogLevel string `json:"loglevel,omitempty"` // debug, info, warning, error, none
}

type XrayAPIConfig struct {
	Tag      string   `json:"tag"`
	Services []string `json:"services"`
}

type XrayStatsConfig struct{}

type XrayPolicyConfig struct {
	System *XrayPolicySystem `json:"system,omitempty"`
}

type XrayPolicySystem struct {
	StatsInboundUplink    bool `json:"statsInboundUplink,omitempty"`
	StatsInboundDownlink  bool `json:"statsInboundDownlink,omitempty"`
	StatsOutboundUplink   bool `json:"statsOutboundUplink,omitempty"`
	StatsOutboundDownlink bool `json:"statsOutboundDownlink,omitempty"`
}

type XrayClient struct {
	ID       string `json:"id,omitempty"`       // VLESS / VMess UUID
	Password string `json:"password,omitempty"` // Trojan / Shadowsocks
	Email    string `json:"email,omitempty"`
	Flow     string `json:"flow,omitempty"`     // xtls-rprx-vision
	Level    int    `json:"level,omitempty"`
}

type XrayInboundSettings struct {
	Clients    []XrayClient   `json:"clients,omitempty"`
	Decryption string         `json:"decryption,omitempty"`
	Fallbacks  []XrayFallback `json:"fallbacks,omitempty"`
}

type XrayFallback struct {
	Alpn string `json:"alpn,omitempty"`
	Path string `json:"path,omitempty"`
	Dest string `json:"dest"`
	Xver int    `json:"xver,omitempty"`
}

type XraySniffingConfig struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride,omitempty"` // ["http", "tls", "quic"]
	RouteOnly    bool     `json:"routeOnly,omitempty"`
}

type XrayStreamSettings struct {
	Network         string               `json:"network,omitempty"`  // tcp, xhttp, ws, grpc
	Security        string               `json:"security,omitempty"` // reality, tls, none
	RealitySettings *XrayRealitySettings `json:"realitySettings,omitempty"`
	TLSSettings     *XrayTLSSettings     `json:"tlsSettings,omitempty"`
	XHTTPSettings   *XrayXHTTPSettings   `json:"xhttpSettings,omitempty"`
	WSSettings      *XrayWSSettings      `json:"wsSettings,omitempty"`
	GRPCSettings    *XrayGRPCSettings    `json:"grpcSettings,omitempty"`
	Sockopt         *XraySockopt         `json:"sockopt,omitempty"`
}

type XrayRealitySettings struct {
	Show         bool     `json:"show,omitempty"`
	Dest         string   `json:"dest,omitempty"`
	Xver         int      `json:"xver,omitempty"`
	ServerNames  []string `json:"serverNames,omitempty"` // 服务端专属 (复数)
	ServerName   string   `json:"serverName,omitempty"`  // 客户端专属 (单数)
	PrivateKey   string   `json:"privateKey,omitempty"`  // 服务端专属
	PublicKey    string   `json:"publicKey,omitempty"`   // 客户端专属
	ShortIds     []string `json:"shortIds,omitempty"`    // 服务端专属 (复数)
	ShortId      string   `json:"shortId,omitempty"`     // 客户端专属 (单数)
	Fingerprint  string   `json:"fingerprint,omitempty"` // 客户端专属
	SpiderX      string   `json:"spiderX,omitempty"`     // 客户端专属
	MinClientVer string   `json:"minClientVer,omitempty"`
	MaxClientVer string   `json:"maxClientVer,omitempty"`
	MaxTimeDiff  int      `json:"maxTimeDiff,omitempty"`
}

type XrayTLSSettings struct {
	ServerName   string            `json:"serverName,omitempty"`
	Certificates []XrayCertificate `json:"certificates,omitempty"`
	ALPN         []string          `json:"alpn,omitempty"`
}

type XrayCertificate struct {
	CertificateFile string `json:"certificateFile,omitempty"`
	KeyFile         string `json:"keyFile,omitempty"`
}

type XrayXHTTPSettings struct {
	Path string `json:"path,omitempty"`
	Mode string `json:"mode,omitempty"` // auto, stream-up, stream-one, packet-up
	Host string `json:"host,omitempty"`
}

type XrayWSSettings struct {
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type XrayGRPCSettings struct {
	ServiceName string `json:"serviceName,omitempty"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

type XraySockopt struct {
	Mark        int    `json:"mark,omitempty"`
	Tproxy      string `json:"tproxy,omitempty"`
	TCPFastOpen bool   `json:"tcpFastOpen,omitempty"`
}

type XrayInbound struct {
	Tag            string              `json:"tag"`
	Listen         string              `json:"listen,omitempty"`
	Port           int                 `json:"port"`
	Protocol       string              `json:"protocol"`
	Settings       json.RawMessage     `json:"settings,omitempty"`
	StreamSettings *XrayStreamSettings `json:"streamSettings,omitempty"`
	Sniffing       *XraySniffingConfig `json:"sniffing,omitempty"`
}

type XrayOutbound struct {
	Tag            string              `json:"tag"`
	Protocol       string              `json:"protocol"`
	Settings       json.RawMessage     `json:"settings,omitempty"`
	StreamSettings *XrayStreamSettings `json:"streamSettings,omitempty"`
}

type XrayRoutingRule struct {
	Type        string   `json:"type"`                 // field
	Tag         string   `json:"tag,omitempty"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
	VlessRoute  string   `json:"vlessRoute,omitempty"` // 16-bit 协议级路由
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        string   `json:"port,omitempty"`
	Network     string   `json:"network,omitempty"`
	Protocol    []string `json:"protocol,omitempty"`
	Attrs       string   `json:"attrs,omitempty"`
}

type XrayRoutingConfig struct {
	DomainStrategy string            `json:"domainStrategy,omitempty"` // AsIs, IPIfNonMatch, IPOnDemand
	Rules          []XrayRoutingRule `json:"rules"`
}

type XrayDNSConfig struct {
	Servers       []interface{} `json:"servers,omitempty"`
	QueryStrategy string        `json:"queryStrategy,omitempty"`
	Tag           string        `json:"tag,omitempty"`
}
