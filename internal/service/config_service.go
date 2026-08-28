package service

import (
	"context"
	"encoding/json"
	"fmt"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
)

type ConfigService struct {
	configMgr   *xray.ConfigManager
	supervisor  *xray.SystemdSupervisor
	inboundRepo domain.InboundRepository
}

func NewConfigService(configMgr *xray.ConfigManager, supervisor *xray.SystemdSupervisor, inboundRepo domain.InboundRepository) *ConfigService {
	return &ConfigService{
		configMgr:   configMgr,
		supervisor:  supervisor,
		inboundRepo: inboundRepo,
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

	// 解析其中的 inbounds 同步到数据库
	var js struct {
		Inbounds []struct {
			Tag            string          `json:"tag"`
			Port           int             `json:"port"`
			Listen         string          `json:"listen"`
			Protocol       string          `json:"protocol"`
			Settings       json.RawMessage `json:"settings"`
			StreamSettings json.RawMessage `json:"streamSettings"`
			Sniffing       json.RawMessage `json:"sniffing"`
		} `json:"inbounds"`
	}

	if err := json.Unmarshal(rawJSON, &js); err == nil {
		for _, in := range js.Inbounds {
			if in.Tag == "" || in.Tag == "api" {
				continue
			}
			existing, _ := s.inboundRepo.GetByTag(ctx, in.Tag)
			if existing != nil {
				existing.Port = in.Port
				existing.Listen = in.Listen
				existing.Protocol = in.Protocol
				existing.SettingsJSON = string(in.Settings)
				existing.StreamSettings = string(in.StreamSettings)
				existing.SniffingJSON = string(in.Sniffing)
				_ = s.inboundRepo.Update(ctx, existing)
			} else {
				newIn := &domain.Inbound{
					Tag:            in.Tag,
					Port:           in.Port,
					Listen:         in.Listen,
					Protocol:       in.Protocol,
					SettingsJSON:   string(in.Settings),
					StreamSettings: string(in.StreamSettings),
					SniffingJSON:   string(in.Sniffing),
					Remark:         in.Tag,
					Enabled:        true,
				}
				_ = s.inboundRepo.Create(ctx, newIn)
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
	return s.inboundRepo.Create(ctx, inbound)
}

func (s *ConfigService) UpdateInbound(ctx context.Context, inbound *domain.Inbound) error {
	return s.inboundRepo.Update(ctx, inbound)
}

func (s *ConfigService) DeleteInbound(ctx context.Context, id uint) error {
	return s.inboundRepo.Delete(ctx, id)
}
