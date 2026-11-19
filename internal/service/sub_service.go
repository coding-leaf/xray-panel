package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	nodeRepo    domain.NodeRepository
}

func NewSubService(userRepo domain.UserRepository, inboundRepo domain.InboundRepository, settingRepo domain.SettingRepository) *SubService {
	return &SubService{
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		settingRepo: settingRepo,
	}
}

// WithNodeRepo 注入可选远端节点仓储，用于多机直连订阅聚合（保持原构造签名兼容）。
func (s *SubService) WithNodeRepo(repo domain.NodeRepository) *SubService {
	s.nodeRepo = repo
	return s
}

func (s *SubService) resolveActiveNodeMap(ctx context.Context) (map[uint]domain.Node, error) {
	if s.nodeRepo == nil {
		return nil, nil
	}
	nodes, err := s.nodeRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	active := make(map[uint]domain.Node, len(nodes))
	for _, n := range nodes {
		if n.Enabled && n.DesiredState == domain.NodeStateActive {
			active[n.ID] = n
		}
	}
	return active, nil
}

func formatNodePrefix(node domain.Node) string {
	label := strings.TrimSpace(node.Region)
	if label == "" {
		label = strings.TrimSpace(node.Name)
	}
	if label == "" {
		return ""
	}
	return fmt.Sprintf("[%s]", label)
}

func applyNodePrefix(name, prefix string) string {
	trimmed := strings.TrimSpace(name)
	if prefix == "" {
		return trimmed
	}
	if trimmed == "" {
		return prefix
	}
	if strings.Contains(trimmed, prefix) {
		return trimmed
	}
	return prefix + " " + trimmed
}

func prepareInboundForNode(ib domain.Inbound, activeNodes map[uint]domain.Node) (domain.Inbound, domain.Node, bool) {
	if ib.IsLocal() {
		return ib, domain.Node{}, true
	}
	node, ok := activeNodes[ib.NodeID]
	if !ok {
		return domain.Inbound{}, domain.Node{}, false
	}
	ibCopy := ib
	if strings.TrimSpace(ibCopy.ExternalHost) == "" && strings.TrimSpace(node.Host) != "" {
		ibCopy.ExternalHost = strings.TrimSpace(node.Host)
	}
	if !ibCopy.IsLocal() && ibCopy.ExternalPort <= 0 {
		ibCopy.ExternalPort = ibCopy.Port
	}
	prefix := formatNodePrefix(node)
	baseName := strings.TrimSpace(ibCopy.Remark)
	if baseName == "" {
		baseName = strings.TrimSpace(ibCopy.Tag)
	}
	ibCopy.Remark = applyNodePrefix(baseName, prefix)

	if prefix != "" {
		if subRoutes := ibCopy.GetSubRoutes(); len(subRoutes) > 0 {
			changed := false
			for i := range subRoutes {
				if strings.TrimSpace(subRoutes[i].Name) != "" {
					subRoutes[i].Name = applyNodePrefix(subRoutes[i].Name, prefix)
					changed = true
				}
			}
			if changed {
				raw, err := json.Marshal(subRoutes)
				if err != nil {
					slog.Warn("Failed to marshal prefixed subRoutes", slog.Uint64("inbound_id", uint64(ibCopy.ID)), slog.String("error", err.Error()))
				} else {
					ibCopy.SubRoutesJson = string(raw)
				}
			}
		}
	}
	return ibCopy, node, true
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

func (s *SubService) loadSettingsMap(ctx context.Context) map[string]string {
	if s.settingRepo == nil {
		return map[string]string{}
	}
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "failed to load settings map, using defaults", slog.Any("error", err))
		return map[string]string{}
	}
	if settings == nil {
		return map[string]string{}
	}
	return settings
}

func parsePortAndFingerprint(settings map[string]string) (int, string) {
	defaultPort := 443
	if defaultPortStr := settings["public_port"]; defaultPortStr != "" {
		_, _ = fmt.Sscanf(defaultPortStr, "%d", &defaultPort)
	}
	defaultFP := "chrome"
	if fp := strings.TrimSpace(settings["sub_default_fingerprint"]); fp != "" {
		defaultFP = fp
	}
	return defaultPort, defaultFP
}

func computeRemainingBytes(user *domain.User) int64 {
	if user.TotalBytes <= 0 {
		return -1
	}
	used := user.UpBytes + user.DownBytes
	remaining := user.TotalBytes - used
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *SubService) resolveSubscriptionNodesWithSettings(ctx context.Context, token string, tagFilter string, reqHost string, settings map[string]string, routeFilter ...uint16) (*domain.User, []*protocol.NodeConfig, error) {
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

	activeNodes, err := s.resolveActiveNodeMap(ctx)
	if err != nil {
		return nil, nil, err
	}

	filteredInbounds := make([]domain.Inbound, 0, len(inbounds))
	for _, rawIn := range inbounds {
		if prepared, _, ok := prepareInboundForNode(rawIn, activeNodes); ok {
			filteredInbounds = append(filteredInbounds, prepared)
		}
	}

	hostDomain := settings["sub_domain"]
	if hostDomain == "" {
		hostDomain = settings["public_url"]
	}
	if hostDomain == "" && reqHost != "" {
		hostDomain = reqHost
	}

	defaultPort, defaultFP := parsePortAndFingerprint(settings)

	nodes := protocol.InboundsToNodeConfigsWithDefaultFP(filteredInbounds, user, hostDomain, defaultPort, tagFilter, defaultFP, routeFilter...)

	// 应用节点命名模板 (仅作用于真实节点)
	if tmpl := settings["sub_node_name_template"]; tmpl != "" {
		protocol.ApplyNodeNameTemplate(nodes, tmpl)
	}

	// 全量聚合订阅顶部虚拟信息节点注入 (流量、到期时间、公告)
	isFullSub := tagFilter == "" && len(routeFilter) == 0
	showInfoNodes := false
	announcement := ""
	if s.settingRepo != nil {
		showInfoNodes = true
		if val, ok := settings["sub_show_info_nodes"]; ok && (strings.EqualFold(val, "false") || val == "0") {
			showInfoNodes = false
		}
		announcement = settings["sub_custom_announcement"]
	}

	if isFullSub && showInfoNodes {
		infoNodes := buildVirtualInfoNodes(user, announcement)
		nodes = append(infoNodes, nodes...)
	}

	return user, nodes, nil
}

func (s *SubService) GetSubscriptionByToken(ctx context.Context, token string, tagFilter string, reqHost string, routeFilter ...uint16) (*domain.SubscriptionPayload, error) {
	settings := s.loadSettingsMap(ctx)
	user, nodes, err := s.resolveSubscriptionNodesWithSettings(ctx, token, tagFilter, reqHost, settings, routeFilter...)
	if err != nil {
		return nil, err
	}

	base64Output, err := sub.ExportSubscription(nodes, sub.FormatBase64)
	if err != nil {
		return nil, err
	}
	rawText, _ := sub.ExportSubscription(nodes, sub.FormatRaw)

	return &domain.SubscriptionPayload{
		NodesRaw:       rawText,
		Base64Data:     base64Output,
		UserEmail:      user.Email,
		UpBytes:        user.UpBytes,
		DownBytes:      user.DownBytes,
		TotalBytes:     user.TotalBytes,
		RemainingBytes: computeRemainingBytes(user),
		ExpireTime:     user.ExpireTime,
	}, nil
}

// ExportUserSubscription 支持按指定格式导出用户订阅 (base64, clash, sing-box 等)，按需单次编码
func (s *SubService) ExportUserSubscription(ctx context.Context, token string, tagFilter string, reqHost string, format string, routeFilter ...uint16) (*domain.SubscriptionPayload, string, error) {
	settings := s.loadSettingsMap(ctx)
	user, nodes, err := s.resolveSubscriptionNodesWithSettings(ctx, token, tagFilter, reqHost, settings, routeFilter...)
	if err != nil {
		return nil, "", err
	}

	profileTitle := "Xray Panel"
	updateInterval := 24
	clashMixedPort := 7890
	clashAllowLan := false
	clashMode := "rule"

	if t := settings["sub_profile_title"]; t != "" {
		profileTitle = t
	}
	if intStr := settings["sub_update_interval"]; intStr != "" {
		if val, err := strconv.Atoi(intStr); err == nil && val > 0 {
			updateInterval = val
		}
	}
	if pStr := settings["sub_clash_mixed_port"]; pStr != "" {
		if val, err := strconv.Atoi(pStr); err == nil && val > 0 {
			clashMixedPort = val
		}
	}
	if lanStr := settings["sub_clash_allow_lan"]; lanStr != "" {
		clashAllowLan = strings.EqualFold(lanStr, "true") || lanStr == "1"
	}
	if m := settings["sub_clash_mode"]; m != "" {
		clashMode = m
	}

	payload := &domain.SubscriptionPayload{
		UserEmail:      user.Email,
		UpBytes:        user.UpBytes,
		DownBytes:      user.DownBytes,
		TotalBytes:     user.TotalBytes,
		RemainingBytes: computeRemainingBytes(user),
		ExpireTime:     user.ExpireTime,
		ProfileTitle:   profileTitle,
		UpdateInterval: updateInterval,
	}

	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" || fmtLower == sub.FormatBase64 || fmtLower == "b64" {
		exported, err := sub.ExportSubscription(nodes, sub.FormatBase64)
		if err != nil {
			return nil, "", err
		}
		payload.Base64Data = exported
		return payload, exported, nil
	}
	if fmtLower == sub.FormatRaw || fmtLower == "plain" || fmtLower == "links" {
		exported, err := sub.ExportSubscription(nodes, sub.FormatRaw)
		if err != nil {
			return nil, "", err
		}
		payload.NodesRaw = exported
		return payload, exported, nil
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

	activeNodes, err := s.resolveActiveNodeMap(ctx)
	if err != nil {
		return nil, err
	}

	settings := s.loadSettingsMap(ctx)
	hostDomain := settings["sub_domain"]
	if hostDomain == "" && baseURL != "" {
		hostDomain = baseURL
	}

	defaultPort, defaultFP := parsePortAndFingerprint(settings)

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

	for _, rawIn := range inbounds {
		if !rawIn.Enabled || !user.HasInbound(rawIn.Tag) || strings.EqualFold(rawIn.Protocol, "dokodemo-door") {
			continue
		}
		in, node, ok := prepareInboundForNode(rawIn, activeNodes)
		if !ok {
			continue
		}
		isVless := in.Protocol == "" || strings.EqualFold(in.Protocol, "vless")
		subRoutes := in.GetSubRoutes()
		if len(subRoutes) == 0 || !isVless {
			link := protocol.BuildShareLinkWithDefaultFP(&in, user, hostDomain, defaultPort, defaultFP)
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
		nodePrefix := ""
		if in.NodeID > 0 {
			nodePrefix = formatNodePrefix(node)
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
				tempIn.Remark = applyNodePrefix(sr.Name, nodePrefix)
			} else {
				tempIn.Remark = fmt.Sprintf("%s - 线路 #%d", baseName, sr.RouteID)
			}
			tempIn.RouteID = sr.RouteID
			link := protocol.BuildShareLinkWithDefaultFP(&tempIn, user, hostDomain, defaultPort, defaultFP)
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
