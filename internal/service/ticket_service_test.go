package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func setupTestTicketServiceDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test_ticket_service.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.User{}, &domain.Ticket{}, &domain.Inbound{}, &domain.Setting{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}

func TestGenerateTicketCode_Entropy(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := GenerateTicketCode()
		if err != nil {
			t.Fatalf("generate code err: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("expected code length 6, got %d", len(code))
		}
		for _, ch := range code {
			if !stringsContainsRune(crockfordCharset, ch) {
				t.Fatalf("invalid character %c in code %s", ch, code)
			}
		}
		if seen[code] {
			t.Fatalf("unexpected collision in 1000 codes: %s", code)
		}
		seen[code] = true
	}
}

func stringsContainsRune(s string, r rune) bool {
	for _, ch := range s {
		if ch == r {
			return true
		}
	}
	return false
}

func TestNormalizeTicketCode(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"7K9X2P", "7K9X2P"},
		{"７Ｋ９Ｘ２Ｐ", "7K9X2P"},                       // 全角大写
		{"７ｋ９ｘ２ｐ", "7K9X2P"},                       // 全角小写
		{" 7k-9_x.2p ", "7K9X2P"},                   // 空格与分隔符
		{"OI Loil", "011011"},                       // 混淆字符映射 (O/o->0, I/i/L/l->1)
		{"０１２３４５６７８９", "0123456789"},               // 全角数字
		{"ＡＢＣＤＥＦＧＨＪＫＭＮ", "ABCDEFGHJKMN"},         // 全角字母
	}

	for _, tc := range cases {
		got := NormalizeTicketCode(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeTicketCode(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestTicketService_GenerateAndClaim(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	ticketRepo := repository.NewTicketRepository(db)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	// 1. 设置系统参数
	_ = settingRepo.Set(ctx, "portal_url", "https://gateway.example.com")

	// 2. 创建活跃用户与入站节点
	user := &domain.User{
		Email:       "testuser@domain.com",
		UUID:        "11111111-2222-3333-4444-555555555555",
		SubToken:    "subtoken123",
		InboundTags: "vless-in",
		Enabled:     true,
		TotalBytes:  10 * 1024 * 1024 * 1024,
	}
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	inbound := &domain.Inbound{
		Tag:      "vless-in",
		Protocol: "vless",
		Port:     443,
		Enabled:  true,
		Remark:   "Tokyo Reality",
	}
	if err := inboundRepo.Create(ctx, inbound); err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	// 3. 生成票据
	ticket, shareText, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 2)
	if err != nil {
		t.Fatalf("generate ticket failed: %v", err)
	}
	if len(ticket.Code) != 6 {
		t.Fatalf("invalid ticket code length: %s", ticket.Code)
	}
	if ticket.RemainingUses != 2 {
		t.Fatalf("expected remaining uses 2, got %d", ticket.RemainingUses)
	}
	if len(shareText) == 0 {
		t.Fatalf("expected non-empty shareText")
	}

	// 4. 正常兑换 (全角小写测试)
	fullWidthInput := ""
	for _, r := range ticket.Code {
		if r >= '0' && r <= '9' {
			fullWidthInput += string(r - '0' + 0xFF10)
		} else if r >= 'A' && r <= 'Z' {
			fullWidthInput += string(r - 'A' + 0xFF41) // 全角小写
		} else {
			fullWidthInput += string(r)
		}
	}

	payload, err := ticketSvc.ClaimTicket(ctx, fullWidthInput, "10.0.0.1", "test.host")
	if err != nil {
		t.Fatalf("claim ticket failed: %v", err)
	}
	if payload.UserEmail != user.Email {
		t.Fatalf("email mismatch: %s vs %s", payload.UserEmail, user.Email)
	}
	if payload.RemainingUses != 1 {
		t.Fatalf("expected remaining uses 1 after first claim, got %d", payload.RemainingUses)
	}
	if len(payload.SubscriptionURL) == 0 {
		t.Fatalf("expected subscription url")
	}

	// 5. 第二次兑换
	payload2, err := ticketSvc.ClaimTicket(ctx, ticket.Code, "10.0.0.1", "test.host")
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}
	if payload2.RemainingUses != 0 {
		t.Fatalf("expected remaining uses 0 after second claim, got %d", payload2.RemainingUses)
	}

	// 6. 第三次兑换失败
	_, err = ticketSvc.ClaimTicket(ctx, ticket.Code, "10.0.0.1", "test.host")
	if !errors.Is(err, domain.ErrTicketInvalidOrExpired) {
		t.Fatalf("expected ErrTicketInvalidOrExpired on 3rd claim, got %v", err)
	}
}

func TestTicketService_UserDisabledOrExpired(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	ticketRepo := repository.NewTicketRepository(db)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	// 1. 禁用用户无法生成票据
	disabledUser := &domain.User{
		Email:    "disabled@test.com",
		UUID:     "uuid-d",
		SubToken: "token-d",
	}
	_ = userRepo.Create(ctx, disabledUser)
	_ = userRepo.UpdateFields(ctx, disabledUser.ID, map[string]interface{}{"enabled": false})

	_, _, err := ticketSvc.GenerateTicket(ctx, disabledUser.ID, 15, 2)
	if !errors.Is(err, domain.ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled on generating ticket, got: %v", err)
	}

	// 2. 生成票据后用户被禁用，兑换时拦截
	activeUser := &domain.User{
		Email:    "active-then-disabled@test.com",
		UUID:     "uuid-ad",
		SubToken: "token-ad",
		Enabled:  true,
	}
	_ = userRepo.Create(ctx, activeUser)
	ticket, _, _ := ticketSvc.GenerateTicket(ctx, activeUser.ID, 15, 2)

	// 禁用用户
	_ = userRepo.UpdateFields(ctx, activeUser.ID, map[string]interface{}{"enabled": false})

	_, err = ticketSvc.ClaimTicket(ctx, ticket.Code, "10.0.0.2", "host")
	if !errors.Is(err, domain.ErrUserDisabled) {
		t.Fatalf("expected ErrUserDisabled on claiming ticket for disabled user, got: %v", err)
	}
}

func TestTicketService_IPFailure_RateLimit(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	ticketRepo := repository.NewTicketRepository(db)
	userRepo := repository.NewUserRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, nil, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	attackerIP := "192.168.10.99"

	// 连续输错 4 次
	for i := 0; i < 4; i++ {
		_, err := ticketSvc.ClaimTicket(ctx, "WRONG1", attackerIP, "host")
		if !errors.Is(err, domain.ErrTicketInvalidOrExpired) {
			t.Fatalf("expected ErrTicketInvalidOrExpired, got %v", err)
		}
	}

	// 第 5 次输错，触发熔断
	_, err := ticketSvc.ClaimTicket(ctx, "WRONG2", attackerIP, "host")
	if !errors.Is(err, domain.ErrTicketInvalidOrExpired) {
		t.Fatalf("expected ErrTicketInvalidOrExpired on 5th failure, got %v", err)
	}

	// 第 6 次请求，直接被 IP 熔断拦截 (30分钟封禁)
	_, err = ticketSvc.ClaimTicket(ctx, "ANYKEY", attackerIP, "host")
	if !errors.Is(err, domain.ErrIPRateLimited) {
		t.Fatalf("expected ErrIPRateLimited on 6th request, got %v", err)
	}
}

func TestTicketService_ConcurrentClaims(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	ticketRepo := repository.NewTicketRepository(db)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	user := &domain.User{Email: "concurrent@test.com", UUID: "uuid-cc", SubToken: "tok-cc", Enabled: true}
	_ = userRepo.Create(ctx, user)
	inbound := &domain.Inbound{Tag: "in-1", Protocol: "vless", Port: 443, Enabled: true}
	_ = inboundRepo.Create(ctx, inbound)

	// 只允许兑换 1 次
	ticket, _, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 1)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	var successCount int32
	var failCount int32
	var wg sync.WaitGroup

	// 10 个并发请求同时抢兑同一个单次票据
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clientIP := "172.16.0." + string(rune('0'+idx))
			_, err := ticketSvc.ClaimTicket(ctx, ticket.Code, clientIP, "host")
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(err, domain.ErrTicketInvalidOrExpired) {
				atomic.AddInt32(&failCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if successCount != 1 {
		t.Fatalf("expected exactly 1 success in concurrent claim, got %d (fail count %d)", successCount, failCount)
	}
	if failCount != 9 {
		t.Fatalf("expected 9 failures in concurrent claim, got %d", failCount)
	}
}

func TestGenerateTicket_LoopbackFallback(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	user := &domain.User{
		Email:       "fallback@domain.com",
		UUID:        "22222222-3333-4444-5555-666666666666",
		SubToken:    "fallbacktoken",
		InboundTags: "vless-in",
		Enabled:     true,
		TotalBytes:  10 * 1024 * 1024 * 1024,
	}
	_ = userRepo.Create(ctx, user)

	// Case 1: public_url 是 127.0.0.1 默认值，未设 portal_url，传入外网 Host
	_ = settingRepo.Set(ctx, "public_url", "http://127.0.0.1:9000")
	_ = settingRepo.Set(ctx, "portal_url", "")

	ticket1, shareText1, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 2, "http://198.51.100.1:9000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(shareText1, "127.0.0.1") {
		t.Fatalf("expected shareText to not contain 127.0.0.1, got: %s", shareText1)
	}
	if !strings.Contains(shareText1, "http://198.51.100.1:9000/portal") {
		t.Fatalf("expected shareText to contain http://198.51.100.1:9000/portal, got: %s", shareText1)
	}
	if !strings.Contains(shareText1, ticket1.Code) {
		t.Fatalf("expected shareText to contain code %s", ticket1.Code)
	}

	// Case 2: 配置了专属中立 portal_url (如 Cloudflare Worker)
	_ = settingRepo.Set(ctx, "portal_url", "https://sub.myworker.workers.dev")
	ticket2, shareText2, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 2, "http://198.51.100.1:9000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(shareText2, "https://sub.myworker.workers.dev/portal") {
		t.Fatalf("expected shareText to prioritize portal_url, got: %s", shareText2)
	}
	if !strings.Contains(shareText2, ticket2.Code) {
		t.Fatalf("expected shareText to contain code %s", ticket2.Code)
	}
}

func TestTicketService_Claim_BurnAfterReading(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	user := &domain.User{Email: "burn@test.com", UUID: "uuid-burn", SubToken: "tok-burn", Enabled: true}
	_ = userRepo.Create(ctx, user)
	inbound := &domain.Inbound{Tag: "in-burn", Protocol: "vless", Port: 443, Enabled: true}
	_ = inboundRepo.Create(ctx, inbound)

	// 生成 1 次性凭证 (严格阅后即焚)
	ticket, _, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 1)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	payload, err := ticketSvc.ClaimTicket(ctx, ticket.Code, "10.0.0.1", "host")
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if payload.RemainingUses != 0 {
		t.Fatalf("expected remaining uses 0, got %d", payload.RemainingUses)
	}

	// 异步删除执行短暂缓冲
	time.Sleep(50 * time.Millisecond)

	// 验证数据库物理记录已彻底抹除
	_, err = ticketRepo.GetByCode(ctx, ticket.Code)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ticket to be physically deleted after uses exhausted, got err: %v", err)
	}
}

func TestTicketService_GenerateTicket_OverwriteOldTickets(t *testing.T) {
	db := setupTestTicketServiceDB(t)
	userRepo := repository.NewUserRepository(db)
	ticketRepo := repository.NewTicketRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	subSvc := NewSubService(userRepo, inboundRepo, settingRepo)
	ticketSvc := NewTicketService(ticketRepo, userRepo, subSvc, settingRepo)
	ctx := context.Background()

	user := &domain.User{Email: "user1@test.com", UUID: "uuid-1", SubToken: "tok-1", Enabled: true}
	_ = userRepo.Create(ctx, user)

	// 第 1 次生成提件码
	ticket1, _, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 2)
	if err != nil {
		t.Fatalf("generate 1 failed: %v", err)
	}

	// 确认 ticket1 存在
	if _, err := ticketRepo.GetByCode(ctx, ticket1.Code); err != nil {
		t.Fatalf("ticket1 should exist: %v", err)
	}

	// 第 2 次为同一用户生成提件码 (应该自动废除 ticket1)
	ticket2, _, err := ticketSvc.GenerateTicket(ctx, user.ID, 15, 2)
	if err != nil {
		t.Fatalf("generate 2 failed: %v", err)
	}

	// 验证 ticket1 已被物理删除失效
	_, err = ticketRepo.GetByCode(ctx, ticket1.Code)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ticket1 to be deleted upon generating ticket2, got err: %v", err)
	}

	// 验证 ticket2 存在且可以兑换
	_, err = ticketRepo.GetByCode(ctx, ticket2.Code)
	if err != nil {
		t.Fatalf("ticket2 should be valid and active: %v", err)
	}
}


