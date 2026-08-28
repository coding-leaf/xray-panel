package domain

type DNSConfig struct {
	Tag             string                 `json:"tag,omitempty"`
	ClientIP        string                 `json:"clientIp,omitempty"`
	QueryStrategy   string                 `json:"queryStrategy,omitempty"` // UseIP, UseIPv4, UseIPv6
	DisableCache    bool                   `json:"disableCache,omitempty"`
	DisableFallback bool                   `json:"disableFallback,omitempty"`
	Servers         []interface{}          `json:"servers"`
	Hosts           map[string]interface{} `json:"hosts,omitempty"`
}
