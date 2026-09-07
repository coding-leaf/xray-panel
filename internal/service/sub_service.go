package service

import (
	"context"
	"fmt"
	"strings"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/protocol"
	"panel/internal/sub"
)

type SubService struct {
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
	settingRepo domain.SettingRepository
}

func NewSubService(userRepo domain.UserRepository, inboundRepo domain.InboundRepository, settingRepo domain.SettingRepository) *SubService {
	return &SubService{
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		settingRepo: settingRepo,
	}
}

func (s *SubService) resolveSubscriptionNodes(ctx context.Context, token string, tagFilter string, reqHost string) (*domain.User, []*protocol.NodeConfig, error) {
	if token == "" {
		return nil, nil, domain.ErrSubscriptionToken
	}

	user, err := s.userRepo.GetBySubToken(ctx, token)
	if err != nil || user == nil {
		return nil, nil, domain.ErrSubscriptionToken
	}

	if !user.Enabled {
		return nil, nil, domain.ErrUserDisabled
	}
	if !user.IsActive() {
		return nil, nil, fmt.Errorf("%w: 用户已过期或流量已超额", domain.ErrQuotaExceeded)
	}

	// 获取所有节点
	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	var hostDomain string
	if s.settingRepo != nil {
		hostDomain, _ = s.settingRepo.Get(ctx, "sub_domain")
		if hostDomain == "" {
			hostDomain, _ = s.settingRepo.Get(ctx, "public_url")
		}
	}
	if hostDomain == "" && reqHost != "" {
		hostDomain = reqHost
	}

	defaultPort := 443
	if s.settingRepo != nil {
		defaultPortStr, _ := s.settingRepo.Get(ctx, "public_port")
		if defaultPortStr != "" {
			_, _ = fmt.Sscanf(defaultPortStr, "%d", &defaultPort)
		}
	}

	nodes := xray.InboundsToNodeConfigs(inbounds, user, hostDomain, defaultPort, tagFilter)
	return user, nodes, nil
}

func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string, tagFilter string, reqHost string) (*domain.SubscriptionPayload, error) {
	user, nodes, err := s.resolveSubscriptionNodes(ctx, token, tagFilter, reqHost)
	if err != nil {
		return nil, err
	}

	base64Output, err := sub.ExportSubscription(nodes, sub.FormatBase64)
	if err != nil {
		return nil, err
	}
	rawText, _ := sub.ExportSubscription(nodes, sub.FormatRaw)

	var remainingBytes int64 = -1
	if user.TotalBytes > 0 {
		used := user.UpBytes + user.DownBytes
		remainingBytes = user.TotalBytes - used
		if remainingBytes < 0 {
			remainingBytes = 0
		}
	}

	return &domain.SubscriptionPayload{
		NodesRaw:       rawText,
		Base64Data:     base64Output,
		UserEmail:      user.Email,
		UpBytes:        user.UpBytes,
		DownBytes:      user.DownBytes,
		TotalBytes:     user.TotalBytes,
		RemainingBytes: remainingBytes,
		ExpireTime:     user.ExpireTime,
	}, nil
}

// ExportUserSubscription 支持按指定格式导出用户订阅 (base64, clash, sing-box 等)
func (s *SubService) ExportUserSubscription(ctx context.Context, token string, tagFilter string, reqHost string, format string) (*domain.SubscriptionPayload, string, error) {
	user, nodes, err := s.resolveSubscriptionNodes(ctx, token, tagFilter, reqHost)
	if err != nil {
		return nil, "", err
	}

	base64Output, err := sub.ExportSubscription(nodes, sub.FormatBase64)
	if err != nil {
		return nil, "", err
	}
	rawText, _ := sub.ExportSubscription(nodes, sub.FormatRaw)

	var remainingBytes int64 = -1
	if user.TotalBytes > 0 {
		used := user.UpBytes + user.DownBytes
		remainingBytes = user.TotalBytes - used
		if remainingBytes < 0 {
			remainingBytes = 0
		}
	}

	payload := &domain.SubscriptionPayload{
		NodesRaw:       rawText,
		Base64Data:     base64Output,
		UserEmail:      user.Email,
		UpBytes:        user.UpBytes,
		DownBytes:      user.DownBytes,
		TotalBytes:     user.TotalBytes,
		RemainingBytes: remainingBytes,
		ExpireTime:     user.ExpireTime,
	}

	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" || fmtLower == sub.FormatBase64 || fmtLower == "b64" {
		return payload, payload.Base64Data, nil
	}
	if fmtLower == sub.FormatRaw || fmtLower == "plain" || fmtLower == "links" {
		return payload, payload.NodesRaw, nil
	}

	exported, err := sub.ExportSubscription(nodes, format)
	if err != nil {
		return nil, "", err
	}

	return payload, exported, nil
}

func (s *SubService) GetUserShareInfo(ctx context.Context, userID uint, baseURL string) (*domain.UserShareResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	var inbounds []domain.Inbound
	if s.inboundRepo != nil {
		inbounds, err = s.inboundRepo.ListAll(ctx)
		if err != nil {
			return nil, err
		}
	}

	var hostDomain string
	if s.settingRepo != nil {
		hostDomain, _ = s.settingRepo.Get(ctx, "sub_domain")
	}
	if hostDomain == "" && baseURL != "" {
		hostDomain = baseURL
	}

	defaultPort := 443
	if s.settingRepo != nil {
		defaultPortStr, _ := s.settingRepo.Get(ctx, "public_port")
		if defaultPortStr != "" {
			_, _ = fmt.Sscanf(defaultPortStr, "%d", &defaultPort)
		}
	}

	resp := &domain.UserShareResponse{
		UserID:    user.ID,
		Email:     user.Email,
		SubToken:  user.SubToken,
		AllSubURL: fmt.Sprintf("%s/sub/%s", baseURL, user.SubToken),
		Nodes:     make([]domain.NodeShareInfo, 0),
	}

	for _, in := range inbounds {
		if !in.Enabled || !user.HasInbound(in.Tag) {
			continue
		}
		link := xray.BuildShareLink(&in, user, hostDomain, defaultPort)
		resp.Nodes = append(resp.Nodes, domain.NodeShareInfo{
			Tag:       in.Tag,
			Protocol:  in.Protocol,
			Remark:    in.Remark,
			ShareLink: link,
			SingleSub: fmt.Sprintf("%s/sub/%s?tag=%s", baseURL, user.SubToken, in.Tag),
		})
	}

	return resp, nil
}
