package xray

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
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

func newBaseXrayConfig(logLevel string) *XrayConfigFile {
	return &XrayConfigFile{
		Log: &XrayLogConfig{
			Access:   "/var/log/xray/access.log",
			Error:    "/var/log/xray/error.log",
			LogLevel: logLevel,
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
}

func appendAPIInbound(cfg *XrayConfigFile, apiPort int) {
	if apiPort <= 0 {
		apiPort = 8080
	}
	apiInboundSettings, _ := json.Marshal(map[string]interface{}{
		"address": "127.0.0.1",
	})
	cfg.Inbounds = append(cfg.Inbounds, XrayInbound{
		Tag:      "api",
		Listen:   "127.0.0.1",
		Port:     apiPort,
		Protocol: "dokodemo-door",
		Settings: apiInboundSettings,
	})
}

func compileDNSConfig(dns *domain.DNSConfig) *XrayDNSConfig {
	if dns == nil {
		return nil
	}
	servers := dns.Servers
	if len(servers) == 0 {
		servers = []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}
	}
	qStrategy := dns.QueryStrategy
	if qStrategy == "" {
		qStrategy = "UseIP"
	}
	tag := dns.Tag
	if tag == "" {
		tag = "dns-in"
	}
	return &XrayDNSConfig{
		Tag:             tag,
		ClientIP:        dns.ClientIP,
		QueryStrategy:   qStrategy,
		DisableCache:    dns.DisableCache,
		DisableFallback: dns.DisableFallback,
		Servers:         servers,
		Hosts:           dns.Hosts,
	}
}

func (c *XrayCompiler) compileOutbounds(outbounds []domain.Outbound) ([]XrayOutbound, error) {
	result := make([]XrayOutbound, 0, len(outbounds)+2)
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
		result = append(result, *compiledOb)
	}

	if !hasDirect {
		result = append(result, XrayOutbound{
			Tag:      "direct",
			Protocol: "freedom",
		})
	}
	if !hasBlock {
		result = append(result, XrayOutbound{
			Tag:      "block",
			Protocol: "blackhole",
		})
	}
	return result, nil
}

func compileRoutingConfig(routing *domain.RoutingConfig) *XrayRoutingConfig {
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
			outboundTag := strings.TrimSpace(r.OutboundTag)
			balancerTag := strings.TrimSpace(r.BalancerTag)
			if outboundTag == "" && balancerTag == "" {
				continue
			}
			ruleType := strings.TrimSpace(r.Type)
			if ruleType == "" {
				ruleType = "field"
			}
			rule := XrayRoutingRule{
				Type:          ruleType,
				Tag:           strings.TrimSpace(r.Tag),
				InboundTag:    cleanStringSlice(r.InboundTag),
				OutboundTag:   outboundTag,
				BalancerTag:   balancerTag,
				DomainMatcher: strings.TrimSpace(r.DomainMatcher),
				VlessRoute:    strings.TrimSpace(r.VlessRoute),
				Domain:        cleanStringSlice(r.Domain),
				IP:            cleanStringSlice(r.IP),
				Port:          strings.TrimSpace(r.Port),
				Network:       strings.TrimSpace(r.Network),
				Protocol:      cleanStringSlice(r.Protocol),
				User:          cleanStringSlice(r.User),
				Attrs:         strings.TrimSpace(r.Attrs),
				Source:        cleanStringSlice(r.Source),
				SourcePort:    strings.TrimSpace(r.SourcePort),
			}
			hasMatchCondition := len(rule.InboundTag) > 0 ||
				rule.VlessRoute != "" ||
				len(rule.Domain) > 0 ||
				len(rule.IP) > 0 ||
				rule.Port != "" ||
				rule.Network != "" ||
				len(rule.Protocol) > 0 ||
				len(rule.User) > 0 ||
				rule.Attrs != "" ||
				len(rule.Source) > 0 ||
				rule.SourcePort != ""
			if !hasMatchCondition {
				continue
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

	return &XrayRoutingConfig{
		DomainStrategy: domainStrategy,
		Rules:          layeredRules,
	}
}

// Compile 将系统业务数据编译为合法的 Xray 官方根配置对象
func (c *XrayCompiler) Compile(
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
) (*XrayConfigFile, error) {
	cfg := newBaseXrayConfig(c.GetLogLevel())

	// 1. 编译 DNS
	cfg.DNS = compileDNSConfig(dns)

	// 2. 编译本机 Inbounds (NodeID == 0) & 注入用户 Clients
	for _, inb := range inbounds {
		if !inb.Enabled || inb.Tag == "api" || inb.NodeID != 0 {
			continue
		}
		compiledInbound, err := c.compileInbound(&inb, users)
		if err != nil {
			return nil, fmt.Errorf("compile inbound %s failed: %w", inb.Tag, err)
		}
		cfg.Inbounds = append(cfg.Inbounds, *compiledInbound)
	}

	// 注入默认 API Inbound (用于 gRPC 本地通信，动态端口或默认 8080)
	apiPort := 8080
	if c != nil && c.grpcPort > 0 {
		apiPort = c.grpcPort
	}
	appendAPIInbound(cfg, apiPort)

	// 3. 编译 Outbounds
	compiledOutbounds, err := c.compileOutbounds(outbounds)
	if err != nil {
		return nil, err
	}
	cfg.Outbounds = compiledOutbounds

	// 4. 分层编译 Routing 规则 (Layer 1 核心通信 + Layer 2 统一规则池单一事实源)
	cfg.Routing = compileRoutingConfig(routing)

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

func sortRemoteInputs(inbounds []domain.Inbound, users []domain.User) ([]domain.Inbound, []domain.User) {
	sortedInbounds := make([]domain.Inbound, len(inbounds))
	copy(sortedInbounds, inbounds)
	sort.Slice(sortedInbounds, func(i, j int) bool {
		if sortedInbounds[i].SortOrder != sortedInbounds[j].SortOrder {
			return sortedInbounds[i].SortOrder < sortedInbounds[j].SortOrder
		}
		if sortedInbounds[i].ID != sortedInbounds[j].ID {
			return sortedInbounds[i].ID < sortedInbounds[j].ID
		}
		return sortedInbounds[i].Tag < sortedInbounds[j].Tag
	})

	sortedUsers := make([]domain.User, len(users))
	copy(sortedUsers, users)
	sort.Slice(sortedUsers, func(i, j int) bool {
		return sortedUsers[i].Email < sortedUsers[j].Email
	})
	return sortedInbounds, sortedUsers
}

// remoteAPIPort is the fixed loopback gRPC API port compiled into every remote node config.json
// (127.0.0.1:8080). It must stay aligned with cmd/agent's default --xray-grpc address.
const remoteAPIPort = 8080

// CompileRemoteNode 为远端直连节点编译独立的 Xray 配置（直连 freedom 出站 + 本地 127.0.0.1:8080 API 入站 + 节点启用入站）。
func (c *XrayCompiler) CompileRemoteNode(inbounds []domain.Inbound, users []domain.User) (*XrayConfigFile, error) {
	return c.CompileRemoteNodeFull(inbounds, nil, nil, nil, users, nil)
}

// CompileRemoteNodeToJSON 编译远端直连节点配置并序列化为格式化 JSON。
func (c *XrayCompiler) CompileRemoteNodeToJSON(inbounds []domain.Inbound, users []domain.User) ([]byte, error) {
	cfg, err := c.CompileRemoteNode(inbounds, users)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "    ")
}

// CompileRemoteNodeFull 编译远端直连节点的完整 Xray 配置（含自定义 Outbounds/Routing/DNS 与静态中转凭据）。
func (c *XrayCompiler) CompileRemoteNodeFull(
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
	staticClients []domain.StaticInboundClient,
) (*XrayConfigFile, error) {
	sortedInbounds, sortedUsers := sortRemoteInputs(inbounds, users)
	cfg := newBaseXrayConfig(c.GetLogLevel())
	cfg.DNS = compileDNSConfig(dns)

	staticByTag := make(map[string][]domain.StaticInboundClient)
	if len(staticClients) > 0 {
		sortedStatic := make([]domain.StaticInboundClient, len(staticClients))
		copy(sortedStatic, staticClients)
		sort.Slice(sortedStatic, func(i, j int) bool {
			if sortedStatic[i].InboundTag != sortedStatic[j].InboundTag {
				return sortedStatic[i].InboundTag < sortedStatic[j].InboundTag
			}
			if sortedStatic[i].Email != sortedStatic[j].Email {
				return sortedStatic[i].Email < sortedStatic[j].Email
			}
			return sortedStatic[i].ID < sortedStatic[j].ID
		})
		for _, sc := range sortedStatic {
			staticByTag[sc.InboundTag] = append(staticByTag[sc.InboundTag], sc)
		}
	}

	for _, inb := range sortedInbounds {
		if !inb.Enabled || inb.Tag == "api" {
			continue
		}
		compiledInbound, err := c.compileInboundWithStatic(&inb, sortedUsers, staticByTag[inb.Tag])
		if err != nil {
			return nil, fmt.Errorf("compile remote inbound %s failed: %w", inb.Tag, err)
		}
		cfg.Inbounds = append(cfg.Inbounds, *compiledInbound)
	}

	appendAPIInbound(cfg, remoteAPIPort)

	compiledOutbounds, err := c.compileOutbounds(outbounds)
	if err != nil {
		return nil, err
	}
	cfg.Outbounds = compiledOutbounds
	cfg.Routing = compileRoutingConfig(routing)

	return cfg, nil
}

// CompileRemoteNodeFullToJSON 编译远端直连节点的完整 Xray 配置并序列化为格式化 JSON。
func (c *XrayCompiler) CompileRemoteNodeFullToJSON(
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
	staticClients []domain.StaticInboundClient,
) ([]byte, error) {
	cfg, err := c.CompileRemoteNodeFull(inbounds, outbounds, routing, dns, users, staticClients)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(cfg, "", "    ")
}

// BuildRemoteNodeUsers 为远端节点的动态用户入站生成按 (inboundTag, email) 排序的活跃授权用户清单。
func (c *XrayCompiler) BuildRemoteNodeUsers(inbounds []domain.Inbound, users []domain.User) ([]domain.AgentSyncUser, error) {
	sortedInbounds, sortedUsers := sortRemoteInputs(inbounds, users)
	result := make([]domain.AgentSyncUser, 0)

	for _, inb := range sortedInbounds {
		if !inb.Enabled || inb.Tag == "api" || !domain.SupportsDynamicUser(inb.Protocol) {
			continue
		}
		proto := strings.ToLower(strings.TrimSpace(inb.Protocol))
		accessor := NewInboundStreamAccessorFromInbound(&inb)
		method := ""
		switch proto {
		case "shadowsocks", "shadowsocks-2022", "ss", "ss-2022", "ss2022":
			method = accessor.ResolveShadowsocksMethod()
		}
		for _, u := range filterActiveUsersForInbound(&inb, sortedUsers) {
			flow := ""
			if proto == "vless" {
				flow = resolveVLESSClientFlow(&inb, accessor, u.Flow)
			}
			result = append(result, domain.AgentSyncUser{
				InboundTag: inb.Tag,
				Protocol:   proto,
				Email:      u.Email,
				UUID:       u.UUID,
				Flow:       flow,
				Method:     method,
			})
		}
	}

	return result, nil
}

func (c *XrayCompiler) compileInbound(inb *domain.Inbound, users []domain.User) (*XrayInbound, error) {
	return c.compileInboundWithStatic(inb, users, nil)
}

func mergeStaticClients(inboundProto string, base []XrayClient, staticClients []domain.StaticInboundClient) []XrayClient {
	if len(staticClients) == 0 {
		return base
	}
	proto := strings.ToLower(strings.TrimSpace(inboundProto))
	if proto != "vless" && proto != "vmess" && proto != "trojan" {
		return base
	}

	seen := make(map[string]bool, len(base))
	for _, cl := range base {
		switch proto {
		case "trojan":
			if k := strings.TrimSpace(cl.Password); k != "" {
				seen[k] = true
			}
		default:
			if k := strings.ToLower(strings.TrimSpace(cl.ID)); k != "" {
				seen[k] = true
			}
		}
	}

	merged := make([]XrayClient, len(base), len(base)+len(staticClients))
	copy(merged, base)

	for _, sc := range staticClients {
		if sc.Protocol != "" && !strings.EqualFold(sc.Protocol, proto) {
			continue
		}
		cred := strings.TrimSpace(sc.ID)
		if cred == "" {
			continue
		}
		dedupKey := cred
		if proto != "trojan" {
			dedupKey = strings.ToLower(cred)
		}
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		switch proto {
		case "vless":
			flow := strings.TrimSpace(sc.Flow)
			if strings.EqualFold(flow, "none") {
				flow = ""
			}
			merged = append(merged, XrayClient{
				ID:    cred,
				Flow:  flow,
				Email: sc.Email,
				Level: 0,
			})
		case "vmess":
			merged = append(merged, XrayClient{
				ID:    cred,
				Email: sc.Email,
				Level: 0,
			})
		case "trojan":
			merged = append(merged, XrayClient{
				Password: cred,
				Email:    sc.Email,
				Level:    0,
			})
		}
	}
	return merged
}

func (c *XrayCompiler) compileInboundWithStatic(inb *domain.Inbound, users []domain.User, staticClients []domain.StaticInboundClient) (*XrayInbound, error) {
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
		if streamSettings.TLSSettings != nil {
			streamSettings.TLSSettings.Fingerprint = "" // 服务端入站严禁包含客户端 uTLS fingerprint
		}
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
					if strings.Contains(sn, "*") {
						return nil, fmt.Errorf("%w: reality serverNames 不支持通配符 (*): %s", domain.ErrInvalidInput, sn)
					}
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
		if len(staticClients) > 0 {
			clients = mergeStaticClients(inb.Protocol, clients, staticClients)
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

	var muxRaw json.RawMessage
	muxStr := strings.TrimSpace(ob.Mux)
	if muxStr != "" && muxStr != "null" && muxStr != "{}" {
		if !json.Valid([]byte(muxStr)) {
			return nil, fmt.Errorf("outbound %s has invalid mux json: %s", ob.Tag, muxStr)
		}

		var mc struct {
			Enabled bool `json:"enabled"`
		}
		_ = json.Unmarshal([]byte(muxStr), &mc)

		hasVision := false
		if strings.EqualFold(ob.Protocol, "vless") && strings.Contains(strings.ToLower(settingsStr), "xtls-rprx-vision") {
			hasVision = true
		}

		if (hasVision && mc.Enabled) || !mc.Enabled {
			// 1. VLESS xtls-rprx-vision 与 Mux 互斥：自动抑制 Mux，防止内核冲突
			// 2. 显式禁用 (enabled=false) 时抑制 Mux，保持输出配置极简且杜绝潜在冲突
			muxRaw = nil
		} else {
			muxRaw = json.RawMessage(muxStr)
		}
	}

	return &XrayOutbound{
		Tag:            ob.Tag,
		Protocol:       ob.Protocol,
		SendThrough:    strings.TrimSpace(ob.SendThrough),
		TargetStrategy: strings.TrimSpace(ob.TargetStrategy),
		Settings:       json.RawMessage(settingsStr),
		StreamSettings: streamSettings,
		Mux:            muxRaw,
	}, nil
}

// cleanStringSlice 去除空白字符串并过滤空元素
func cleanStringSlice(arr []string) []string {
	if len(arr) == 0 {
		return nil
	}
	var res []string
	for _, item := range arr {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res
}
