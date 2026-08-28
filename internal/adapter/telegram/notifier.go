package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"panel/internal/domain"
	"panel/internal/pkg/logger"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotAdapter struct {
	mu          sync.RWMutex
	botToken    string
	adminChatID int64
	bot         *tgbotapi.BotAPI
	ReloadChan  chan struct{}
}

func NewBotAdapter(botToken string, adminChatID int64) *BotAdapter {
	return &BotAdapter{
		botToken:    botToken,
		adminChatID: adminChatID,
		ReloadChan:  make(chan struct{}, 1),
	}
}

func (b *BotAdapter) Init() error {
	if b.botToken == "" {
		return nil
	}
	bot, err := tgbotapi.NewBotAPI(b.botToken)
	if err != nil {
		return fmt.Errorf("init telegram bot failed: %w", err)
	}
	b.bot = bot
	slog.Info("Telegram bot initialized", slog.String("username", bot.Self.UserName))
	return nil
}

func (b *BotAdapter) UpdateConfig(botToken string, adminChatID int64) error {
	b.mu.Lock()
	b.botToken = botToken
	b.adminChatID = adminChatID

	if botToken == "" {
		b.bot = nil
		b.mu.Unlock()
		select {
		case b.ReloadChan <- struct{}{}:
		default:
		}
		return nil
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	b.bot = bot
	b.mu.Unlock()

	select {
	case b.ReloadChan <- struct{}{}:
	default:
	}

	slog.Info("Telegram bot config updated and reloaded", slog.String("username", bot.Self.UserName))
	return nil
}

func (b *BotAdapter) SendMessage(ctx context.Context, text string) error {
	b.mu.RLock()
	bot := b.bot
	chatID := b.adminChatID
	b.mu.RUnlock()

	if bot == nil || chatID == 0 {
		return nil
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to send telegram message", slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (b *BotAdapter) SendTrafficAlert(ctx context.Context, alert domain.TrafficAlert) error {
	text := fmt.Sprintf(
		"⚠️ <b>用户流量预警</b>\n\n"+
			"👤 <b>用户:</b> <code>%s</code>\n"+
			"📊 <b>已用:</b> %.2f GB / %.2f GB (%.1f%%)\n"+
			"⏰ <b>时间:</b> 实时告警",
		alert.Email,
		float64(alert.UsedBytes)/(1024*1024*1024),
		float64(alert.TotalBytes)/(1024*1024*1024),
		alert.UsageRatio*100,
	)
	return b.SendMessage(ctx, text)
}

func (b *BotAdapter) SendSystemAlert(ctx context.Context, alert domain.SystemAlert) error {
	text := fmt.Sprintf(
		"🚨 <b>系统负载告警</b>\n\n"+
			"🏷️ <b>指标:</b> %s\n"+
			"📈 <b>当前值:</b> %.1f%% (阈值: %.1f%%)\n"+
			"📝 <b>描述:</b> %s\n"+
			"⏰ <b>时间:</b> 实时告警",
		alert.Metric,
		alert.CurrentVal,
		alert.Threshold,
		alert.Description,
	)
	return b.SendMessage(ctx, text)
}

func (b *BotAdapter) SendServiceStatusAlert(ctx context.Context, status domain.ServiceStatus) error {
	state := "🟢 运行中"
	if !status.Active {
		state = "🔴 已停止/异常"
	}
	text := fmt.Sprintf(
		"🔔 <b>Xray 核心服务状态变更</b>\n\n"+
			"📊 <b>状态:</b> %s (%s)\n"+
			"🆔 <b>PID:</b> %d\n"+
			"⏱️ <b>运行时间:</b> %s\n"+
			"⏰ <b>时间:</b> 实时通知",
		state,
		status.SubState,
		status.PID,
		status.Uptime,
	)
	return b.SendMessage(ctx, text)
}

func (b *BotAdapter) SendCertAlert(ctx context.Context, alert domain.CertAlert) error {
	text := fmt.Sprintf(
		"🔐 <b>TLS 证书过期提醒</b>\n\n"+
			"📁 <b>域名/证书:</b> <code>%s</code>\n"+
			"⏳ <b>剩余天数:</b> %d 天\n"+
			"⏰ <b>到期时间:</b> %s\n"+
			"📄 <b>路径:</b> <code>%s</code>",
		alert.DomainName,
		alert.DaysLeft,
		alert.NotAfter,
		alert.Path,
	)
	return b.SendMessage(ctx, text)
}
