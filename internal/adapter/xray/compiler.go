package xray

import (
	"encoding/json"
	"fmt"
	"strings"

	"panel/internal/domain"
)

// XrayCompiler 强类型 Xray 配置单向编译器
type XrayCompiler struct{}

func NewXrayCompiler() *XrayCompiler {
	return &XrayCompiler{}
}

// Compile 将系统业务数据编译为合法的 Xray 官方根配置对象
func (c *XrayCompiler) Compile(
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
) (*XrayConfigFile, error) {
	cfg := &XrayConfigFile{
		Log: &XrayLogConfig{
			Access:   "/var/log/xray/access.log",
			Error:    "/var/log/xray/error.log",
			LogLevel: "warning",
		},
		API: &XrayAPIConfig{
			Tag:      "api",
			Services: []string{"HandlerService", "StatsService"},
		},
		Stats: &XrayStatsConfig{},
		Policy: &XrayPolicyConfig{
			System: &XrayPolicySystem{
				StatsInboundUplink:    true,
				StatsInboundDownlink:  true,
				StatsOutboundUplink:   true,
				StatsOutboundDownlink: true,
			},
		},
		Inbounds:  make([]XrayInbound, 0),
		Outbounds: make([]XrayOutbound, 0),
	}

	// 1. 编译 DNS
	if dns != nil {
		servers := dns.Servers
		if len(servers) == 0 {
			servers = []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}
		}
		qStrategy := dns.QueryStrategy
		if qStrategy == "" {
			qStrategy = "UseIP"
		}
		cfg.DNS = &XrayDNSConfig{
			Servers:       servers,
			QueryStrategy: qStrategy,
			Tag:           "dns-in",
		}
	}

	// 2. 编译 Inbounds & 注入用户 Clients
	for _, inb := range inbounds {
		if !inb.Enabled || inb.Tag == "api" {
			continue
		}
		compiledInbound, err := c.compileInbound(&inb, users)
		if err != nil {
			return nil, fmt.Errorf("compile inbound %s failed: %w", inb.Tag, err)
		}
		cfg.Inbounds = append(cfg.Inbounds, *compiledInbound)
	}

	// 注入默认 API Inbound (用于 gRPC 本地通信，默认 8080 端口)
	apiInboundSettings, _ := json.Marshal(map[string]interface{}{
		"address": "127.0.0.1",
	})
	cfg.Inbounds = append(cfg.Inbounds, XrayInbound{
		Tag:      "api",
		Listen:   "127.0.0.1",
		Port:     8080,
		Protocol: "dokodemo-door",
		Settings: apiInboundSettings,
	})

	// 3. 编译 Outbounds
	hasDirect := false
	hasBlock := false
	for _, ob := range outbounds {
		if ob.Tag == "direct" {
			hasDirect = true
		}
		if ob.Tag == "block" {
			hasBlock = true
		}
		compiledOb, err := c.compileOutbound(&ob)
		if err != nil {
			return nil, fmt.Errorf("compile outbound %s failed: %w", ob.Tag, err)
		}
		cfg.Outbounds = append(cfg.Outbounds, *compiledOb)
	}

	// 确保基础直连和黑洞存在
	if !hasDirect {
		cfg.Outbounds = append(cfg.Outbounds, XrayOutbound{
			Tag:      "direct",
			Protocol: "freedom",
		})
	}
	if !hasBlock {
		cfg.Outbounds = append(cfg.Outbounds, XrayOutbound{
			Tag:      "block",
			Protocol: "blackhole",
		})
	}

	// 4. 分层编译 Routing 规则 (Layer 1 ~ Layer 4)
	domainStrategy := "IPIfNonMatch"
	if routing != nil && routing.DomainStrategy != "" {
		domainStrategy = routing.DomainStrategy
	}

	var layeredRules []XrayRoutingRule

	// Layer 1: 系统内置 API 规则
	layeredRules = append(layeredRules, XrayRoutingRule{
		Type:        "field",
		InboundTag:  []string{"api"},
		OutboundTag: "api",
	})

	// Layer 2: 接入网关通道分流规则 (Scoped Scoped Rules - 协议级与单端口多出口)
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		for _, sr := range inb.GetSubRoutes() {
			if sr.Enabled && sr.RouteID > 0 && sr.OutboundTag != "" {
				layeredRules = append(layeredRules, XrayRoutingRule{
					Type:        "field",
					InboundTag:  []string{inb.Tag}, // 严格限定 Inbound 命名空间，防止冲突
					VlessRoute:  fmt.Sprintf("%d", sr.RouteID),
					OutboundTag: sr.OutboundTag,
				})
			}
		}
	}

	// Layer 3: 自定义全局分流规则 (用户在 Routing 视图维护的规则)
	if routing != nil {
		for _, r := range routing.Rules {
			// 过滤掉已被自动管理的 api 规则与 vlessRoute 规则
			if len(r.InboundTag) > 0 && r.InboundTag[0] == "api" {
				continue
			}
			if r.VlessRoute != "" {
				continue
			}
			layeredRules = append(layeredRules, XrayRoutingRule{
				Type:        "field",
				Tag:         r.Tag,
				InboundTag:  r.InboundTag,
				OutboundTag: r.OutboundTag,
				Domain:      r.Domain,
				IP:          r.IP,
				Port:        r.Port,
				Network:     r.Network,
				Protocol:    r.Protocol,
				Attrs:       r.Attrs,
			})
		}
	}

	cfg.Routing = &XrayRoutingConfig{
		DomainStrategy: domainStrategy,
		Rules:          layeredRules,
	}

	return cfg, nil
}

// CompileToJSON 编译并输出格式化美化的 JSON 字节流
func (c *XrayCompiler) CompileToJSON(
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
) ([]byte, error) {
	cfg, err := c.Compile(inbounds, outbounds, routing, dns, users)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "    ")
}

func (c *XrayCompiler) compileInbound(inb *domain.Inbound, users []domain.User) (*XrayInbound, error) {
	var streamSettings *XrayStreamSettings
	if inb.StreamSettings != "" {
		_ = json.Unmarshal([]byte(inb.StreamSettings), &streamSettings)
	}

	var sniffing *XraySniffingConfig
	if inb.SniffingJSON != "" {
		_ = json.Unmarshal([]byte(inb.SniffingJSON), &sniffing)
	}

	// 推导 Vision Flow (仅限 TCP + REALITY/TLS)
	inboundFlow := ""
	protocolLower := strings.ToLower(inb.Protocol)
	if protocolLower == "vless" && streamSettings != nil {
		net := streamSettings.Network
		sec := streamSettings.Security
		if (net == "" || net == "tcp") && (sec == "reality" || sec == "tls") {
			inboundFlow = "xtls-rprx-vision"
		}
	}

	// 收集并投影授权到该 Inbound 的用户 Clients
	var clients []XrayClient
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		if !u.HasInbound(inb.Tag) {
			continue
		}

		client := XrayClient{
			Email: u.Email,
			Level: 0,
		}

		switch protocolLower {
		case "vless":
			client.ID = u.UUID
			client.Flow = inboundFlow
		case "vmess":
			client.ID = u.UUID
		case "trojan", "shadowsocks":
			client.Password = u.UUID
		default:
			client.ID = u.UUID
		}

		clients = append(clients, client)
	}

	// 服务端 Inbound 移除 publicKey 字段以防止部分 Xray 内核版本解析报警
	if streamSettings != nil && streamSettings.RealitySettings != nil {
		streamSettings.RealitySettings.PublicKey = ""
	}

	// 组装 Settings
	var settingsMap map[string]interface{}
	if inb.SettingsJSON != "" {
		_ = json.Unmarshal([]byte(inb.SettingsJSON), &settingsMap)
	}
	if settingsMap == nil {
		settingsMap = make(map[string]interface{})
	}

	// 若当前用户库中暂未关联到此 Inbound，优先回退提取 settingsJson 中原有 clients，若仍为空则注入合规占位客户端
	if len(clients) == 0 {
		if rawClients, ok := settingsMap["clients"].([]interface{}); ok && len(rawClients) > 0 {
			// 保留原本存在于 settingsJson 的 clients
		} else {
			placeholderID := "00000000-0000-0000-0000-000000000001"
			clients = append(clients, XrayClient{
				ID:       placeholderID,
				Password: placeholderID,
				Email:    "default@panel.local",
				Flow:     inboundFlow,
				Level:    0,
			})
			settingsMap["clients"] = clients
		}
	} else {
		settingsMap["clients"] = clients
	}

	if protocolLower == "vless" && settingsMap["decryption"] == nil {
		settingsMap["decryption"] = "none"
	}

	settingsBytes, err := json.Marshal(settingsMap)
	if err != nil {
		return nil, err
	}

	listen := inb.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}

	return &XrayInbound{
		Tag:            inb.Tag,
		Listen:         listen,
		Port:           inb.Port,
		Protocol:       inb.Protocol,
		Settings:       settingsBytes,
		StreamSettings: streamSettings,
		Sniffing:       sniffing,
	}, nil
}

func (c *XrayCompiler) compileOutbound(ob *domain.Outbound) (*XrayOutbound, error) {
	var streamSettings *XrayStreamSettings
	if ob.StreamSettings != "" {
		_ = json.Unmarshal([]byte(ob.StreamSettings), &streamSettings)
	}

	var settingsBytes json.RawMessage
	if ob.SettingsJSON != "" {
		settingsBytes = json.RawMessage(ob.SettingsJSON)
	} else {
		settingsBytes = json.RawMessage("{}")
	}

	return &XrayOutbound{
		Tag:            ob.Tag,
		Protocol:       ob.Protocol,
		Settings:       settingsBytes,
		StreamSettings: streamSettings,
	}, nil
}
