package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

func buildVirtualInfoNodes(user *domain.User, announcement string) []*protocol.NodeConfig {
	var nodes []*protocol.NodeConfig

	// 1. 公告节点 (可选，配置时前置展示在最顶部)
	ann := strings.TrimSpace(announcement)
	if ann != "" {
		nodes = append(nodes, createVirtualTrojanNode(fmt.Sprintf("📢 公告: %s", ann)))
	}

	// 2. 流量状态节点
	var trafficName string
	used := user.UpBytes + user.DownBytes
	if user.TotalBytes > 0 {
		remaining := user.TotalBytes - used
		if remaining < 0 {
			remaining = 0
		}
		trafficName = fmt.Sprintf("📊 剩余流量: %.2f GB / %.2f GB", float64(remaining)/1073741824, float64(user.TotalBytes)/1073741824)
	} else {
		trafficName = fmt.Sprintf("📊 已用流量: %.2f GB (不限配额)", float64(used)/1073741824)
	}
	nodes = append(nodes, createVirtualTrojanNode(trafficName))

	// 3. 到期状态节点
	var expireName string
	if user.ExpireTime > 0 {
		expireName = fmt.Sprintf("⏳ 到期时间: %s", time.UnixMilli(user.ExpireTime).Format("2006-01-02"))
	} else {
		expireName = "⏳ 到期时间: 长期有效"
	}
	nodes = append(nodes, createVirtualTrojanNode(expireName))

	return nodes
}

func createVirtualTrojanNode(name string) *protocol.NodeConfig {
	return &protocol.NodeConfig{
		Name:     name,
		Protocol: "trojan",
		Address:  "127.0.0.1",
		Port:     10000,
		UUID:     "00000000-0000-0000-0000-000000000000",
		Params: map[string]string{
			"type":     "tcp",
			"security": "none",
		},
	}
}

func (s *SubService) resolveSubscriptionNodes(ctx context.Context, token string, tagFilter string, reqHost string, routeFilter ...uint16) (*domain.User, []*protocol.NodeConfig, error) {
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

	var defaultFP string
	if s.settingRepo != nil {
		if fp, err := s.settingRepo.Get(ctx, "sub_default_fingerprint"); err == nil && strings.TrimSpace(fp) != "" {
			defaultFP = strings.TrimSpace(fp)
			protocol.SetDefaultFingerprint(defaultFP)
		} else {
			protocol.SetDefaultFingerprint("chrome")
		}
	}

	nodes := protocol.InboundsToNodeConfigs(inbounds, user, hostDomain, defaultPort, tagFilter, routeFilter...)

	// 应用默认 TLS 指纹回退
	if defaultFP != "" {
		for _, n := range nodes {
			sec := n.GetParam("security")
			if sec == "reality" || sec == "tls" {
				if n.GetParam("fp") == "" || n.GetParam("fp") == "chrome" {
					n.SetParam("fp", defaultFP)
				}
			}
		}
	}

	// 应用节点命名模板 (仅作用于真实节点)
	if s.settingRepo != nil {
		if tmpl, err := s.settingRepo.Get(ctx, "sub_node_name_template"); err == nil && tmpl != "" {
			protocol.ApplyNodeNameTemplate(nodes, tmpl)
		}
	}

	// 全量聚合订阅顶部虚拟信息节点注入 (流量、到期时间、公告)
	isFullSub := tagFilter == "" && len(routeFilter) == 0
	showInfoNodes := false
	announcement := ""
	if s.settingRepo != nil {
		showInfoNodes = true
		if val, err := s.settingRepo.Get(ctx, "sub_show_info_nodes"); err == nil && (strings.EqualFold(val, "false") || val == "0") {
			showInfoNodes = false
		}
		ann, _ := s.settingRepo.Get(ctx, "sub_custom_announcement")
		announcement = ann
	}

	if isFullSub && showInfoNodes {
		infoNodes := buildVirtualInfoNodes(user, announcement)
		nodes = append(infoNodes, nodes...)
	}

	return user, nodes, nil
}

func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string, tagFilter string, reqHost string, routeFilter ...uint16) (*domain.SubscriptionPayload, error) {
	user, nodes, err := s.resolveSubscriptionNodes(ctx, token, tagFilter, reqHost, routeFilter...)
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
func (s *SubService) ExportUserSubscription(ctx context.Context, token string, tagFilter string, reqHost string, format string, routeFilter ...uint16) (*domain.SubscriptionPayload, string, error) {
	user, nodes, err := s.resolveSubscriptionNodes(ctx, token, tagFilter, reqHost, routeFilter...)
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

	profileTitle := "Xray Panel"
	updateInterval := 24
	clashMixedPort := 7890
	clashAllowLan := false
	clashMode := "rule"

	if s.settingRepo != nil {
		if t, err := s.settingRepo.Get(ctx, "sub_profile_title"); err == nil && t != "" {
			profileTitle = t
		}
		if intStr, err := s.settingRepo.Get(ctx, "sub_update_interval"); err == nil && intStr != "" {
			if val, err := strconv.Atoi(intStr); err == nil && val > 0 {
				updateInterval = val
			}
		}
		if pStr, err := s.settingRepo.Get(ctx, "sub_clash_mixed_port"); err == nil && pStr != "" {
			if val, err := strconv.Atoi(pStr); err == nil && val > 0 {
				clashMixedPort = val
			}
		}
		if lanStr, err := s.settingRepo.Get(ctx, "sub_clash_allow_lan"); err == nil && lanStr != "" {
			clashAllowLan = strings.EqualFold(lanStr, "true") || lanStr == "1"
		}
		if m, err := s.settingRepo.Get(ctx, "sub_clash_mode"); err == nil && m != "" {
			clashMode = m
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
		ProfileTitle:   profileTitle,
		UpdateInterval: updateInterval,
	}

	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" || fmtLower == sub.FormatBase64 || fmtLower == "b64" {
		return payload, payload.Base64Data, nil
	}
	if fmtLower == sub.FormatRaw || fmtLower == "plain" || fmtLower == "links" {
		return payload, payload.NodesRaw, nil
	}

	clashParams := sub.ClashParams{
		MixedPort: clashMixedPort,
		AllowLan:  clashAllowLan,
		Mode:      clashMode,
	}

	exported, err := sub.ExportSubscription(nodes, format, clashParams)
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

	defaultFP := "chrome"
	if s.settingRepo != nil {
		if fp, err := s.settingRepo.Get(ctx, "sub_default_fingerprint"); err == nil && strings.TrimSpace(fp) != "" {
			defaultFP = strings.TrimSpace(fp)
		}
	}
	protocol.SetDefaultFingerprint(defaultFP)

	resp := &domain.UserShareResponse{
		UserID:    user.ID,
		Email:     user.Email,
		SubToken:  user.SubToken,
		AllSubURL: fmt.Sprintf("%s/sub/%s", baseURL, user.SubToken),
		Nodes:     make([]domain.NodeShareInfo, 0),
	}

	sort.SliceStable(inbounds, func(i, j int) bool {
		if inbounds[i].SortOrder != inbounds[j].SortOrder {
			return inbounds[i].SortOrder < inbounds[j].SortOrder
		}
		return inbounds[i].ID < inbounds[j].ID
	})

	for _, in := range inbounds {
		if !in.Enabled || !user.HasInbound(in.Tag) || strings.EqualFold(in.Protocol, "dokodemo-door") {
			continue
		}
		isVless := in.Protocol == "" || strings.EqualFold(in.Protocol, "vless")
		subRoutes := in.GetSubRoutes()
		if len(subRoutes) == 0 || !isVless {
			link := protocol.BuildShareLink(&in, user, hostDomain, defaultPort)
			remark := in.Remark
			if remark == "" {
				remark = in.Tag
			}
			resp.Nodes = append(resp.Nodes, domain.NodeShareInfo{
				Tag:       in.Tag,
				Protocol:  in.Protocol,
				Remark:    remark,
				ShareLink: link,
				SingleSub: fmt.Sprintf("%s/sub/%s?tag=%s", baseURL, user.SubToken, in.Tag),
			})
			continue
		}

		sort.SliceStable(subRoutes, func(i, j int) bool {
			return subRoutes[i].SortOrder < subRoutes[j].SortOrder
		})

		baseName := in.Remark
		if baseName == "" {
			baseName = in.Tag
		}

		for _, sr := range subRoutes {
			if !sr.Enabled {
				continue
			}
			if !sr.CanAccess(user.Email) {
				continue
			}
			tempIn := in
			if sr.Name != "" {
				tempIn.Remark = sr.Name
			} else {
				tempIn.Remark = fmt.Sprintf("%s - 线路 #%d", baseName, sr.RouteID)
			}
			tempIn.RouteID = sr.RouteID
			link := protocol.BuildShareLink(&tempIn, user, hostDomain, defaultPort)
			resp.Nodes = append(resp.Nodes, domain.NodeShareInfo{
				Tag:       in.Tag,
				Protocol:  in.Protocol,
				Remark:    tempIn.Remark,
				ShareLink: link,
				SingleSub: fmt.Sprintf("%s/sub/%s?tag=%s&route=%d", baseURL, user.SubToken, in.Tag, sr.RouteID),
			})
		}
	}

	return resp, nil
}
