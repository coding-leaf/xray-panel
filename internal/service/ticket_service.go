package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"panel/internal/domain"

	"github.com/patrickmn/go-cache"
)

const crockfordCharset = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // len = 32

// ipFailureRecord 结构化记录客户端 IP 失败惩罚状态，区分滑动窗口与封禁期
type ipFailureRecord struct {
	FailCount   int
	WindowStart time.Time
	BannedUntil time.Time
}

type TicketService struct {
	ticketRepo    domain.TicketRepository
	userRepo      domain.UserRepository
	subSvc        *SubService
	settingRepo   domain.SettingRepository
	failedIPCache *cache.Cache
	ipMutex       sync.Mutex
}

func NewTicketService(
	ticketRepo domain.TicketRepository,
	userRepo domain.UserRepository,
	subSvc *SubService,
	settingRepo domain.SettingRepository,
) *TicketService {
	return &TicketService{
		ticketRepo:    ticketRepo,
		userRepo:      userRepo,
		subSvc:        subSvc,
		settingRepo:   settingRepo,
		failedIPCache: cache.New(1*time.Hour, 10*time.Minute),
	}
}

// GenerateTicketCode 使用 crypto/rand 生成 6 位标准 Crockford Base32 编码 (无偏位运算采样)
func GenerateTicketCode() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("read crypto rand failed: %w", err)
	}

	var sb strings.Builder
	sb.Grow(6)
	for _, b := range bytes {
		// 0..255 通过 b & 0x1F 无偏映射到 0..31 (256 能被 32 整除，每种结果概率恰好为 8/256)
		idx := b & 0x1F
		sb.WriteByte(crockfordCharset[idx])
	}
	return sb.String(), nil
}

// NormalizeTicketCode 对用户输入的提取码进行清洗：全角转半角、去空白符号、转大写、混淆字符映射
func NormalizeTicketCode(input string) string {
	// 1. 全角字符 (Full-width) 转半角 ASCII
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		if r == 0x3000 {
			b.WriteRune(' ')
		} else if r >= 0xFF01 && r <= 0xFF5E {
			b.WriteRune(r - 0xFEE0)
		} else {
			b.WriteRune(r)
		}
	}
	s := b.String()

	// 2. 剥离所有空白、短横线、下划线及点号
	f := func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '_' || r == '.'
	}
	parts := strings.FieldsFunc(s, f)
	s = strings.Join(parts, "")

	// 3. 转大写
	s = strings.ToUpper(s)

	// 4. Crockford 容错映射 (O/o -> 0, I/i/L/l -> 1)
	replacer := strings.NewReplacer("O", "0", "I", "1", "L", "1")
	return replacer.Replace(s)
}

func (s *TicketService) checkIPStatus(clientIP string) error {
	if clientIP == "" {
		return nil
	}
	s.ipMutex.Lock()
	defer s.ipMutex.Unlock()

	now := time.Now()
	val, found := s.failedIPCache.Get(clientIP)
	if !found {
		return nil
	}
	rec := val.(ipFailureRecord)
	if now.Before(rec.BannedUntil) {
		return domain.ErrIPRateLimited
	}
	return nil
}

func (s *TicketService) recordIPFailure(clientIP string) {
	if clientIP == "" {
		return
	}
	s.ipMutex.Lock()
	defer s.ipMutex.Unlock()

	now := time.Now()
	val, found := s.failedIPCache.Get(clientIP)
	var rec ipFailureRecord
	if !found {
		rec = ipFailureRecord{
			FailCount:   1,
			WindowStart: now,
		}
	} else {
		rec = val.(ipFailureRecord)
		if now.Sub(rec.WindowStart) > 10*time.Minute {
			rec.FailCount = 1
			rec.WindowStart = now
			rec.BannedUntil = time.Time{}
		} else {
			rec.FailCount++
			if rec.FailCount >= 5 {
				rec.BannedUntil = now.Add(30 * time.Minute)
			}
		}
	}
	s.failedIPCache.Set(clientIP, rec, 1*time.Hour)
}

func (s *TicketService) clearIPFailure(clientIP string) {
	if clientIP == "" {
		return
	}
	s.ipMutex.Lock()
	defer s.ipMutex.Unlock()

	s.failedIPCache.Delete(clientIP)
}

// GenerateTicket 为指定用户生成 6 位临时提取凭证，并格式化微信分享文案
func (s *TicketService) GenerateTicket(ctx context.Context, userID uint, ttlMinutes int, maxUses int, fallbackBaseURL ...string) (*domain.Ticket, string, error) {
	if ttlMinutes <= 0 {
		ttlMinutes = 15
	}
	if maxUses <= 0 {
		maxUses = 2
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if !user.Enabled {
		return nil, "", domain.ErrUserDisabled
	}

	code, err := GenerateTicketCode()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate ticket code: %w", err)
	}

	now := time.Now().Unix()
	ticket := &domain.Ticket{
		Code:          code,
		UserID:        userID,
		RemainingUses: maxUses,
		ExpiresAt:     now + int64(ttlMinutes*60),
		CreatedAt:     now,
	}

	// 单用户单活动码策略：为该用户生成新提件码前，自动废弃并物理清理该用户之前所有未用完的旧安全码 (覆写策略)
	if s.ticketRepo != nil {
		_ = s.ticketRepo.DeleteByUserID(ctx, userID)
	}

	if err := s.ticketRepo.Create(ctx, ticket); err != nil {
		return nil, "", err
	}

	// 安全异步清理过期凭证，脱钩请求上下文，防 context canceled
	go func() {
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.ticketRepo.CleanExpired(cleanCtx)
	}()

	// 优先从配置项读取中立门户域名（portal_url），其次读取面板公网域名（public_url）
	var baseURL string
	if s.settingRepo != nil {
		baseURL, _ = s.settingRepo.Get(ctx, "portal_url")
		if baseURL == "" {
			baseURL, _ = s.settingRepo.Get(ctx, "public_url")
		}
	}
	// 若配置未指定，或为本地环回 (127.0.0.1 / localhost)，则优先降级为当前请求实际访问的 Host
	isLoopback := baseURL == "" || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "localhost")
	if isLoopback && len(fallbackBaseURL) > 0 && fallbackBaseURL[0] != "" {
		baseURL = fallbackBaseURL[0]
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:9000"
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	portalURL := fmt.Sprintf("%s/portal", strings.TrimRight(baseURL, "/"))
	shareText := fmt.Sprintf("提取入口：%s ，提件码：%s（%d分钟内有效）", portalURL, code, ttlMinutes)

	return ticket, shareText, nil
}

// ClaimTicket 兑换凭证，执行 IP 熔断检查、CAS 原子扣减与用户状态核验，下发双轨数据
func (s *TicketService) ClaimTicket(ctx context.Context, rawCode string, clientIP string, reqHost string) (*domain.TicketClaimPayload, error) {
	if err := s.checkIPStatus(clientIP); err != nil {
		return nil, err
	}

	cleanCode := NormalizeTicketCode(rawCode)
	if len(cleanCode) != 6 {
		s.recordIPFailure(clientIP)
		return nil, domain.ErrTicketInvalidOrExpired
	}

	// CAS 条件原子扣减，仅有抢占成功的请求可继续获取节点
	ticket, err := s.ticketRepo.ConsumeAtomic(ctx, cleanCode)
	if err != nil {
		s.recordIPFailure(clientIP)
		return nil, domain.ErrTicketInvalidOrExpired
	}

	// 兑换成功，立即释放该 IP 的失败惩罚
	s.clearIPFailure(clientIP)

	// 若兑换后剩余次数已归零（阅后即焚），立即异步物理销毁该凭据数据库记录
	if ticket.RemainingUses <= 0 {
		go func(tid uint) {
			delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.ticketRepo.Delete(delCtx, tid)
		}(ticket.ID)
	}

	// 二次安全核验用户状态：若账号已禁用或欠费超额，阻断下发
	user := ticket.User
	if user == nil {
		user, err = s.userRepo.GetByID(ctx, ticket.UserID)
		if err != nil || user == nil {
			return nil, domain.ErrNotFound
		}
	}
	if !user.Enabled {
		return nil, domain.ErrUserDisabled
	}
	if !user.IsActive() {
		return nil, domain.ErrQuotaExceeded
	}

	// 解析中立或公网基础 URL
	var baseURL string
	if s.settingRepo != nil {
		baseURL, _ = s.settingRepo.Get(ctx, "portal_url")
		if baseURL == "" {
			baseURL, _ = s.settingRepo.Get(ctx, "public_url")
		}
	}
	isLoopback := baseURL == "" || strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "localhost")
	if isLoopback && reqHost != "" {
		baseURL = reqHost
	}
	if baseURL != "" && !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	shareInfo, err := s.subSvc.GetUserShareInfo(ctx, user.ID, baseURL)
	if err != nil {
		return nil, err
	}

	emergencyNodes := make([]string, 0, len(shareInfo.Nodes))
	for _, node := range shareInfo.Nodes {
		if node.ShareLink != "" {
			emergencyNodes = append(emergencyNodes, node.ShareLink)
		}
	}

	return &domain.TicketClaimPayload{
		UserEmail:       user.Email,
		EmergencyNodes:  emergencyNodes,
		SubscriptionURL: shareInfo.AllSubURL,
		RemainingUses:   ticket.RemainingUses,
		ExpiresAt:       ticket.ExpiresAt,
	}, nil
}
