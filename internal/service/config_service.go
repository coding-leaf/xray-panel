package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
	"panel/internal/pkg/jsonc"
	"panel/internal/protocol"
)

type ServiceSupervisor interface {
	Reload(ctx context.Context) error
	Restart(ctx context.Context) error
}

type ConfigStorage interface {
	ReadRawConfig() ([]byte, error)
	WriteConfig(ctx context.Context, rawJSON []byte) error
	ValidateConfig(ctx context.Context, rawJSON []byte) error
}

type ConfigCompiler interface {
	CompileToJSON(inbounds []domain.Inbound, outbounds []domain.Outbound, routing *domain.RoutingConfig, dns *domain.DNSConfig, users []domain.User) ([]byte, error)
}

type ConfigService struct {
	mu           sync.Mutex
	configMgr    ConfigStorage
	supervisor   ServiceSupervisor
	inboundRepo  domain.InboundRepository
	userRepo     domain.UserRepository
	snapshotRepo domain.ConfigSnapshotRepository
	compiler     ConfigCompiler
}

func NewConfigService(
	configMgr ConfigStorage,
	supervisor ServiceSupervisor,
	inboundRepo domain.InboundRepository,
	userRepo domain.UserRepository,
	snapshotRepo domain.ConfigSnapshotRepository,
	compiler ...ConfigCompiler,
) *ConfigService {
	var c ConfigCompiler
	if len(compiler) > 0 && compiler[0] != nil {
		c = compiler[0]
	}
	return &ConfigService{
		configMgr:    configMgr,
		supervisor:   supervisor,
		inboundRepo:  inboundRepo,
		userRepo:     userRepo,
		snapshotRepo: snapshotRepo,
		compiler:     c,
	}
}

func (s *ConfigService) recordSnapshotWithRaw(ctx context.Context, remark string, oldRaw []byte) {
	if s.snapshotRepo == nil || len(oldRaw) == 0 {
		return
	}
	if saveErr := s.snapshotRepo.Save(ctx, &domain.ConfigSnapshot{
		RawConfig: string(oldRaw),
		Remark:    remark,
		CreatedAt: time.Now(),
	}); saveErr != nil {
		slog.Warn("Failed to save config snapshot before write", slog.String("error", saveErr.Error()))
	}
}

func (s *ConfigService) recordSnapshotBeforeWrite(ctx context.Context, remark string) {
	if s.snapshotRepo == nil || s.configMgr == nil {
		return
	}
	oldRaw, err := s.configMgr.ReadRawConfig()
	if err == nil && len(oldRaw) > 0 {
		s.recordSnapshotWithRaw(ctx, remark, oldRaw)
	}
}

func (s *ConfigService) ListSnapshots(ctx context.Context, limit int) ([]domain.ConfigSnapshot, error) {
	if s.snapshotRepo == nil {
		return []domain.ConfigSnapshot{}, nil
	}
	return s.snapshotRepo.List(ctx, limit)
}

func (s *ConfigService) RollbackSnapshot(ctx context.Context, id uint) error {
	if s.snapshotRepo == nil {
		return fmt.Errorf("snapshot repository not configured")
	}
	snapshot, err := s.snapshotRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("snapshot not found: %w", err)
	}

	return s.SaveAndApplyRawConfig(ctx, []byte(snapshot.RawConfig))
}

func (s *ConfigService) GetRawConfig(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configMgr.ReadRawConfig()
}

func (s *ConfigService) ValidateRawConfig(ctx context.Context, rawJSON []byte) error {
	return s.configMgr.ValidateConfig(ctx, rawJSON)
}

func (s *ConfigService) SaveAndApplyRawConfig(ctx context.Context, rawJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveAndApplyRawConfigLocked(ctx, rawJSON)
}

func (s *ConfigService) saveAndApplyRawConfigLocked(ctx context.Context, rawJSON []byte) error {
	s.recordSnapshotBeforeWrite(ctx, "保存并应用原生 JSON 配置")
	if err := s.configMgr.WriteConfig(ctx, rawJSON); err != nil {
		return err
	}

	if err := s.syncFromRawJSONLocked(ctx, rawJSON); err != nil {
		return fmt.Errorf("sync from raw config failed: %w", err)
	}
	if s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}

func (s *ConfigService) SyncFromFile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := s.configMgr.ReadRawConfig()
	if err != nil || len(raw) == 0 {
		return nil
	}
	return s.syncFromRawJSONLocked(ctx, raw)
}

func (s *ConfigService) compileAndWriteLocked(
	ctx context.Context,
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
	remark string,
	reload bool,
) error {
	return s.compileAndWriteWithRawLocked(ctx, inbounds, outbounds, routing, dns, users, remark, reload, nil, false)
}

func (s *ConfigService) compileAndWriteWithRawLocked(
	ctx context.Context,
	inbounds []domain.Inbound,
	outbounds []domain.Outbound,
	routing *domain.RoutingConfig,
	dns *domain.DNSConfig,
	users []domain.User,
	remark string,
	reload bool,
	oldRaw []byte,
	hasOldRaw bool,
) error {
	if s.compiler == nil {
		return fmt.Errorf("compiler not configured")
	}
	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, routing, dns, users)
	if err != nil {
		return fmt.Errorf("compile config failed: %w", err)
	}

	if hasOldRaw {
		s.recordSnapshotWithRaw(ctx, remark, oldRaw)
	} else {
		s.recordSnapshotBeforeWrite(ctx, remark)
	}
	if s.configMgr != nil {
		if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
			return err
		}
	}
	if reload && s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}

// SaveConfigQuietly 核心强类型编译管道：读取所有领域实体 ➔ 编译为合规 JSON ➔ 校验写入 ➔ 静默持久化 (不平滑重载 supervisor)
func (s *ConfigService) SaveConfigQuietly(ctx context.Context, remark string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveConfigQuietlyLocked(ctx, remark)
}

func (s *ConfigService) saveConfigQuietlyLocked(ctx context.Context, remark string) error {
	// 1. 读取 Inbounds（仅查询 1 次，直接复用于路由兜底对账）
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read inbounds failed, abort saving to protect config: %w", err)
		}
	}

	// 2-4. 单次读盘 + 单次 JSONC 清洗反序列化，同时提取 Outbounds / Routing / DNS 与原始快照字节
	var (
		outbounds []domain.Outbound
		routing   *domain.RoutingConfig
		dns       *domain.DNSConfig
		oldRaw    []byte
	)
	sections, err := s.readParsedSectionsLocked(ctx, inbounds, true)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read config sections failed, abort saving to protect config: %w", err)
		}
		routing = &domain.RoutingConfig{DomainStrategy: "IPIfNonMatch"}
		s.ensureSubRouteRules(routing, inbounds)
	} else {
		outbounds = sections.outbounds
		routing = sections.routing
		dns = sections.dns
		oldRaw = sections.rawBytes
	}

	// 5. 读取 Users
	var users []domain.User
	if s.userRepo != nil {
		users, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read users failed, abort saving to protect config: %w", err)
		}
	}

	// 6-7. 单向编译并写入 (复用 oldRaw 写入快照，不调用 supervisor.Reload)
	return s.compileAndWriteWithRawLocked(ctx, inbounds, outbounds, routing, dns, users, remark, false, oldRaw, true)
}

// RecompileAndApply 核心强类型编译管道：读取所有领域实体 ➔ 编译为合规 JSON ➔ 校验写入 ➔ 平滑重载
func (s *ConfigService) RecompileAndApply(ctx context.Context, remark string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recompileAndApplyLocked(ctx, remark)
}

func (s *ConfigService) recompileAndApplyLocked(ctx context.Context, remark string) error {
	if err := s.saveConfigQuietlyLocked(ctx, remark); err != nil {
		return err
	}

	if s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}

func (s *ConfigService) syncFromRawJSONLocked(ctx context.Context, rawJSON []byte) error {
	cleaned := jsonc.StripJSONC(rawJSON)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return fmt.Errorf("parse raw config json failed: %w", err)
	}

	inboundsRaw, ok := root["inbounds"].([]interface{})
	if !ok {
		return nil
	}

	for _, inRaw := range inboundsRaw {
		ib, ok := inRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := ib["tag"].(string)
		if tag == "" || tag == "api" {
			continue
		}

		port := 0
		if pFloat, ok := ib["port"].(float64); ok {
			port = int(pFloat)
		}
		listen, _ := ib["listen"].(string)
		protocol, _ := ib["protocol"].(string)

		var existing *domain.Inbound
		if s.inboundRepo != nil {
			var err error
			existing, err = s.inboundRepo.GetByTag(ctx, tag)
			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return fmt.Errorf("lookup inbound %q failed: %w", tag, err)
			}
		}

		if strings.EqualFold(protocol, "vless") {
			if settingsMap, ok := ib["settings"].(map[string]interface{}); ok {
				rawFlow, _ := settingsMap["flow"].(string)
				if strings.TrimSpace(rawFlow) == "" {
					if existing != nil {
						var prevSettings map[string]interface{}
						if err := json.Unmarshal([]byte(existing.SettingsJSON), &prevSettings); err == nil {
							if pf, ok := prevSettings["flow"].(string); ok && strings.TrimSpace(pf) != "" {
								settingsMap["flow"] = strings.TrimSpace(pf)
							}
						}
					} else {
						hasClientWithFlow := false
						hasAnyClient := false
						if clientsRaw, ok := settingsMap["clients"].([]interface{}); ok && len(clientsRaw) > 0 {
							hasAnyClient = true
							for _, cRaw := range clientsRaw {
								if cMap, ok := cRaw.(map[string]interface{}); ok {
									if cf, _ := cMap["flow"].(string); strings.TrimSpace(cf) != "" {
										hasClientWithFlow = true
										break
									}
								}
							}
						}
						if hasAnyClient && !hasClientWithFlow {
							settingsMap["flow"] = "none"
						}
					}
				}
			}
		}

		if existing != nil {
			mergePanelOnlyStreamFields(ib["streamSettings"], existing.StreamSettings)
		}

		settingsBytes, err := json.Marshal(ib["settings"])
		if err != nil {
			return fmt.Errorf("marshal settings for inbound %q failed: %w", tag, err)
		}
		streamBytes, err := json.Marshal(ib["streamSettings"])
		if err != nil {
			return fmt.Errorf("marshal streamSettings for inbound %q failed: %w", tag, err)
		}
		sniffingBytes, err := json.Marshal(ib["sniffing"])
		if err != nil {
			return fmt.Errorf("marshal sniffing for inbound %q failed: %w", tag, err)
		}

		if s.inboundRepo != nil {
			if existing != nil {
				existing.Port = port
				existing.Listen = listen
				existing.Protocol = protocol
				existing.SettingsJSON = string(settingsBytes)
				existing.StreamSettings = string(streamBytes)
				existing.SniffingJSON = string(sniffingBytes)
				if err := s.inboundRepo.Update(ctx, existing); err != nil {
					return fmt.Errorf("update inbound %q failed: %w", tag, err)
				}
			} else {
				newIn := &domain.Inbound{
					Tag:            tag,
					Port:           port,
					Listen:         listen,
					Protocol:       protocol,
					SettingsJSON:   string(settingsBytes),
					StreamSettings: string(streamBytes),
					SniffingJSON:   string(sniffingBytes),
					Remark:         tag,
					Enabled:        true,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}
				if err := s.inboundRepo.Create(ctx, newIn); err != nil {
					return fmt.Errorf("create inbound %q failed: %w", tag, err)
				}
			}
		}

		// 收集每个 inbound 下的 clients（用户）
		if s.userRepo != nil {
			if settingsMap, ok := ib["settings"].(map[string]interface{}); ok {
				if clientsRaw, ok := settingsMap["clients"].([]interface{}); ok {
					for _, cRaw := range clientsRaw {
						cMap, ok := cRaw.(map[string]interface{})
						if !ok {
							continue
						}
						email, _ := cMap["email"].(string)
						uuidStr, _ := cMap["id"].(string)
						if uuidStr == "" {
							uuidStr, _ = cMap["password"].(string)
						}
						flow, _ := cMap["flow"].(string)

						if email == "" || uuidStr == "" {
							continue
						}

						existingUser, err := s.userRepo.GetByEmail(ctx, email)
						if err != nil && !errors.Is(err, domain.ErrNotFound) {
							return fmt.Errorf("lookup user %q failed: %w", email, err)
						}
						if existingUser != nil {
							existingUser.UUID = uuidStr
							tags := existingUser.GetInboundTagList()
							if !existingUser.HasInbound(tag) {
								tags = append(tags, tag)
								existingUser.InboundTags = strings.Join(tags, ",")
							}
							if err := s.userRepo.Update(ctx, existingUser); err != nil {
								return fmt.Errorf("update user %q failed: %w", email, err)
							}
						} else {
							tokenBytes := make([]byte, 16)
							if _, err := rand.Read(tokenBytes); err != nil {
								return fmt.Errorf("generate sub token for user %q failed: %w", email, err)
							}
							subToken := hex.EncodeToString(tokenBytes)

							newUser := &domain.User{
								UUID:        uuidStr,
								Email:       email,
								InboundTag:  tag,
								InboundTags: tag,
								Flow:        flow,
								SubToken:    subToken,
								Enabled:     true,
								CreatedAt:   time.Now(),
								UpdatedAt:   time.Now(),
							}
							if err := s.userRepo.Create(ctx, newUser); err != nil {
								return fmt.Errorf("create user %q failed: %w", email, err)
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// mergePanelOnlyStreamFields preserves panel-only client hints in StreamSettings
// (tlsSettings.fingerprint and realitySettings.{publicKey,fingerprint,spiderX})
// that XrayCompiler strips when generating server-side config.json.
func mergePanelOnlyStreamFields(incomingRaw interface{}, prevStreamJSON string) {
	incStream, ok := incomingRaw.(map[string]interface{})
	if !ok || incStream == nil || strings.TrimSpace(prevStreamJSON) == "" {
		return
	}
	var prevStream map[string]interface{}
	if err := json.Unmarshal([]byte(prevStreamJSON), &prevStream); err != nil || prevStream == nil {
		return
	}

	if incTLS, ok := incStream["tlsSettings"].(map[string]interface{}); ok {
		if prevTLS, ok := prevStream["tlsSettings"].(map[string]interface{}); ok {
			if curFP, _ := incTLS["fingerprint"].(string); strings.TrimSpace(curFP) == "" {
				if prevFP, _ := prevTLS["fingerprint"].(string); strings.TrimSpace(prevFP) != "" {
					incTLS["fingerprint"] = strings.TrimSpace(prevFP)
				}
			}
		}
	}

	if incReal, ok := incStream["realitySettings"].(map[string]interface{}); ok {
		if prevReal, ok := prevStream["realitySettings"].(map[string]interface{}); ok {
			incPriv, _ := incReal["privateKey"].(string)
			prevPriv, _ := prevReal["privateKey"].(string)
			incPriv = strings.TrimSpace(incPriv)
			prevPriv = strings.TrimSpace(prevPriv)
			if prevPriv == "" || incPriv == "" || incPriv == prevPriv {
				for _, key := range []string{"publicKey", "fingerprint", "spiderX"} {
					if curVal, _ := incReal[key].(string); strings.TrimSpace(curVal) == "" {
						if prevVal, _ := prevReal[key].(string); strings.TrimSpace(prevVal) != "" {
							incReal[key] = prevVal
						}
					}
				}
			}
		}
	}
}

type statusProvider interface {
	GetStatus(ctx context.Context) (domain.ServiceStatus, error)
}

func (s *ConfigService) ListInbounds(ctx context.Context) ([]domain.Inbound, error) {
	list, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	needCoreStatus := false
	for _, ib := range list {
		if ib.Enabled && ib.Port > 0 && isPureUDPInbound(ib) {
			needCoreStatus = true
			break
		}
	}

	coreRunning := false
	if needCoreStatus {
		coreRunning = s.isCoreRunning(ctx)
	}

	var wg sync.WaitGroup
	for i := range list {
		ib := &list[i]
		if !ib.Enabled || ib.Port <= 0 {
			ib.LatencyMs = 0
			ib.IsAlive = false
			continue
		}
		if isPureUDPInbound(*ib) {
			ib.LatencyMs = 0
			ib.IsAlive = coreRunning
			continue
		}
		wg.Add(1)
		go func(idx int, listen string, port int) {
			defer wg.Done()
			list[idx].LatencyMs, list[idx].IsAlive = probeInboundPort(listen, port)
		}(i, ib.Listen, ib.Port)
	}
	wg.Wait()

	return list, nil
}

func (s *ConfigService) isCoreRunning(ctx context.Context) bool {
	if s.supervisor == nil {
		return true
	}
	sp, ok := s.supervisor.(statusProvider)
	if !ok {
		return true
	}
	st, err := sp.GetStatus(ctx)
	if err != nil {
		slog.Warn("Failed to query core status for UDP inbound check", slog.String("error", err.Error()))
		return false
	}
	return st.Active
}

func isPureUDPInbound(ib domain.Inbound) bool {
	proto := strings.ToLower(strings.TrimSpace(ib.Protocol))
	if proto == "hysteria2" || proto == "hy2" {
		return true
	}
	if proto != "dokodemo-door" {
		return false
	}
	var settings struct {
		Network string `json:"network"`
	}
	if ib.SettingsJSON != "" {
		if err := json.Unmarshal([]byte(ib.SettingsJSON), &settings); err == nil {
			if netVal := strings.TrimSpace(settings.Network); netVal != "" {
				return isPureUDPNetwork(netVal)
			}
		}
	}
	var stream struct {
		Network string `json:"network"`
	}
	if ib.StreamSettings != "" {
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err == nil {
			if netVal := strings.TrimSpace(stream.Network); netVal != "" {
				return isPureUDPNetwork(netVal)
			}
		}
	}
	return false
}

func isPureUDPNetwork(network string) bool {
	hasUDP := false
	for _, part := range strings.Split(strings.ToLower(network), ",") {
		switch strings.TrimSpace(part) {
		case "tcp":
			return false
		case "udp":
			hasUDP = true
		}
	}
	return hasUDP
}

func probeInboundPort(listen string, port int) (int64, bool) {
	if port <= 0 {
		return 0, false
	}
	host := strings.TrimSpace(listen)
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::", "[::]":
		host = "::1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return 0, false
	}
	_ = conn.Close()
	elapsed := time.Since(start).Milliseconds()
	if elapsed == 0 {
		elapsed = 1
	}
	return elapsed, true
}

func (s *ConfigService) CreateInbound(ctx context.Context, inbound *domain.Inbound) error {
	if inbound.Tag == "" {
		return fmt.Errorf("%w: tag is required", domain.ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.inboundRepo.Create(ctx, inbound); err != nil {
		return err
	}

	return s.recompileAndApplyLocked(ctx, fmt.Sprintf("创建入站节点 %s", inbound.Tag))
}

func (s *ConfigService) UpdateInbound(ctx context.Context, inbound *domain.Inbound) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.inboundRepo.Update(ctx, inbound); err != nil {
		return err
	}

	return s.recompileAndApplyLocked(ctx, fmt.Sprintf("更新入站节点 %s", inbound.Tag))
}

func (s *ConfigService) DeleteInbound(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inbound, err := s.inboundRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.inboundRepo.Delete(ctx, id); err != nil {
		return err
	}

	return s.recompileAndApplyLocked(ctx, fmt.Sprintf("删除入站节点 %s", inbound.Tag))
}

func (s *ConfigService) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	return s.SaveConfigQuietly(ctx, fmt.Sprintf("静默同步用户 %s 节点授权", user.Email))
}

// === 出站管理 (Outbounds Management) ===

// cleanMux 规范化 Mux 字段:
// 1. 若为 ""、"{}"、"null" 或 {"enabled":false}，视为显式清空 Mux，返回 ""；
// 2. 否则返回去除首尾空白的 JSON 字符串。
func cleanMux(muxVal string) string {
	trimmed := strings.TrimSpace(muxVal)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return ""
	}
	var mc struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(trimmed), &mc); err == nil && !mc.Enabled {
		return ""
	}
	return trimmed
}

type rawOutboundItem struct {
	Tag            string          `json:"tag"`
	Protocol       string          `json:"protocol"`
	SendThrough    string          `json:"sendThrough"`
	TargetStrategy string          `json:"targetStrategy"`
	Settings       json.RawMessage `json:"settings"`
	StreamSettings json.RawMessage `json:"streamSettings"`
	Mux            json.RawMessage `json:"mux"`
}

type parsedRawSections struct {
	rawBytes  []byte
	outbounds []domain.Outbound
	routing   *domain.RoutingConfig
	dns       *domain.DNSConfig
}

func parseOutboundsList(items []rawOutboundItem) []domain.Outbound {
	var list []domain.Outbound
	for _, ob := range items {
		var muxStr string
		if len(ob.Mux) > 0 {
			muxStr = cleanMux(string(ob.Mux))
		}
		var streamSettingsStr string
		if len(ob.StreamSettings) > 0 && string(ob.StreamSettings) != "null" {
			streamSettingsStr = string(ob.StreamSettings)
		}
		var settingsJSONStr string
		if len(ob.Settings) > 0 && string(ob.Settings) != "null" {
			settingsJSONStr = string(ob.Settings)
		}

		list = append(list, domain.Outbound{
			Tag:            ob.Tag,
			Protocol:       ob.Protocol,
			SendThrough:    ob.SendThrough,
			TargetStrategy: ob.TargetStrategy,
			SettingsJSON:   settingsJSONStr,
			StreamSettings: streamSettingsStr,
			Mux:            muxStr,
		})
	}
	return list
}

func (s *ConfigService) normalizeRoutingConfig(
	ctx context.Context,
	cfg domain.RoutingConfig,
	inbounds []domain.Inbound,
	hasInbounds bool,
) (*domain.RoutingConfig, error) {
	if cfg.DomainStrategy == "" {
		cfg.DomainStrategy = "IPIfNonMatch"
	}

	// 自动过滤内部 api 规则，并对历史遗留堆叠规则进行幂等去重
	seen := make(map[string]bool)
	var cleanRules []domain.RoutingRule
	for _, r := range cfg.Rules {
		if (len(r.InboundTag) > 0 && r.InboundTag[0] == "api") || r.OutboundTag == "api" {
			continue
		}
		keyBytes, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("marshal routing rule failed: %w", err)
		}
		key := string(keyBytes)
		if seen[key] {
			continue
		}
		seen[key] = true
		cleanRules = append(cleanRules, r)
	}
	cfg.Rules = cleanRules

	// 确保所有启用的 SubRoutes 在 routing.Rules 中均有显式规则（若无则默认追加至尾部）
	if !hasInbounds && s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("read inbounds for routing failed: %w", err)
		}
	}
	if s.inboundRepo != nil || hasInbounds {
		s.ensureSubRouteRules(&cfg, inbounds)
	}

	return &cfg, nil
}

func normalizeDNSConfig(cfg domain.DNSConfig) *domain.DNSConfig {
	if len(cfg.Servers) == 0 {
		cfg.Servers = []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}
	}
	if cfg.QueryStrategy == "" {
		cfg.QueryStrategy = "UseIP"
	}
	return &cfg
}

func (s *ConfigService) readParsedSectionsLocked(ctx context.Context, inbounds []domain.Inbound, hasInbounds bool) (*parsedRawSections, error) {
	if s.configMgr == nil {
		return nil, domain.ErrNotFound
	}
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return nil, err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root struct {
		Outbounds []rawOutboundItem    `json:"outbounds"`
		Routing   domain.RoutingConfig `json:"routing"`
		DNS       domain.DNSConfig     `json:"dns"`
	}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return nil, err
	}

	routing, err := s.normalizeRoutingConfig(ctx, root.Routing, inbounds, hasInbounds)
	if err != nil {
		return nil, err
	}

	return &parsedRawSections{
		rawBytes:  raw,
		outbounds: parseOutboundsList(root.Outbounds),
		routing:   routing,
		dns:       normalizeDNSConfig(root.DNS),
	}, nil
}

func (s *ConfigService) ListOutbounds(ctx context.Context) ([]domain.Outbound, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listOutboundsLocked(ctx)
}

func (s *ConfigService) listOutboundsLocked(ctx context.Context) ([]domain.Outbound, error) {
	if s.configMgr == nil {
		return nil, domain.ErrNotFound
	}
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return nil, err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root struct {
		Outbounds []rawOutboundItem `json:"outbounds"`
	}

	if err := json.Unmarshal(cleaned, &root); err != nil {
		return nil, err
	}

	return parseOutboundsList(root.Outbounds), nil
}

func (s *ConfigService) SaveOutbound(ctx context.Context, outbound domain.Outbound) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existingList, err := s.listOutboundsLocked(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort saving to protect config: %w", err)
	}

	found := false
	var updatedList []domain.Outbound
	for _, ob := range existingList {
		if ob.Tag == outbound.Tag {
			found = true
			targetOb := outbound
			if strings.TrimSpace(targetOb.Mux) == "" {
				// 旧版/未传 mux 字段时保留已有配置
				targetOb.Mux = ob.Mux
			} else {
				targetOb.Mux = cleanMux(targetOb.Mux)
			}
			updatedList = append(updatedList, targetOb)
		} else {
			updatedList = append(updatedList, ob)
		}
	}
	if !found {
		outbound.Mux = cleanMux(outbound.Mux)
		updatedList = append(updatedList, outbound)
	}

	// 临时保存到配置并重新编译
	return s.saveOutboundsListAndRecompileLocked(ctx, updatedList, fmt.Sprintf("保存出站节点 %s", outbound.Tag))
}

func (s *ConfigService) DeleteOutbound(ctx context.Context, tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existingList, err := s.listOutboundsLocked(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort deleting to protect config: %w", err)
	}
	var updatedList []domain.Outbound
	for _, ob := range existingList {
		if ob.Tag != tag {
			updatedList = append(updatedList, ob)
		}
	}

	return s.saveOutboundsListAndRecompileLocked(ctx, updatedList, fmt.Sprintf("删除出站节点 %s", tag))
}

func (s *ConfigService) saveOutboundsListAndRecompileLocked(ctx context.Context, outbounds []domain.Outbound, remark string) error {
	// 读取当前 Inbounds, Routing, DNS, Users
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read inbounds failed, abort saving to protect config: %w", err)
		}
	}
	var (
		routing *domain.RoutingConfig
		dns     *domain.DNSConfig
		oldRaw  []byte
	)
	sections, err := s.readParsedSectionsLocked(ctx, inbounds, true)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read config sections failed, abort saving to protect config: %w", err)
		}
	} else {
		routing = sections.routing
		dns = sections.dns
		oldRaw = sections.rawBytes
	}
	var users []domain.User
	if s.userRepo != nil {
		users, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read users failed, abort saving to protect config: %w", err)
		}
	}

	return s.compileAndWriteWithRawLocked(ctx, inbounds, outbounds, routing, dns, users, remark, true, oldRaw, true)
}

// === 路由设置 (Routing Management) ===

func (s *ConfigService) GetRoutingConfig(ctx context.Context) (*domain.RoutingConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getRoutingConfigLocked(ctx)
}

func (s *ConfigService) getRoutingConfigLocked(ctx context.Context) (*domain.RoutingConfig, error) {
	if s.configMgr == nil {
		return nil, domain.ErrNotFound
	}
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return nil, err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root struct {
		Routing domain.RoutingConfig `json:"routing"`
	}

	if err := json.Unmarshal(cleaned, &root); err != nil {
		return nil, err
	}

	return s.normalizeRoutingConfig(ctx, root.Routing, nil, false)
}

func (s *ConfigService) SaveRoutingConfig(ctx context.Context, cfg *domain.RoutingConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveRoutingConfigLocked(ctx, cfg)
}

func (s *ConfigService) saveRoutingConfigLocked(ctx context.Context, cfg *domain.RoutingConfig) error {
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read inbounds failed, abort saving to protect config: %w", err)
		}
	}
	// 确保入站分流线路有默认规则兜底（若完全无匹配则放尾部）
	s.ensureSubRouteRules(cfg, inbounds)

	var (
		outbounds []domain.Outbound
		dns       *domain.DNSConfig
		oldRaw    []byte
	)
	sections, err := s.readParsedSectionsLocked(ctx, inbounds, true)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read config sections failed, abort saving to protect config: %w", err)
		}
	} else {
		outbounds = sections.outbounds
		dns = sections.dns
		oldRaw = sections.rawBytes
	}
	var users []domain.User
	if s.userRepo != nil {
		users, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read users failed, abort saving to protect config: %w", err)
		}
	}

	return s.compileAndWriteWithRawLocked(ctx, inbounds, outbounds, cfg, dns, users, "更新分流路由规则", true, oldRaw, true)
}

func isPureSubRouteFallbackRule(r domain.RoutingRule) bool {
	return len(r.Domain) == 0 &&
		len(r.IP) == 0 &&
		r.Port == "" &&
		r.Network == "" &&
		len(r.Protocol) == 0 &&
		r.Attrs == "" &&
		r.Tag == "" &&
		len(r.Source) == 0 &&
		r.SourcePort == "" &&
		r.BalancerTag == "" &&
		r.DomainMatcher == ""
}

func (s *ConfigService) ensureSubRouteRules(routing *domain.RoutingConfig, inbounds []domain.Inbound) {
	if routing == nil {
		return
	}

	// 1. 建立受管入站及其启用 SubRoute 的权威索引: inboundTag -> (routeIDStr -> SubRoute)
	managedInbounds := make(map[string]map[string]domain.SubRoute, len(inbounds))
	for _, inb := range inbounds {
		activeMap := make(map[string]domain.SubRoute)
		if inb.Enabled {
			for _, sr := range inb.GetSubRoutes() {
				if !sr.Enabled || sr.RouteID <= 0 || sr.OutboundTag == "" {
					continue
				}
				routeIDStr := fmt.Sprintf("%d", sr.RouteID)
				activeMap[routeIDStr] = sr
			}
		}
		managedInbounds[inb.Tag] = activeMap
	}

	// 2. 清理已删除/改号的僵尸 SubRoute 纯兜底规则，并就地同步已修改的 OutboundTag 与 AllowedUsers
	var reconciled []domain.RoutingRule
	for _, r := range routing.Rules {
		if r.VlessRoute != "" && len(r.InboundTag) == 1 && isPureSubRouteFallbackRule(r) {
			inbTag := r.InboundTag[0]
			if activeMap, isManaged := managedInbounds[inbTag]; isManaged {
				sr, stillExists := activeMap[r.VlessRoute]
				if !stillExists {
					// 该 SubRoute ID 已在入站中被删除、停用或改号，移除残留僵尸规则
					continue
				}
				// 就地同步最新的目标出站与授权用户白名单
				r.OutboundTag = sr.OutboundTag
				if len(sr.AllowedUsers) > 0 {
					r.User = sr.AllowedUsers
				} else {
					r.User = nil
				}
			}
		}
		reconciled = append(reconciled, r)
	}
	routing.Rules = reconciled

	// 3. 补齐尚未存在于 routing.Rules 中的新增 SubRoute 兜底规则
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		for _, sr := range inb.GetSubRoutes() {
			if !sr.Enabled || sr.RouteID <= 0 || sr.OutboundTag == "" {
				continue
			}
			routeIDStr := fmt.Sprintf("%d", sr.RouteID)

			matched := false
			for _, r := range routing.Rules {
				if r.VlessRoute == routeIDStr && isPureSubRouteFallbackRule(r) {
					if len(r.InboundTag) == 0 || sliceContains(r.InboundTag, inb.Tag) {
						matched = true
						break
					}
				}
			}

			if !matched {
				rule := domain.RoutingRule{
					Type:        "field",
					InboundTag:  []string{inb.Tag},
					VlessRoute:  routeIDStr,
					OutboundTag: sr.OutboundTag,
				}
				if len(sr.AllowedUsers) > 0 {
					rule.User = sr.AllowedUsers
				}
				routing.Rules = append(routing.Rules, rule)
			}
		}
	}
}

func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// === DNS 设置 (DNS Management) ===

func (s *ConfigService) GetDNSConfig(ctx context.Context) (*domain.DNSConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getDNSConfigLocked(ctx)
}

func (s *ConfigService) getDNSConfigLocked(ctx context.Context) (*domain.DNSConfig, error) {
	if s.configMgr == nil {
		return nil, domain.ErrNotFound
	}
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return nil, err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root struct {
		DNS domain.DNSConfig `json:"dns"`
	}

	if err := json.Unmarshal(cleaned, &root); err != nil {
		return nil, err
	}

	return normalizeDNSConfig(root.DNS), nil
}

func (s *ConfigService) SaveDNSConfig(ctx context.Context, cfg *domain.DNSConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveDNSConfigLocked(ctx, cfg)
}

func (s *ConfigService) saveDNSConfigLocked(ctx context.Context, cfg *domain.DNSConfig) error {
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read inbounds failed, abort saving to protect config: %w", err)
		}
	}
	var (
		outbounds []domain.Outbound
		routing   *domain.RoutingConfig
		oldRaw    []byte
	)
	sections, err := s.readParsedSectionsLocked(ctx, inbounds, true)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read config sections failed, abort saving to protect config: %w", err)
		}
	} else {
		outbounds = sections.outbounds
		routing = sections.routing
		oldRaw = sections.rawBytes
	}
	var users []domain.User
	if s.userRepo != nil {
		users, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return fmt.Errorf("read users failed, abort saving to protect config: %w", err)
		}
	}

	return s.compileAndWriteWithRawLocked(ctx, inbounds, outbounds, routing, cfg, users, "更新 DNS 解析设置", true, oldRaw, true)
}

func (s *ConfigService) RestartService(ctx context.Context) error {
	return s.supervisor.Restart(ctx)
}

func (s *ConfigService) GenerateRealityKeyPair() (*protocol.RealityKeyPair, error) {
	return protocol.GenerateRealityKeyPair()
}
