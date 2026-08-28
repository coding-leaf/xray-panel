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

func (h *BotHandler) StartPolling(ctx context.Context) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if bot == nil {
		return
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	slog.Info("Telegram Bot polling started")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				if update.Message == nil || !update.Message.IsCommand() {
					continue
				}

				h.handleCommand(ctx, update.Message)
			}
		}
	}()
}

func (h *BotHandler) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	// 校验管理员权限
	h.adapter.mu.RLock()
	adminChatID := h.adapter.adminChatID
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()

	if adminChatID != 0 && msg.Chat.ID != adminChatID {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "⛔ 无权使用此机器人。")
		_, _ = bot.Send(reply)
		return
	}

	cmd := msg.Command()
	args := strings.TrimSpace(msg.CommandArguments())

	switch cmd {
	case "start", "help":
		text := "🤖 <b>Xray 独立运维管理机器人</b>\n\n" +
			"/status - 查看系统与 Xray 运行状态\n" +
			"/traffic - 查看用户流量消耗统计\n" +
			"/sub [email] - 获取指定用户的订阅分发链接\n" +
			"/restart - 重启 Xray 核心服务\n"
		h.reply(msg.Chat.ID, text)

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
			h.reply(msg.Chat.ID, "用法: <code>/sub 用户邮箱</code>")
			return
		}
		u, err := h.userRepo.GetByEmail(ctx, args)
		if err != nil {
			h.reply(msg.Chat.ID, "❌ 未找到该用户")
			return
		}
		subURL := fmt.Sprintf("%s/api/sub/%s", strings.TrimRight(h.publicURL, "/"), u.SubToken)
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
			h.reply(msg.Chat.ID, "⚠️ 重启完成，但当前服务状态异常: "+status.SubState)
		}
	}
}

func (h *BotHandler) reply(chatID int64, text string) {
	h.adapter.mu.RLock()
	bot := h.adapter.bot
	h.adapter.mu.RUnlock()
	if bot == nil {
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	_, _ = bot.Send(msg)
}
