package xray

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"panel/internal/domain"
)

// XrayCompiler 强类型 Xray 配置单向编译器
type XrayCompiler struct {
	grpcPort int
	registry *ProtocolRegistry
	logLevel string
	mu       sync.RWMutex
}

func NewXrayCompiler(grpcPort ...int) *XrayCompiler {
	port := 8080
	if len(grpcPort) > 0 && grpcPort[0] > 0 {
		port = grpcPort[0]
	}
	return &XrayCompiler{
		grpcPort: port,
		registry: DefaultProtocolRegistry(),
		logLevel: "warning",
	}
}

// SetLogLevel 设置 Xray 核心运行日志等级（支持 debug, info, warning, error, none；缺省/非法回退为 warning）
func (c *XrayCompiler) SetLogLevel(lvl string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	lvl = strings.ToLower(strings.TrimSpace(lvl))
	switch lvl {
	case "debug", "info", "warning", "error", "none":
		c.logLevel = lvl
	case "warn":
		c.logLevel = "warning"
	default:
		c.logLevel = "warning"
	}
}

// GetLogLevel 获取当前配置的 Xray 核心运行日志等级，默认 warning
func (c *XrayCompiler) GetLogLevel() string {
	if c == nil {
		return "warning"
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.logLevel == "" {
		return "warning"
	}
	return c.logLevel
}

// WithProtocolRegistry 配置自定义协议适配器注册中心
func (c *XrayCompiler) WithProtocolRegistry(reg *ProtocolRegistry) *XrayCompiler {
	c.registry = reg
	return c
}

func (c *XrayCompiler) getRegistry() *ProtocolRegistry {
	if c != nil && c.registry != nil {
		return c.registry
	}
	return DefaultProtocolRegistry()
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
			LogLevel: c.GetLogLevel(),
		},
		API: &XrayAPIConfig{
			Tag:      "api",
			Services: []string{"HandlerService", "StatsService"},
		},
		Stats: &XrayStatsConfig{},
		Policy: &XrayPolicyConfig{
			Levels: map[string]XrayPolicyLevel{
				"0": {
					StatsUserUplink:   true,
					StatsUserDownlink: true,
				},
			},
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

	// 注入默认 API Inbound (用于 gRPC 本地通信，动态端口或默认 8080)
	apiInboundSettings, _ := json.Marshal(map[string]interface{}{
		"address": "127.0.0.1",
	})
	apiPort := 8080
	if c != nil && c.grpcPort > 0 {
		apiPort = c.grpcPort
	}
	cfg.Inbounds = append(cfg.Inbounds, XrayInbound{
		Tag:      "api",
		Listen:   "127.0.0.1",
		Port:     apiPort,
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

	// 4. 分层编译 Routing 规则 (Layer 1 核心通信 + Layer 2 统一规则池单一事实源)
	domainStrategy := "IPIfNonMatch"
	if routing != nil && routing.DomainStrategy != "" {
		domainStrategy = routing.DomainStrategy
	}

	var layeredRules []XrayRoutingRule

	// Layer 1: 系统内置核心 API 规则 (仅保障面板本地 gRPC 调度)
	layeredRules = append(layeredRules, XrayRoutingRule{
		Type:        "field",
		InboundTag:  []string{"api"},
		OutboundTag: "api",
	})

	// Layer 2: 统一分流规则池 (完全放权、单一事实源，忠实按序输出并做幂等去重保护)
	if routing != nil {
		seen := make(map[string]bool)
		for _, r := range routing.Rules {
			// 过滤掉已被自动管理的 api 规则，防止重复
			if (len(r.InboundTag) > 0 && r.InboundTag[0] == "api") || r.OutboundTag == "api" {
				continue
			}
			ruleType := r.Type
			if ruleType == "" {
				ruleType = "field"
			}
			rule := XrayRoutingRule{
				Type:        ruleType,
				Tag:         r.Tag,
				InboundTag:  r.InboundTag,
				OutboundTag: r.OutboundTag,
				VlessRoute:  r.VlessRoute,
				Domain:      r.Domain,
				IP:          r.IP,
				Port:        r.Port,
				Network:     r.Network,
				Protocol:    r.Protocol,
				User:        r.User,
				Attrs:       r.Attrs,
			}
			keyBytes, _ := json.Marshal(rule)
			key := string(keyBytes)
			if seen[key] {
				continue
			}
			seen[key] = true
			layeredRules = append(layeredRules, rule)
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
	accessor := NewInboundStreamAccessorFromInbound(inb)

	// 服务端 Inbound REALITY 字段规范化清洗与严格校验 (杜绝 Xray 内核 non-empty serverName 报错及假兜底)
	if streamSettings != nil {
		if strings.EqualFold(streamSettings.Security, "reality") {
			if streamSettings.RealitySettings == nil {
				return nil, fmt.Errorf("%w: realitySettings is required when security is reality", domain.ErrInvalidInput)
			}
			r := streamSettings.RealitySettings
			if len(r.ServerNames) == 0 && r.ServerName != "" {
				r.ServerNames = []string{r.ServerName}
			}
			r.ServerName = ""  // 服务端严禁包含单数 serverName
			r.PublicKey = ""   // 服务端严禁包含 publicKey
			r.Fingerprint = "" // 服务端严禁包含 fingerprint
			r.SpiderX = ""     // 服务端严禁包含 spiderX
			if len(r.ShortIds) == 0 && r.ShortId != "" {
				r.ShortIds = []string{r.ShortId}
			}
			r.ShortId = "" // 服务端严禁包含单数 shortId
			if r.Dest == "" && r.Target != "" {
				r.Dest = r.Target
			}
			r.Target = "" // 服务端严禁包含 target 兼容字段

			r.Dest = strings.TrimSpace(r.Dest)
			if r.Dest == "" {
				return nil, fmt.Errorf("%w: reality dest is required", domain.ErrInvalidInput)
			}
			host, portStr, err := net.SplitHostPort(r.Dest)
			if err != nil || host == "" || portStr == "" {
				return nil, fmt.Errorf("%w: reality dest must be in host:port format, got %q", domain.ErrInvalidInput, r.Dest)
			}
			portNum, err := strconv.Atoi(portStr)
			if err != nil || portNum <= 0 || portNum > 65535 {
				return nil, fmt.Errorf("%w: reality dest has invalid port %q", domain.ErrInvalidInput, portStr)
			}

			var validSN []string
			for _, sn := range r.ServerNames {
				sn = strings.TrimSpace(sn)
				if sn != "" {
					validSN = append(validSN, sn)
				}
			}
			r.ServerNames = validSN
			if len(r.ServerNames) == 0 {
				return nil, fmt.Errorf("%w: reality serverNames is required and must contain at least one non-empty server name", domain.ErrInvalidInput)
			}

			r.PrivateKey = strings.TrimSpace(r.PrivateKey)
			if r.PrivateKey == "" {
				return nil, fmt.Errorf("%w: reality privateKey is required", domain.ErrInvalidInput)
			}
		} else {
			streamSettings.RealitySettings = nil
		}
	}

	// 组装 Settings
	var settingsMap map[string]interface{}
	if inb.SettingsJSON != "" {
		_ = json.Unmarshal([]byte(inb.SettingsJSON), &settingsMap)
	}
	if settingsMap == nil {
		settingsMap = make(map[string]interface{})
	}

	adapter, hasAdapter := c.getRegistry().ResolveAdapter(inb)
	if hasAdapter {
		clients, err := adapter.CompileClients(inb, users, accessor)
		if err != nil {
			return nil, err
		}
		if clients != nil {
			settingsMap["clients"] = clients
		}
		adapter.DecorateSettings(inb, settingsMap)
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
	streamStr := strings.TrimSpace(ob.StreamSettings)
	if streamStr != "" && streamStr != "null" && streamStr != "{}" {
		_ = json.Unmarshal([]byte(streamStr), &streamSettings)
	}

	// 客户端 Outbound REALITY 字段规范化清洗 (杜绝 Xray 内核 non-empty serverNames 报错)
	if streamSettings != nil && streamSettings.RealitySettings != nil {
		r := streamSettings.RealitySettings
		if r.ServerName == "" && len(r.ServerNames) > 0 {
			r.ServerName = r.ServerNames[0]
		}
		r.ServerNames = nil // 客户端严禁包含复数 serverNames
		r.PrivateKey = ""   // 客户端严禁包含 privateKey
		r.Dest = ""         // 客户端严禁包含 dest
		r.Target = ""       // 客户端严禁包含 target
		if r.ShortId == "" && len(r.ShortIds) > 0 {
			r.ShortId = r.ShortIds[0]
		}
		r.ShortIds = nil // 客户端严禁包含复数 shortIds
	}

	settingsStr := strings.TrimSpace(ob.SettingsJSON)
	if settingsStr == "" || settingsStr == "null" {
		settingsStr = "{}"
	}

	// 针对 VLESS 出站清洗顶层 settings.flow（Xray 客户端 VLessOutboundConfig 仅支持 "" 或有效 flow，杜绝 "none"）
	if strings.EqualFold(ob.Protocol, "vless") && settingsStr != "{}" {
		var sm map[string]interface{}
		if err := json.Unmarshal([]byte(settingsStr), &sm); err == nil {
			if f, ok := sm["flow"].(string); ok && !strings.EqualFold(f, "xtls-rprx-vision") && !strings.EqualFold(f, "xtls-rprx-vision-udp443") {
				delete(sm, "flow")
				if b, err := json.Marshal(sm); err == nil {
					settingsStr = string(b)
				}
			}
		}
	}

	return &XrayOutbound{
		Tag:            ob.Tag,
		Protocol:       ob.Protocol,
		Settings:       json.RawMessage(settingsStr),
		StreamSettings: streamSettings,
	}, nil
}
