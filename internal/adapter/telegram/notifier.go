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
	mu         sync.RWMutex
	botToken   string
	adminChatID int64
	bot        *tgbotapi.BotAPI
	running    bool
	stopChan   chan struct{}
}

func NewBotAdapter(botToken string, adminChatID int64) *BotAdapter {
	return &BotAdapter{
		botToken:    botToken,
		adminChatID: adminChatID,
		stopChan:    make(chan struct{}),
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
	defer b.mu.Unlock()

	b.botToken = botToken
	b.adminChatID = adminChatID

	if botToken == "" {
		b.bot = nil
		return nil
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		return err
	}
	b.bot = bot
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
		"🚨 <b>系统高负载告警</b>\n\n"+
			"📈 <b>指标:</b> %s\n"+
			"⚡ <b>当前值:</b> %.1f%%\n"+
			"🛑 <b>阈值:</b> %.1f%%\n"+
			"📝 <b>描述:</b> %s",
		alert.Metric, alert.CurrentVal, alert.Threshold, alert.Description,
	)
	return b.SendMessage(ctx, text)
}

func (b *BotAdapter) SendServiceStatusAlert(ctx context.Context, status domain.ServiceStatus) error {
	stateIcon := "🟢"
	if !status.Active {
		stateIcon = "🔴"
	}
	text := fmt.Sprintf(
		"%s <b>Xray 服务状态变更</b>\n\n"+
			"<b>状态:</b> %s\n"+
			"<b>子状态:</b> %s",
		stateIcon,
		map[bool]string{true: "正常运行中", false: "异常已停止"}[status.Active],
		status.SubState,
	)
	return b.SendMessage(ctx, text)
}
