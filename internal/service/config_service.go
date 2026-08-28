package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/pkg/jsonc"
)

type ConfigService struct {
	configMgr   *xray.ConfigManager
	supervisor  *xray.SystemdSupervisor
	inboundRepo domain.InboundRepository
	userRepo    domain.UserRepository
}

func NewConfigService(
	configMgr *xray.ConfigManager,
	supervisor *xray.SystemdSupervisor,
	inboundRepo domain.InboundRepository,
	userRepo domain.UserRepository,
) *ConfigService {
	return &ConfigService{
		configMgr:   configMgr,
		supervisor:  supervisor,
		inboundRepo: inboundRepo,
		userRepo:    userRepo,
	}
}

func (s *ConfigService) GetRawConfig(ctx context.Context) ([]byte, error) {
	return s.configMgr.ReadRawConfig()
}

func (s *ConfigService) ValidateRawConfig(ctx context.Context, rawJSON []byte) error {
	return s.configMgr.ValidateConfig(ctx, rawJSON)
}

func (s *ConfigService) SaveAndApplyRawConfig(ctx context.Context, rawJSON []byte) error {
	if err := s.configMgr.WriteConfig(ctx, rawJSON); err != nil {
		return err
	}

	cleaned := jsonc.StripJSONC(rawJSON)

	// 1. 解析 inbounds 同步至数据库
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

				// 2. 收集每个 inbound 下的 clients（用户）
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

	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) ListInbounds(ctx context.Context) ([]domain.Inbound, error) {
	return s.inboundRepo.ListAll(ctx)
}

func (s *ConfigService) CreateInbound(ctx context.Context, inbound *domain.Inbound) error {
	if inbound.Tag == "" {
		return fmt.Errorf("%w: tag is required", domain.ErrInvalidInput)
	}

	// 1. 存入数据库
	if err := s.inboundRepo.Create(ctx, inbound); err != nil {
		return err
	}

	// 2. 双向同步回写 config.json
	_ = s.syncInboundToFile(ctx, inbound, false)
	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) UpdateInbound(ctx context.Context, inbound *domain.Inbound) error {
	if err := s.inboundRepo.Update(ctx, inbound); err != nil {
		return err
	}
	_ = s.syncInboundToFile(ctx, inbound, false)
	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) DeleteInbound(ctx context.Context, id uint) error {
	inbound, err := s.inboundRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.inboundRepo.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.syncInboundToFile(ctx, inbound, true)
	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) syncInboundToFile(ctx context.Context, inbound *domain.Inbound, isDelete bool) error {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	inbounds, _ := root["inbounds"].([]interface{})
	var newInbounds []interface{}
	found := false

	for _, ibRaw := range inbounds {
		ibMap, ok := ibRaw.(map[string]interface{})
		if !ok {
			newInbounds = append(newInbounds, ibRaw)
			continue
		}
		tag, _ := ibMap["tag"].(string)
		if tag == inbound.Tag {
			found = true
			if isDelete {
				continue // 移除该 Inbound
			}
			// 更新现有 Inbound
			ibMap["port"] = inbound.Port
			ibMap["listen"] = inbound.Listen
			ibMap["protocol"] = inbound.Protocol

			var streamObj map[string]interface{}
			_ = json.Unmarshal([]byte(inbound.StreamSettings), &streamObj)
			if streamObj != nil {
				ibMap["streamSettings"] = streamObj
			}

			var sniffObj map[string]interface{}
			_ = json.Unmarshal([]byte(inbound.SniffingJSON), &sniffObj)
			if sniffObj != nil {
				ibMap["sniffing"] = sniffObj
			}
			newInbounds = append(newInbounds, ibMap)
		} else {
			newInbounds = append(newInbounds, ibRaw)
		}
	}

	if !found && !isDelete {
		newMap := map[string]interface{}{
			"tag":      inbound.Tag,
			"port":     inbound.Port,
			"listen":   inbound.Listen,
			"protocol": inbound.Protocol,
			"settings": map[string]interface{}{
				"clients": []interface{}{},
			},
		}
		var streamObj map[string]interface{}
		_ = json.Unmarshal([]byte(inbound.StreamSettings), &streamObj)
		if streamObj != nil {
			newMap["streamSettings"] = streamObj
		}
		var sniffObj map[string]interface{}
		_ = json.Unmarshal([]byte(inbound.SniffingJSON), &sniffObj)
		if sniffObj != nil {
			newMap["sniffing"] = sniffObj
		}
		newInbounds = append(newInbounds, newMap)
	}

	root["inbounds"] = newInbounds
	modifiedBytes, err := json.Marshal(root)
	if err != nil {
		return err
	}

	return s.configMgr.WriteConfig(ctx, modifiedBytes)
}

func (s *ConfigService) SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	inbounds, ok := root["inbounds"].([]interface{})
	if !ok {
		return nil
	}

	tagSet := make(map[string]bool)
	for _, t := range authorizedTags {
		tagSet[t] = true
	}

	for _, ibRaw := range inbounds {
		ibMap, ok := ibRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := ibMap["tag"].(string)

		settings, ok := ibMap["settings"].(map[string]interface{})
		if !ok || settings == nil {
			settings = make(map[string]interface{})
			ibMap["settings"] = settings
		}

		clientsRaw, _ := settings["clients"].([]interface{})
		var newClients []interface{}
		found := false
		shouldHave := tagSet[tag] && !isDelete

		for _, cRaw := range clientsRaw {
			cMap, ok := cRaw.(map[string]interface{})
			if !ok {
				newClients = append(newClients, cRaw)
				continue
			}
			email, _ := cMap["email"].(string)
			if email == user.Email {
				found = true
				if shouldHave {
					cMap["id"] = user.UUID
					cMap["flow"] = user.Flow
					newClients = append(newClients, cMap)
				}
				// 否则从该 inbound 移除
			} else {
				newClients = append(newClients, cRaw)
			}
		}

		if !found && shouldHave {
			newClient := map[string]interface{}{
				"id":    user.UUID,
				"email": user.Email,
				"flow":  user.Flow,
				"level": 0,
			}
			newClients = append(newClients, newClient)
		}

		settings["clients"] = newClients
	}

	root["inbounds"] = inbounds
	modifiedBytes, err := json.Marshal(root)
	if err != nil {
		return err
	}

	return s.configMgr.WriteConfig(ctx, modifiedBytes)
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
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	outbounds, _ := root["outbounds"].([]interface{})
	var newOutbounds []interface{}
	found := false

	for _, obRaw := range outbounds {
		obMap, ok := obRaw.(map[string]interface{})
		if !ok {
			newOutbounds = append(newOutbounds, obRaw)
			continue
		}
		tag, _ := obMap["tag"].(string)
		if tag == outbound.Tag {
			found = true
			obMap["protocol"] = outbound.Protocol
			var sObj map[string]interface{}
			_ = json.Unmarshal([]byte(outbound.SettingsJSON), &sObj)
			if sObj != nil {
				obMap["settings"] = sObj
			}
			var strObj map[string]interface{}
			_ = json.Unmarshal([]byte(outbound.StreamSettings), &strObj)
			if strObj != nil {
				obMap["streamSettings"] = strObj
			}
			newOutbounds = append(newOutbounds, obMap)
		} else {
			newOutbounds = append(newOutbounds, obRaw)
		}
	}

	if !found {
		newMap := map[string]interface{}{
			"tag":      outbound.Tag,
			"protocol": outbound.Protocol,
		}
		var sObj map[string]interface{}
		_ = json.Unmarshal([]byte(outbound.SettingsJSON), &sObj)
		if sObj != nil {
			newMap["settings"] = sObj
		}
		var strObj map[string]interface{}
		_ = json.Unmarshal([]byte(outbound.StreamSettings), &strObj)
		if strObj != nil {
			newMap["streamSettings"] = strObj
		}
		newOutbounds = append(newOutbounds, newMap)
	}

	root["outbounds"] = newOutbounds
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}

	if err := s.configMgr.WriteConfig(ctx, data); err != nil {
		return err
	}

	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) DeleteOutbound(ctx context.Context, tag string) error {
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	outbounds, _ := root["outbounds"].([]interface{})
	var newOutbounds []interface{}

	for _, obRaw := range outbounds {
		obMap, ok := obRaw.(map[string]interface{})
		if !ok {
			newOutbounds = append(newOutbounds, obRaw)
			continue
		}
		t, _ := obMap["tag"].(string)
		if t != tag {
			newOutbounds = append(newOutbounds, obRaw)
		}
	}

	root["outbounds"] = newOutbounds
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}

	if err := s.configMgr.WriteConfig(ctx, data); err != nil {
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
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	root["routing"] = cfg
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}

	if err := s.configMgr.WriteConfig(ctx, data); err != nil {
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
	raw, err := s.configMgr.ReadRawConfig()
	if err != nil {
		return err
	}
	cleaned := jsonc.StripJSONC(raw)

	var root map[string]interface{}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		return err
	}

	root["dns"] = cfg
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}

	if err := s.configMgr.WriteConfig(ctx, data); err != nil {
		return err
	}

	return s.supervisor.Reload(ctx)
}

func (s *ConfigService) RestartService(ctx context.Context) error {
	return s.supervisor.Restart(ctx)
}
