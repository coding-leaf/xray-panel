package domain

type Outbound struct {
	Tag          string `json:"tag"`
	Protocol     string `json:"protocol"` // freedom, blackhole, wireguard, shadowsocks, vless, vmess, trojan, socks, http
	SettingsJSON string `json:"settingsJson"`
	StreamSettings string `json:"streamSettings,omitempty"`
}

type RoutingRule struct {
	Tag         string   `json:"tag,omitempty"`
	Type        string   `json:"type,omitempty"` // field
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        string   `json:"port,omitempty"`
	Network     string   `json:"network,omitempty"`
	Protocol    []string `json:"protocol,omitempty"`
	Attrs       string   `json:"attrs,omitempty"`
}

type RoutingConfig struct {
	DomainStrategy string        `json:"domainStrategy"` // AsIs, IPIfNonMatch, IPOnDemand
	Rules          []RoutingRule `json:"rules"`
}
