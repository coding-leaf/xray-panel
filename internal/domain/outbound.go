package domain

type Outbound struct {
	Tag            string `json:"tag"`
	Protocol       string `json:"protocol"` // freedom, blackhole, wireguard, shadowsocks, vless, vmess, trojan, socks, http
	SendThrough    string `json:"sendThrough,omitempty"`
	TargetStrategy string `json:"targetStrategy,omitempty"`
	SettingsJSON   string `json:"settingsJson"`
	StreamSettings string `json:"streamSettings,omitempty"`
	Mux            string `json:"mux,omitempty"`
}

type RoutingRule struct {
	Tag           string   `json:"tag,omitempty"`
	Type          string   `json:"type,omitempty"` // field
	InboundTag    []string `json:"inboundTag,omitempty"`
	OutboundTag   string   `json:"outboundTag,omitempty"`
	BalancerTag   string   `json:"balancerTag,omitempty"`
	DomainMatcher string   `json:"domainMatcher,omitempty"`
	VlessRoute    string   `json:"vlessRoute,omitempty"` // 匹配 VLESS 协议级 16 位路由标识 (如 "1", "2", "1,2", "100-200")
	Domain        []string `json:"domain,omitempty"`
	IP            []string `json:"ip,omitempty"`
	Port          string   `json:"port,omitempty"`
	Network       string   `json:"network,omitempty"`
	Protocol      []string `json:"protocol,omitempty"`
	User          []string `json:"user,omitempty"`
	Attrs         string   `json:"attrs,omitempty"`
	Source        []string `json:"source,omitempty"`
	SourcePort    string   `json:"sourcePort,omitempty"`
}

type RoutingConfig struct {
	DomainStrategy string        `json:"domainStrategy"` // AsIs, IPIfNonMatch, IPOnDemand
	Rules          []RoutingRule `json:"rules"`
}
