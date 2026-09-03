package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/pkg/jsonc"
)

type ServiceSupervisor interface {
	Reload(ctx context.Context) error
	Restart(ctx context.Context) error
}

type ConfigService struct {
	configMgr    *xray.ConfigManager
	supervisor   ServiceSupervisor
	inboundRepo  domain.InboundRepository
	userRepo     domain.UserRepository
	snapshotRepo domain.ConfigSnapshotRepository
	compiler     *xray.XrayCompiler
}

func NewConfigService(
	configMgr *xray.ConfigManager,
	supervisor ServiceSupervisor,
	inboundRepo domain.InboundRepository,
	userRepo domain.UserRepository,
	snapshotRepo domain.ConfigSnapshotRepository,
	compiler ...*xray.XrayCompiler,
) *ConfigService {
	c := xray.NewXrayCompiler()
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

func (s *ConfigService) recordSnapshotBeforeWrite(ctx context.Context, remark string) {
	if s.snapshotRepo == nil {
		return
	}
	oldRaw, err := s.configMgr.ReadRawConfig()
	if err == nil && len(oldRaw) > 0 {
		_ = s.snapshotRepo.Save(ctx, &domain.ConfigSnapshot{
			RawConfig: string(oldRaw),
			Remark:    remark,
			CreatedAt: time.Now(),
		})
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
	return s.configMgr.ReadRawConfig()
}

func (s *ConfigService) ValidateRawConfig(ctx context.Context, rawJSON []byte) error {
	return s.configMgr.ValidateConfig(ctx, rawJSON)
}

func (s *ConfigService) SaveAndApplyRawConfig(ctx context.Context, rawJSON []byte) error {
	s.recordSnapshotBeforeWrite(ctx, "保存并应用原生 JSON 配置")
	if err := s.configMgr.WriteConfig(ctx, rawJSON); err != nil {
		return err
	}

	_ = s.syncFromRawJSON(ctx, rawJSON)
	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) SyncFromFile(ctx context.Context) error {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil || len(raw) == 0 {
		return nil
	}
	return s.syncFromRawJSON(ctx, raw)
}

// SaveConfigQuietly 核心强类型编译管道：读取所有领域实体 ➔ 编译为合规 JSON ➔ 校验写入 ➔ 静默持久化 (不平滑重载 supervisor)
func (s *ConfigService) SaveConfigQuietly(ctx context.Context, remark string) error {
	// 1. 读取 Inbounds
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		var err error
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return err
		}
	}

	// 2. 读取 Outbounds
	outbounds, err := s.ListOutbounds(ctx)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read outbounds failed, abort saving to protect config: %w", err)
		}
		outbounds = nil
	}

	// 3. 读取 Routing
	routing, err := s.GetRoutingConfig(ctx)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read routing failed, abort saving to protect config: %w", err)
		}
		routing = nil
	}

	// 4. 读取 DNS
	dns, err := s.GetDNSConfig(ctx)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("read dns failed, abort saving to protect config: %w", err)
		}
		dns = nil
	}

	// 5. 读取 Users
	var users []domain.User
	if s.userRepo != nil {
		users, err = s.userRepo.ListAll(ctx)
		if err != nil {
			return err
		}
	}

	// 6. 单向编译为强类型 JSON
	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, routing, dns, users)
	if err != nil {
		return fmt.Errorf("compile config failed: %w", err)
	}

	// 7. 写入 (不调用 supervisor.Reload)
	s.recordSnapshotBeforeWrite(ctx, remark)
	if s.configMgr != nil {
		if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
			return err
		}
	}

	return nil
}

// RecompileAndApply 核心强类型编译管道：读取所有领域实体 ➔ 编译为合规 JSON ➔ 校验写入 ➔ 平滑重载
func (s *ConfigService) RecompileAndApply(ctx context.Context, remark string) error {
	if err := s.SaveConfigQuietly(ctx, remark); err != nil {
		return err
	}

	if s.supervisor != nil {
		return s.supervisor.Reload(ctx)
	}
	return nil
}

func (s *ConfigService) syncFromRawJSON(ctx context.Context, rawJSON []byte) error {
	cleaned := jsonc.StripJSONC(rawJSON)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err == nil {
		if inboundsRaw, ok := root["inbounds"].([]interface{}); ok {
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

				settingsBytes, _ := json.Marshal(ib["settings"])
				streamBytes, _ := json.Marshal(ib["streamSettings"])
				sniffingBytes, _ := json.Marshal(ib["sniffing"])

				existing, _ := s.inboundRepo.GetByTag(ctx, tag)
				if existing != nil {
					existing.Port = port
					existing.Listen = listen
					existing.Protocol = protocol
					existing.SettingsJSON = string(settingsBytes)
					existing.StreamSettings = string(streamBytes)
					existing.SniffingJSON = string(sniffingBytes)
					_ = s.inboundRepo.Update(ctx, existing)
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
					_ = s.inboundRepo.Create(ctx, newIn)
				}

				// 收集每个 inbound 下的 clients（用户）
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

							existingUser, _ := s.userRepo.GetByEmail(ctx, email)
							if existingUser != nil {
								existingUser.UUID = uuidStr
								existingUser.Flow = flow
								tags := existingUser.GetInboundTagList()
								if !existingUser.HasInbound(tag) {
									tags = append(tags, tag)
									existingUser.InboundTags = strings.Join(tags, ",")
								}
								_ = s.userRepo.Update(ctx, existingUser)
							} else {
								tokenBytes := make([]byte, 16)
								_, _ = rand.Read(tokenBytes)
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
								_ = s.userRepo.Create(ctx, newUser)
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func (s *ConfigService) ListInbounds(ctx context.Context) ([]domain.Inbound, error) {
	list, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].LatencyMs, list[i].IsAlive = probeInboundPort(list[i].Listen, list[i].Port)
	}
	return list, nil
}

func probeInboundPort(listen string, port int) (int64, bool) {
	if port <= 0 {
		return 0, false
	}
	host := listen
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 1000*time.Millisecond)
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

	if err := s.inboundRepo.Create(ctx, inbound); err != nil {
		return err
	}

	return s.RecompileAndApply(ctx, fmt.Sprintf("创建入站节点 %s", inbound.Tag))
}

func (s *ConfigService) UpdateInbound(ctx context.Context, inbound *domain.Inbound) error {
	if err := s.inboundRepo.Update(ctx, inbound); err != nil {
		return err
	}

	return s.RecompileAndApply(ctx, fmt.Sprintf("更新入站节点 %s", inbound.Tag))
}

func (s *ConfigService) DeleteInbound(ctx context.Context, id uint) error {
	inbound, err := s.inboundRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.inboundRepo.Delete(ctx, id); err != nil {
		return err
	}

	return s.RecompileAndApply(ctx, fmt.Sprintf("删除入站节点 %s", inbound.Tag))
}

func (s *ConfigService) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	return s.SaveConfigQuietly(ctx, fmt.Sprintf("静默同步用户 %s 节点授权", user.Email))
}

// === 出站管理 (Outbounds Management) ===

func (s *ConfigService) ListOutbounds(ctx context.Context) ([]domain.Outbound, error) {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return nil, err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root struct {
		Outbounds []struct {
			Tag            string          `json:"tag"`
			Protocol       string          `json:"protocol"`
			Settings       json.RawMessage `json:"settings"`
			StreamSettings json.RawMessage `json:"streamSettings"`
		} `json:"outbounds"`
	}

	if err := json.Unmarshal(cleaned, &root); err != nil {
		return nil, err
	}

	var list []domain.Outbound
	for _, ob := range root.Outbounds {
		list = append(list, domain.Outbound{
			Tag:            ob.Tag,
			Protocol:       ob.Protocol,
			SettingsJSON:   string(ob.Settings),
			StreamSettings: string(ob.StreamSettings),
		})
	}
	return list, nil
}

func (s *ConfigService) SaveOutbound(ctx context.Context, outbound domain.Outbound) error {
	existingList, err := s.ListOutbounds(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort saving to protect config: %w", err)
	}
	found := false
	var updatedList []domain.Outbound
	for _, ob := range existingList {
		if ob.Tag == outbound.Tag {
			found = true
			updatedList = append(updatedList, outbound)
		} else {
			updatedList = append(updatedList, ob)
		}
	}
	if !found {
		updatedList = append(updatedList, outbound)
	}

	// 临时保存到配置并重新编译
	return s.saveOutboundsListAndRecompile(ctx, updatedList, fmt.Sprintf("保存出站节点 %s", outbound.Tag))
}

func (s *ConfigService) DeleteOutbound(ctx context.Context, tag string) error {
	existingList, err := s.ListOutbounds(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort deleting to protect config: %w", err)
	}
	var updatedList []domain.Outbound
	for _, ob := range existingList {
		if ob.Tag != tag {
			updatedList = append(updatedList, ob)
		}
	}

	return s.saveOutboundsListAndRecompile(ctx, updatedList, fmt.Sprintf("删除出站节点 %s", tag))
}

func (s *ConfigService) saveOutboundsListAndRecompile(ctx context.Context, outbounds []domain.Outbound, remark string) error {
	// 读取当前 Inbounds, Routing, DNS, Users
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, _ = s.inboundRepo.ListAll(ctx)
	}
	routing, err := s.GetRoutingConfig(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read routing failed, abort saving to protect config: %w", err)
	}
	dns, err := s.GetDNSConfig(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read dns failed, abort saving to protect config: %w", err)
	}
	var users []domain.User
	if s.userRepo != nil {
		users, _ = s.userRepo.ListAll(ctx)
	}

	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, routing, dns, users)
	if err != nil {
		return err
	}

	s.recordSnapshotBeforeWrite(ctx, remark)
	if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
		return err
	}
	return s.supervisor.Reload(ctx)
}

// === 路由设置 (Routing Management) ===

func (s *ConfigService) GetRoutingConfig(ctx context.Context) (*domain.RoutingConfig, error) {
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

	if root.Routing.DomainStrategy == "" {
		root.Routing.DomainStrategy = "IPIfNonMatch"
	}

	return &root.Routing, nil
}

func (s *ConfigService) SaveRoutingConfig(ctx context.Context, cfg *domain.RoutingConfig) error {
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, _ = s.inboundRepo.ListAll(ctx)
	}
	outbounds, err := s.ListOutbounds(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort saving to protect config: %w", err)
	}
	dns, err := s.GetDNSConfig(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read dns failed, abort saving to protect config: %w", err)
	}
	var users []domain.User
	if s.userRepo != nil {
		users, _ = s.userRepo.ListAll(ctx)
	}

	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, cfg, dns, users)
	if err != nil {
		return err
	}

	s.recordSnapshotBeforeWrite(ctx, "更新分流路由规则")
	if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
		return err
	}
	return s.supervisor.Reload(ctx)
}

// === DNS 设置 (DNS Management) ===

func (s *ConfigService) GetDNSConfig(ctx context.Context) (*domain.DNSConfig, error) {
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

	if len(root.DNS.Servers) == 0 {
		root.DNS.Servers = []interface{}{"https://1.1.1.1/dns-query", "8.8.8.8", "localhost"}
	}
	if root.DNS.QueryStrategy == "" {
		root.DNS.QueryStrategy = "UseIP"
	}

	return &root.DNS, nil
}

func (s *ConfigService) SaveDNSConfig(ctx context.Context, cfg *domain.DNSConfig) error {
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, _ = s.inboundRepo.ListAll(ctx)
	}
	outbounds, err := s.ListOutbounds(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read outbounds failed, abort saving to protect config: %w", err)
	}
	routing, err := s.GetRoutingConfig(ctx)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("read routing failed, abort saving to protect config: %w", err)
	}
	var users []domain.User
	if s.userRepo != nil {
		users, _ = s.userRepo.ListAll(ctx)
	}

	jsonBytes, err := s.compiler.CompileToJSON(inbounds, outbounds, routing, cfg, users)
	if err != nil {
		return err
	}

	s.recordSnapshotBeforeWrite(ctx, "更新 DNS 解析设置")
	if err := s.configMgr.WriteConfig(ctx, jsonBytes); err != nil {
		return err
	}
	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) RestartService(ctx context.Context) error {
	return s.supervisor.Restart(ctx)
}
