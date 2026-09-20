package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"panel/internal/app"
	"panel/internal/domain"
	"panel/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var _ app.Service = (*BotHandler)(nil)

// XrayServiceController 定义 Telegram Bot 所需的最小服务控制契约 (遵循 ISP 接口隔离原则)
type XrayServiceController interface {
	GetServiceStatus(ctx context.Context) (domain.ServiceStatus, error)
	GetVersion(ctx context.Context) (string, error)
	RestartService(ctx context.Context) error
}

// UserServiceController 定义 Telegram Bot 所需的用户业务编排契约 (遵循 ISP 原则)
type UserServiceController interface {
	CreateUser(ctx context.Context, dto service.CreateUserDTO) (*domain.User, error)
	UpdateUser(ctx context.Context, id uint, dto domain.UpdateUserDTO) (*domain.User, error)
	ResetTraffic(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

type BotHandler struct {
	adapter     *BotAdapter
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
	monitor     domain.HostMonitor
	xrayManager XrayServiceController
	userSvc     UserServiceController
	publicURL   string
	actionMu    sync.Mutex
}

func NewBotHandler(
	adapter *BotAdapter,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	monitor domain.HostMonitor,
	xrayManager XrayServiceController,
	userSvc UserServiceController,
	publicURL string,
) *BotHandler {
	return &BotHandler{
		adapter:     adapter,
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		monitor:     monitor,
		xrayManager: xrayManager,
		userSvc:     userSvc,
		publicURL:   publicURL,
	}
}

func (h *BotHandler) registerBotCommands(bot *tgbotapi.BotAPI) {
	if bot == nil {
		return
	}
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "status", Description: "📊 查看系统与 Xray 运行状态"},
		tgbotapi.BotCommand{Command: "user", Description: "👤 查询指定用户信息与快捷管理"},
		tgbotapi.BotCommand{Command: "adduser", Description: "➕ 创建新用户并生成专属订阅"},
		tgbotapi.BotCommand{Command: "traffic", Description: "📈 查看用户流量消耗概览"},
		tgbotapi.BotCommand{Command: "sub", Description: "🔗 获取指定用户的专属订阅"},
		tgbotapi.BotCommand{Command: "restart", Description: "🔄 重启 Xray 核心服务"},
		tgbotapi.BotCommand{Command: "menu", Description: "📱 呼出快捷交互菜单键盘"},
		tgbotapi.BotCommand{Command: "help", Description: "❓ 查看帮助指引说明"},
	)
	if _, err := bot.Request(commands); err != nil {
		slog.Warn("Failed to sync telegram bot cloud commands", slog.String("error", err.Error()))
	} else {
		slog.Info("Telegram bot cloud commands registered successfully", slog.String("username", bot.Self.UserName))
	}
}

type contextAwareHTTPClient struct {
	base tgbotapi.HTTPClient
	ctx  context.Context
}

func (c *contextAwareHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if c.ctx != nil {
		req = req.WithContext(c.ctx)
	}
	if c.base != nil {
		return c.base.Do(req)
	}
	return http.DefaultClient.Do(req)
}

// Start runs the Telegram bot polling loop as an app.Service.
// When ctx is canceled, it immediately stops long-polling and returns cleanly.
func (h *BotHandler) Start(ctx context.Context) error {
	slog.Info("Telegram Bot service started")

	for {
		if ctx.Err() != nil {
			slog.Info("Telegram Bot service stopped gracefully")
			return nil
		}

		h.adapter.mu.RLock()
		bot := h.adapter.bot
		h.adapter.mu.RUnlock()

		if bot == nil {
			select {
			case <-ctx.Done():
				slog.Info("Telegram Bot service stopped gracefully")
				return nil
			case <-h.adapter.ReloadChan:
				continue
			case <-time.After(3 * time.Second):
				continue
			}
		}

		// 自动注册云端 Menu 按钮指令列表
		h.registerBotCommands(bot)

		// 注入感知 context 的 HTTP 客户端，当 ctx 取消时立即中断底层 HTTP 长轮询连接
		origClient := bot.Client
		bot.Client = &contextAwareHTTPClient{
			base: origClient,
			ctx:  ctx,
		}

		u := tgbotapi.NewUpdate(0)
		u.Timeout = 30
		slog.Info("Telegram Bot long polling started", slog.String("username", bot.Self.UserName))

		var wg sync.WaitGroup

	pollLoop:
		for {
			select {
			case <-ctx.Done():
				slog.Info("Telegram Bot context canceled, disconnecting long-polling...")
				break pollLoop
			case <-h.adapter.ReloadChan:
				slog.Info("Telegram Bot reloading configuration...")
				break pollLoop
			default:
			}

			updates, err := bot.GetUpdates(u)
			if err != nil {
				if ctx.Err() != nil {
					slog.Info("Telegram Bot context canceled during updates fetch")
					break pollLoop
				}
				// 针对网络偶发波动退避重试，允许被 context 或 reload 立即打断
				select {
				case <-ctx.Done():
					break pollLoop
				case <-h.adapter.ReloadChan:
					break pollLoop
				case <-time.After(3 * time.Second):
					continue pollLoop
				}
			}

			for _, update := range updates {
				if update.UpdateID >= u.Offset {
					u.Offset = update.UpdateID + 1
				}
				if update.Message != nil {
					msg := update.Message
					wg.Add(1)
					go func() {
						defer wg.Done()
						h.handleMessage(ctx, msg)
					}()
				}
				if update.CallbackQuery != nil {
					cb := update.CallbackQuery
					wg.Add(1)
					go func() {
						defer wg.Done()
						h.handleCallbackQuery(ctx, cb)
					}()
				}
			}
		}

		wg.Wait()

		// 还原原始 Client
		bot.Client = origClient

		if ctx.Err() != nil {
			slog.Info("Telegram Bot service stopped gracefully")
			return nil
		}
	}
}

// StartPolling provides backward compatibility for asynchronous callers.
func (h *BotHandler) StartPolling(ctx context.Context) {
	go func() {
		_ = h.Start(ctx)
	}()
}

func (h *BotHandler) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	h.adapter.mu.RLock()
	adminChatID := h.adapter.adminChatID
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	// 1. 若尚未在后台绑定管理员 Chat ID，向任意交互用户提示其 Chat ID 以便于填入后台
	if adminChatID == 0 {
		text := fmt.Sprintf(
			"👋 <b>欢迎使用 Xray 面板运维机器人</b>\n\n"+
				"🆔 <b>您的 Telegram Chat ID 为:</b> <code>%d</code>\n\n"+
				"💡 <b>请复制上方数字 ID</b>，登录面板并在【系统设置】->【管理员 Chat ID】中填入并保存，即可完成管理员权限绑定！",
			msg.Chat.ID,
		)
		h.reply(msg.Chat.ID, text)
		return
	}

	// 2. 权限校验
	if msg.Chat.ID != adminChatID {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "⛔ 无权使用此机器人。")
		_, _ = bot.Send(reply)
		return
	}

	cmd := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())
	rawText := strings.TrimSpace(msg.Text)

	// 3. 支持快捷菜单按钮文字点击
	if !msg.IsCommand() {
		switch rawText {
		case "📊 系统状态", "📊 状态", "状态":
			cmd = "status"
		case "📈 流量统计", "📈 流量", "流量":
			cmd = "traffic"
		case "🔄 重启 Xray", "🔄 重启服务", "重启":
			cmd = "restart"
		case "❓ 帮助指引", "❓ 帮助", "帮助":
			cmd = "help"
		case "📱 快捷菜单", "菜单":
			cmd = "menu"
		default:
			h.sendMenuWithKeyboard(msg.Chat.ID, "🤖 <b>Xray 独立运维管理机器人</b>\n\n请点击下方快捷按钮或发送指令：")
			return
		}
	}

	switch cmd {
	case "start", "menu", "help":
		text := "🤖 <b>Xray 独立运维管理机器人</b>\n\n" +
			"📊 /status - 查看系统与 Xray 运行状态\n" +
			"👤 /user &lt;email&gt; - 查询用户卡片并进行快捷管理\n" +
			"➕ /adduser &lt;email&gt; [GB] [天数] - 创建用户并生成专属订阅\n" +
			"📈 /traffic - 查看用户流量消耗统计\n" +
			"🔗 /sub &lt;email&gt; - 获取指定用户专属订阅\n" +
			"🔄 /restart - 重启 Xray 核心服务\n" +
			"📱 /menu - 呼出快捷交互键盘\n\n" +
			"<i>可直接点击下方快捷按钮进行操作：</i>"
		h.sendMenuWithKeyboard(msg.Chat.ID, text)

	case "status":
		var metrics *domain.SystemMetrics
		if h.monitor != nil {
			var err error
			metrics, err = h.monitor.GetSystemMetrics(ctx)
			if err != nil {
				h.reply(msg.Chat.ID, "❌ 获取系统指标失败: "+err.Error())
				return
			}
		}
		var svcStatus domain.ServiceStatus
		var xrayVer string
		if h.xrayManager != nil {
			svcStatus, _ = h.xrayManager.GetServiceStatus(ctx)
			xrayVer, _ = h.xrayManager.GetVersion(ctx)
		}

		text, keyboard := RenderStatusCard(metrics, svcStatus, xrayVer, time.Now())
		h.replyWithKeyboard(msg.Chat.ID, text, keyboard)

	case "user":
		if args == "" {
			h.reply(msg.Chat.ID, "用法: <code>/user &lt;email&gt;</code>\n示例: <code>/user test@example.com</code>")
			return
		}
		email := strings.TrimSpace(args)
		var user *domain.User
		var err error
		if h.userSvc != nil {
			user, err = h.userSvc.GetByEmail(ctx, email)
		} else if h.userRepo != nil {
			user, err = h.userRepo.GetByEmail(ctx, email)
		}
		if err != nil || user == nil {
			h.reply(msg.Chat.ID, fmt.Sprintf("❌ 未找到用户: <code>%s</code>", email))
			return
		}
		text, keyboard := RenderUserCard(user, h.publicURL)
		h.replyWithKeyboard(msg.Chat.ID, text, keyboard)

	case "adduser":
		email, totalBytes, expireDays, err := ParseAddUserCommand(args)
		if err != nil {
			h.reply(msg.Chat.ID, fmt.Sprintf("❌ 参数错误: %s", err.Error()))
			return
		}

		if h.userSvc == nil {
			h.reply(msg.Chat.ID, "❌ 用户服务未就绪")
			return
		}

		var activeTags []string
		if h.inboundRepo != nil {
			inbounds, inErr := h.inboundRepo.ListAll(ctx)
			if inErr == nil {
				for _, in := range inbounds {
					if in.Enabled {
						activeTags = append(activeTags, in.Tag)
					}
				}
			}
		}

		if len(activeTags) == 0 {
			h.reply(msg.Chat.ID, "❌ 创建失败: 当前无可用或启用的入站节点，请先在面板添加入站节点")
			return
		}

		dto := service.CreateUserDTO{
			Email:       email,
			TotalBytes:  totalBytes,
			ExpireDays:  expireDays,
			InboundTags: activeTags,
		}

		user, err := h.userSvc.CreateUser(ctx, dto)
		if err != nil {
			h.reply(msg.Chat.ID, fmt.Sprintf("❌ 创建用户失败: %s", err.Error()))
			return
		}

		subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(h.publicURL, "/"), user.SubToken)
		_, keyboard := RenderUserCard(user, h.publicURL)

		usedGB := float64(user.UpBytes+user.DownBytes) / (1024 * 1024 * 1024)
		totalStr := "无限制"
		if user.TotalBytes > 0 {
			totalStr = fmt.Sprintf("%.2f GB", float64(user.TotalBytes)/(1024*1024*1024))
		}
		expireStr := "永不过期"
		if user.ExpireTime > 0 {
			expireStr = time.UnixMilli(user.ExpireTime).Format("2006-01-02 15:04:05")
		}

		resText := fmt.Sprintf(
			"✅ <b>用户创建成功！</b>\n\n"+
				"📧 <b>邮箱:</b> <code>%s</code>\n"+
				"🔑 <b>UUID:</b> <code>%s</code>\n"+
				"📊 <b>流量配额:</b> %.2f GB / %s\n"+
				"⏳ <b>到期时间:</b> %s\n"+
				"🌐 <b>专属订阅:</b>\n<code>%s</code>",
			user.Email, user.UUID, usedGB, totalStr, expireStr, subURL,
		)

		h.replyWithKeyboard(msg.Chat.ID, resText, keyboard)

	case "traffic":
		users, err := h.userRepo.ListAll(ctx)
		if err != nil {
			h.reply(msg.Chat.ID, "❌ 获取用户列表失败: "+err.Error())
			return
		}

		var sb strings.Builder
		sb.WriteString("📈 <b>用户流量消耗概览</b>\n\n")
		for _, u := range users {
			usedGB := float64(u.UpBytes+u.DownBytes) / (1024 * 1024 * 1024)
			totalGB := float64(u.TotalBytes) / (1024 * 1024 * 1024)
			totalStr := fmt.Sprintf("%.2f GB", totalGB)
			if u.TotalBytes <= 0 {
				totalStr = "无限制"
			}
			state := "🟢"
			if !u.IsActive() {
				state = "🔴"
			}
			sb.WriteString(fmt.Sprintf("%s <code>%s</code>: %.2f GB / %s\n", state, u.Email, usedGB, totalStr))
		}
		h.reply(msg.Chat.ID, sb.String())

	case "sub":
		if args == "" {
			h.reply(msg.Chat.ID, "用法: <code>/sub 用户邮箱</code>\n示例: <code>/sub test@example.com</code>")
			return
		}
		u, err := h.userRepo.GetByEmail(ctx, args)
		if err != nil {
			h.reply(msg.Chat.ID, "❌ 未找到该用户")
			return
		}
		subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(h.publicURL, "/"), u.SubToken)
		text := fmt.Sprintf(
			"🔗 <b>用户专属订阅链接</b>\n\n"+
				"👤 <b>用户:</b> <code>%s</code>\n"+
				"🔑 <b>Token:</b> <code>%s</code>\n"+
				"🌐 <b>链接:</b>\n<code>%s</code>",
			u.Email, u.SubToken, subURL,
		)
		h.reply(msg.Chat.ID, text)

	case "restart":
		h.reply(msg.Chat.ID, "⏳ 正在重启 Xray 服务...")
		err := h.xrayManager.RestartService(ctx)
		if err != nil {
			h.reply(msg.Chat.ID, "❌ 重启失败: "+err.Error())
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
		status, _ := h.xrayManager.GetServiceStatus(ctx)
		if status.Active {
			h.reply(msg.Chat.ID, "✅ Xray 服务已成功平滑重启并恢复运行！")
		} else {
			h.reply(msg.Chat.ID, "⚠️ Xray 重启后状态异常: "+status.SubState)
		}

	default:
		h.sendMenuWithKeyboard(msg.Chat.ID, "❓ 未知指令。请使用下方快捷菜单或发送 /help。")
	}
}

func (h *BotHandler) sendMenuWithKeyboard(chatID int64, htmlText string) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = "HTML"
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("📊 系统状态"),
			tgbotapi.NewKeyboardButton("📈 流量统计"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🔄 重启 Xray"),
			tgbotapi.NewKeyboardButton("❓ 帮助指引"),
		),
	)
	keyboard.ResizeKeyboard = true
	msg.ReplyMarkup = keyboard
	_, _ = bot.Send(msg)
}

func (h *BotHandler) reply(chatID int64, htmlText string) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = "HTML"
	_, _ = bot.Send(msg)
}

func (h *BotHandler) replyWithKeyboard(chatID int64, htmlText string, markup interface{}) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	msg := tgbotapi.NewMessage(chatID, htmlText)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = markup
	_, _ = bot.Send(msg)
}

func (h *BotHandler) editMessage(chatID int64, messageID int, htmlText string, keyboard *tgbotapi.InlineKeyboardMarkup) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	edit := tgbotapi.NewEditMessageText(chatID, messageID, htmlText)
	edit.ParseMode = "HTML"
	if keyboard != nil {
		edit.ReplyMarkup = keyboard
	}
	_, _ = bot.Send(edit)
}

func (h *BotHandler) handleCallbackQuery(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb == nil {
		return
	}

	h.adapter.mu.RLock()
	adminChatID := h.adapter.adminChatID
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	// 1. 鉴权守卫：adminChatID 校验
	if adminChatID == 0 {
		alertMsg := fmt.Sprintf("⚠️ 面板尚未绑定管理员 Chat ID。\n您的 Telegram ID: %d", cb.From.ID)
		cfg := tgbotapi.NewCallbackWithAlert(cb.ID, alertMsg)
		_, _ = bot.Request(cfg)
		return
	}

	if cb.From.ID != adminChatID {
		cfg := tgbotapi.NewCallbackWithAlert(cb.ID, "⛔ 无权操作")
		_, _ = bot.Request(cfg)
		return
	}

	// 2. TryLock 防并发连击保护
	if !h.actionMu.TryLock() {
		cfg := tgbotapi.NewCallback(cb.ID, "⚠️ 操作正在执行中，请勿重复点击")
		cfg.ShowAlert = false
		_, _ = bot.Request(cfg)
		return
	}
	defer h.actionMu.Unlock()

	answered := false
	answer := func(text string, showAlert bool) {
		if answered {
			return
		}
		answered = true
		cfg := tgbotapi.NewCallback(cb.ID, text)
		cfg.ShowAlert = showAlert
		_, _ = bot.Request(cfg)
	}
	defer func() {
		if !answered {
			answer("", false)
		}
	}()

	domainName, action, targetID, err := ParseCallbackData(cb.Data)
	if err != nil {
		answer("❌ 无效的指令数据", true)
		return
	}

	if cb.Message == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	msgID := cb.Message.MessageID

	switch domainName {
	case "status":
		switch action {
		case "refresh":
			var metrics *domain.SystemMetrics
			if h.monitor != nil {
				metrics, _ = h.monitor.GetSystemMetrics(ctx)
			}
			var svcStatus domain.ServiceStatus
			var xrayVer string
			if h.xrayManager != nil {
				svcStatus, _ = h.xrayManager.GetServiceStatus(ctx)
				xrayVer, _ = h.xrayManager.GetVersion(ctx)
			}
			text, keyboard := RenderStatusCard(metrics, svcStatus, xrayVer, time.Now())
			h.editMessage(chatID, msgID, text, &keyboard)
			answer("✅ 状态指标已刷新", false)

		case "restart":
			answer("⏳ 正在重启 Xray 服务...", false)
			if h.xrayManager != nil {
				_ = h.xrayManager.RestartService(ctx)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			var metrics *domain.SystemMetrics
			if h.monitor != nil {
				metrics, _ = h.monitor.GetSystemMetrics(ctx)
			}
			var svcStatus domain.ServiceStatus
			var xrayVer string
			if h.xrayManager != nil {
				svcStatus, _ = h.xrayManager.GetServiceStatus(ctx)
				xrayVer, _ = h.xrayManager.GetVersion(ctx)
			}
			text, keyboard := RenderStatusCard(metrics, svcStatus, xrayVer, time.Now())
			h.editMessage(chatID, msgID, text, &keyboard)

		default:
			answer("❓ 未知操作", true)
		}

	case "user":
		if h.userSvc == nil {
			answer("❌ 用户服务未就绪", true)
			return
		}

		switch action {
		case "toggle":
			user, err := h.userSvc.GetByID(ctx, targetID)
			if err != nil || user == nil {
				answer("❌ 用户不存在", true)
				return
			}
			newEnabled := !user.Enabled
			updatedUser, err := h.userSvc.UpdateUser(ctx, targetID, domain.UpdateUserDTO{
				Enabled: &newEnabled,
			})
			if err != nil {
				answer(fmt.Sprintf("❌ 操作失败: %s", err.Error()), true)
				return
			}
			text, keyboard := RenderUserCard(updatedUser, h.publicURL)
			h.editMessage(chatID, msgID, text, &keyboard)
			statusStr := "已启用"
			if !updatedUser.Enabled {
				statusStr = "已禁用"
			}
			answer(fmt.Sprintf("✅ 用户 %s %s", updatedUser.Email, statusStr), false)

		case "reset":
			if err := h.userSvc.ResetTraffic(ctx, targetID); err != nil {
				answer(fmt.Sprintf("❌ 重置失败: %s", err.Error()), true)
				return
			}
			user, err := h.userSvc.GetByID(ctx, targetID)
			if err != nil || user == nil {
				answer("❌ 用户不存在", true)
				return
			}
			text, keyboard := RenderUserCard(user, h.publicURL)
			h.editMessage(chatID, msgID, text, &keyboard)
			answer(fmt.Sprintf("✅ 用户 %s 流量已重置", user.Email), false)

		case "sub":
			user, err := h.userSvc.GetByID(ctx, targetID)
			if err != nil || user == nil {
				answer("❌ 用户不存在", true)
				return
			}
			subURL := fmt.Sprintf("%s/sub/%s", strings.TrimRight(h.publicURL, "/"), user.SubToken)
			answer(fmt.Sprintf("🔗 专属订阅链接:\n%s", subURL), true)

		case "refresh":
			user, err := h.userSvc.GetByID(ctx, targetID)
			if err != nil || user == nil {
				answer("❌ 用户不存在", true)
				return
			}
			text, keyboard := RenderUserCard(user, h.publicURL)
			h.editMessage(chatID, msgID, text, &keyboard)
			answer("✅ 用户状态已刷新", false)

		default:
			answer("❓ 未知操作", true)
		}

	default:
		answer("❓ 未知领域指令", true)
	}
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ParseAddUserCommand 解析 /adduser 参数
func ParseAddUserCommand(args string) (email string, totalBytes int64, expireDays int, err error) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return "", 0, 0, fmt.Errorf("参数不能为空，用法: /adduser <email> [GB] [天数]")
	}

	parts := strings.Fields(trimmed)
	if len(parts) > 3 {
		return "", 0, 0, fmt.Errorf("参数过多，用法: /adduser <email> [GB] [天数]")
	}

	email = parts[0]
	if !emailRegex.MatchString(email) {
		return "", 0, 0, fmt.Errorf("无效的邮箱格式: %s", email)
	}

	if len(parts) >= 2 {
		gb, parseErr := strconv.ParseFloat(parts[1], 64)
		if parseErr != nil {
			return "", 0, 0, fmt.Errorf("无效的流量限制数值: %s", parts[1])
		}
		if gb < 0 {
			return "", 0, 0, fmt.Errorf("流量限制不能为负数")
		}
		totalBytes = int64(gb * 1024 * 1024 * 1024)
	}

	if len(parts) >= 3 {
		days, parseErr := strconv.Atoi(parts[2])
		if parseErr != nil {
			return "", 0, 0, fmt.Errorf("无效的有效天数: %s", parts[2])
		}
		if days < 0 {
			return "", 0, 0, fmt.Errorf("有效天数不能为负数")
		}
		expireDays = days
	}

	return email, totalBytes, expireDays, nil
}

// ParseCallbackData 解析 Telegram 内联按钮的回调数据
func ParseCallbackData(data string) (domain string, action string, targetID uint, err error) {
	if len(data) > 64 {
		return "", "", 0, fmt.Errorf("回调数据超出 64 字节长度限制")
	}
	parts := strings.Split(data, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", 0, fmt.Errorf("无效的回调数据格式")
	}

	domain = parts[0]
	action = parts[1]
	if domain == "" || action == "" {
		return "", "", 0, fmt.Errorf("回调 domain 或 action 不能为空")
	}

	if domain == "user" {
		if len(parts) != 3 {
			return "", "", 0, fmt.Errorf("用户操作必须指定用户 ID")
		}
		id, parseErr := strconv.ParseUint(parts[2], 10, 64)
		if parseErr != nil || id == 0 {
			return "", "", 0, fmt.Errorf("无效的用户 ID: %s", parts[2])
		}
		targetID = uint(id)
		return domain, action, targetID, nil
	}

	if len(parts) == 3 {
		id, parseErr := strconv.ParseUint(parts[2], 10, 64)
		if parseErr != nil {
			return "", "", 0, fmt.Errorf("无效的目标 ID: %s", parts[2])
		}
		targetID = uint(id)
	}

	return domain, action, targetID, nil
}

// RenderStatusCard 渲染系统与核心状态卡片及操作按钮
func RenderStatusCard(metrics *domain.SystemMetrics, svcStatus domain.ServiceStatus, xrayVer string, updateTime time.Time) (string, tgbotapi.InlineKeyboardMarkup) {
	var (
		cpuUsage    float64
		memUsedGB   float64
		memTotalGB  float64
		memPct      float64
		diskUsedGB  float64
		diskTotalGB float64
		diskPct     float64
		netUpMB     float64
		netDownMB   float64
		uptimeHours uint64
	)
	if metrics != nil {
		cpuUsage = metrics.CPUUsagePercent
		memUsedGB = float64(metrics.MemoryUsedBytes) / (1024 * 1024 * 1024)
		memTotalGB = float64(metrics.MemoryTotalBytes) / (1024 * 1024 * 1024)
		memPct = metrics.MemoryUsagePct
		diskUsedGB = float64(metrics.DiskUsedBytes) / (1024 * 1024 * 1024)
		diskTotalGB = float64(metrics.DiskTotalBytes) / (1024 * 1024 * 1024)
		diskPct = metrics.DiskUsagePct
		netUpMB = float64(metrics.NetUpSpeedBps) / (1024 * 1024)
		netDownMB = float64(metrics.NetDownSpeedBps) / (1024 * 1024)
		uptimeHours = metrics.UptimeSeconds / 3600
	}

	activeStr := "🔴 已停止"
	if svcStatus.Active {
		activeStr = "🟢 正常运行"
	}

	timeStr := updateTime.Format("2006-01-02 15:04:05")

	text := fmt.Sprintf(
		"📊 <b>系统运行状态</b>\n\n"+
			"🖥️ <b>CPU:</b> %.1f%%\n"+
			"🧠 <b>内存:</b> %.2f GB / %.2f GB (%.1f%%)\n"+
			"💾 <b>磁盘:</b> %.2f GB / %.2f GB (%.1f%%)\n"+
			"⚡ <b>网络速率:</b> ⬆️ %.2f MB/s | ⬇️ %.2f MB/s\n"+
			"⏱️ <b>开机时长:</b> %d 小时\n\n"+
			"🚀 <b>Xray:</b> %s (%s)\n"+
			"🏷️ <b>版本:</b> <code>%s</code>\n"+
			"🕒 <b>更新时间:</b> %s",
		cpuUsage, memUsedGB, memTotalGB, memPct, diskUsedGB, diskTotalGB, diskPct,
		netUpMB, netDownMB, uptimeHours, activeStr, svcStatus.SubState, xrayVer, timeStr,
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 刷新指标", "status:refresh"),
			tgbotapi.NewInlineKeyboardButtonData("⚡ 重启核心", "status:restart"),
		),
	)

	return text, keyboard
}

// RenderUserCard 渲染用户详情卡片及管理操作按钮
func RenderUserCard(user *domain.User, publicURL string) (string, tgbotapi.InlineKeyboardMarkup) {
	if user == nil {
		return "❌ 用户不存在", tgbotapi.InlineKeyboardMarkup{}
	}

	statusStr := "🟢 正常"
	if !user.Enabled {
		statusStr = "🔴 已禁用"
	} else if user.IsExpired() {
		statusStr = "⚠️ 已过期"
	} else if user.IsTrafficExceeded() {
		statusStr = "⚠️ 流量超额"
	}

	usedGB := float64(user.UpBytes+user.DownBytes) / (1024 * 1024 * 1024)
	totalStr := "无限制"
	if user.TotalBytes > 0 {
		totalStr = fmt.Sprintf("%.2f GB", float64(user.TotalBytes)/(1024*1024*1024))
	}

	expireStr := "永不过期"
	if user.ExpireTime > 0 {
		expireStr = time.UnixMilli(user.ExpireTime).Format("2006-01-02 15:04:05")
	}

	resetStr := "不重置"
	if user.ResetDay > 0 {
		resetStr = fmt.Sprintf("每月 %d 号", user.ResetDay)
	}

	text := fmt.Sprintf(
		"👤 <b>用户信息卡片</b>\n\n"+
			"📧 <b>邮箱:</b> <code>%s</code>\n"+
			"🔑 <b>UUID:</b> <code>%s</code>\n"+
			"🚦 <b>状态:</b> %s\n"+
			"📊 <b>流量配额:</b> %.2f GB / %s\n"+
			"⏳ <b>到期时间:</b> %s\n"+
			"🔄 <b>重置周期:</b> %s",
		user.Email, user.UUID, statusStr, usedGB, totalStr, expireStr, resetStr,
	)

	toggleText := "⛔ 禁用用户"
	if !user.Enabled {
		toggleText = "✅ 启用用户"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(toggleText, fmt.Sprintf("user:toggle:%d", user.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🔄 重置流量", fmt.Sprintf("user:reset:%d", user.ID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 获取订阅", fmt.Sprintf("user:sub:%d", user.ID)),
			tgbotapi.NewInlineKeyboardButtonData("🔄 刷新状态", fmt.Sprintf("user:refresh:%d", user.ID)),
		),
	)

	return text, keyboard
}
