package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"panel/internal/adapter/xray"
	"panel/internal/domain"
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

func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string, tagFilter string) (*domain.SubscriptionPayload, error) {
	if token == "" {
		return nil, domain.ErrSubscriptionToken
	}

	user, err := s.userRepo.GetBySubToken(ctx, token)
	if err != nil {
		user, err = s.userRepo.GetByUUID(ctx, token)
	}
	if err != nil {
		user, err = s.userRepo.GetByEmail(ctx, token)
	}
	if err != nil || user == nil {
		return nil, domain.ErrSubscriptionToken
	}

	if !user.Enabled {
		return nil, domain.ErrUserDisabled
	}
	if !user.IsActive() {
		return nil, fmt.Errorf("%w: 用户已过期或流量已超额", domain.ErrQuotaExceeded)
	}

	// 获取所有节点
	inbounds, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	hostDomain, _ := s.settingRepo.Get(ctx, "sub_domain")
	defaultPortStr, _ := s.settingRepo.Get(ctx, "public_port")
	defaultPort := 443
	if defaultPortStr != "" {
		_, _ = fmt.Sscanf(defaultPortStr, "%d", &defaultPort)
	}

	var shareLinks []string
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		// 校验用户是否被授权属于该节点
		if !user.HasInbound(in.Tag) {
			continue
		}
		// 如果指定了单节点过滤参数，仅输出该节点
		if tagFilter != "" && in.Tag != tagFilter {
			continue
		}

		link := xray.BuildShareLink(&in, user, hostDomain, defaultPort)
		if link != "" {
			shareLinks = append(shareLinks, link)
		}
	}

	rawText := strings.Join(shareLinks, "\n")
	base64Output := base64.StdEncoding.EncodeToString([]byte(rawText))

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

func (s *SubService) GetUserShareInfo(ctx context.Context, userID uint, baseURL string) (*domain.UserShareResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	inbounds, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	hostDomain, _ := s.settingRepo.Get(ctx, "sub_domain")
	if hostDomain == "" && baseURL != "" {
		hostDomain = baseURL
	}

	defaultPortStr, _ := s.settingRepo.Get(ctx, "public_port")
	defaultPort := 443
	if defaultPortStr != "" {
		_, _ = fmt.Sscanf(defaultPortStr, "%d", &defaultPort)
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
