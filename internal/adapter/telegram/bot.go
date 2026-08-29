package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"panel/internal/domain"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotHandler struct {
	adapter     *BotAdapter
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
	monitor     domain.HostMonitor
	xrayManager domain.XrayManager
	publicURL   string
}

func NewBotHandler(
	adapter *BotAdapter,
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	monitor domain.HostMonitor,
	xrayManager domain.XrayManager,
	publicURL string,
) *BotHandler {
	return &BotHandler{
		adapter:     adapter,
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		monitor:     monitor,
		xrayManager: xrayManager,
		publicURL:   publicURL,
	}
}

func (h *BotHandler) registerBotCommands(bot *tgbotapi.BotAPI) {
	if bot == nil {
		return
	}
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "status", Description: "📊 查看系统与 Xray 运行状态"},
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

func (h *BotHandler) StartPolling(ctx context.Context) {
	go func() {
		for {
			h.adapter.mu.RLock()
			bot := h.adapter.bot
			h.adapter.mu.RUnlock()

			if bot == nil {
				select {
				case <-ctx.Done():
					return
				case <-h.adapter.ReloadChan:
					continue
				case <-time.After(3 * time.Second):
					continue
				}
			}

			// 自动注册云端 Menu 按钮指令列表
			h.registerBotCommands(bot)

			u := tgbotapi.NewUpdate(0)
			u.Timeout = 30
			updates := bot.GetUpdatesChan(u)
			slog.Info("Telegram Bot long polling started", slog.String("username", bot.Self.UserName))

			pollCtx, cancelPoll := context.WithCancel(ctx)

		pollLoop:
			for {
				select {
				case <-pollCtx.Done():
					break pollLoop
				case <-h.adapter.ReloadChan:
					cancelPoll()
					bot.StopReceivingUpdates()
					break pollLoop
				case update, ok := <-updates:
					if !ok {
						cancelPoll()
						break pollLoop
					}
					if update.Message != nil {
						h.handleMessage(ctx, update.Message)
					}
				}
			}
			cancelPoll()
		}
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
			"📈 /traffic - 查看用户流量消耗统计\n" +
			"🔗 /sub [email] - 获取指定用户专属订阅\n" +
			"🔄 /restart - 重启 Xray 核心服务\n" +
			"📱 /menu - 呼出快捷交互键盘\n\n" +
			"<i>可直接点击下方快捷按钮进行操作：</i>"
		h.sendMenuWithKeyboard(msg.Chat.ID, text)

	case "status":
		metrics, err := h.monitor.GetSystemMetrics(ctx)
		if err != nil {
			h.reply(msg.Chat.ID, "❌ 获取系统指标失败: "+err.Error())
			return
		}
		svcStatus, _ := h.xrayManager.GetServiceStatus(ctx)
		xrayVer, _ := h.xrayManager.GetVersion(ctx)

		text := fmt.Sprintf(
			"📊 <b>系统运行状态</b>\n\n"+
				"🖥️ <b>CPU:</b> %.1f%%\n"+
				"🧠 <b>内存:</b> %.2f GB / %.2f GB (%.1f%%)\n"+
				"💾 <b>磁盘:</b> %.2f GB / %.2f GB (%.1f%%)\n"+
				"⚡ <b>网络速率:</b> ⬆️ %.2f MB/s | ⬇️ %.2f MB/s\n"+
				"⏱️ <b>开机时长:</b> %d 小时\n\n"+
				"🚀 <b>Xray:</b> %s (%s)\n"+
				"🏷️ <b>版本:</b> <code>%s</code>",
			metrics.CPUUsagePercent,
			float64(metrics.MemoryUsedBytes)/(1024*1024*1024),
			float64(metrics.MemoryTotalBytes)/(1024*1024*1024),
			metrics.MemoryUsagePct,
			float64(metrics.DiskUsedBytes)/(1024*1024*1024),
			float64(metrics.DiskTotalBytes)/(1024*1024*1024),
			metrics.DiskUsagePct,
			float64(metrics.NetUpSpeedBps)/(1024*1024),
			float64(metrics.NetDownSpeedBps)/(1024*1024),
			metrics.UptimeSeconds/3600,
			map[bool]string{true: "🟢 正常运行", false: "🔴 已停止"}[svcStatus.Active],
			svcStatus.SubState,
			xrayVer,
		)
		h.reply(msg.Chat.ID, text)

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
		time.Sleep(1 * time.Second)
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
