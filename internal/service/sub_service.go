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

func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string) (*domain.SubscriptionPayload, error) {
	if token == "" {
		return nil, domain.ErrSubscriptionToken
	}

	user, err := s.userRepo.GetBySubToken(ctx, token)
	if err != nil {
		return nil, domain.ErrSubscriptionToken
	}

	if !user.Enabled {
		return nil, domain.ErrUserDisabled
	}
	if user.IsExpired() {
		return nil, fmt.Errorf("%w: user expired", domain.ErrSubscriptionToken)
	}
	if user.IsTrafficExceeded() {
		return nil, domain.ErrQuotaExceeded
	}

	// 获取用户绑定的 Inbound 或所有可用 Inbound
	inbounds, err := s.inboundRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	hostDomain, _ := s.settingRepo.Get(ctx, "sub_domain")

	var shareLinks []string
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		link := xray.BuildShareLink(&in, user, hostDomain)
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
		RemainingBytes: remainingBytes,
		ExpireTime:     user.ExpireTime,
	}, nil
}

func (s *SubService) GetUserShareLink(ctx context.Context, userID uint) (string, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	inbound, err := s.inboundRepo.GetByTag(ctx, user.InboundTag)
	if err != nil {
		return "", err
	}
	hostDomain, _ := s.settingRepo.Get(ctx, "sub_domain")
	return xray.BuildShareLink(inbound, user, hostDomain), nil
}
