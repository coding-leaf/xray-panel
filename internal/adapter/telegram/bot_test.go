package telegram_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"panel/internal/adapter/telegram"
	panelDomain "panel/internal/domain"
	"panel/internal/service"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestBotHandler_LifecycleNilBot(t *testing.T) {
	adapter := telegram.NewBotAdapter("", 0)
	_ = adapter.Init()

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, nil, "http://localhost")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- handler.Start(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BotHandler failed to stop within timeout")
	}
}

func TestBotHandler_StartPollingCompatibility(t *testing.T) {
	adapter := telegram.NewBotAdapter("", 0)
	_ = adapter.Init()

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, nil, "http://localhost")

	ctx, cancel := context.WithCancel(context.Background())
	handler.StartPolling(ctx)

	time.Sleep(20 * time.Millisecond)
	cancel()
	// Just verify no panic/hang
	time.Sleep(50 * time.Millisecond)
}

func TestBotHandler_LifecycleActiveLongPolling(t *testing.T) {
	longPollStarted := make(chan struct{}, 1)
	requestCanceled := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
			return
		}
		if strings.Contains(r.URL.Path, "getMyCommands") || strings.Contains(r.URL.Path, "setMyCommands") {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		if strings.Contains(r.URL.Path, "getUpdates") {
			_, _ = io.ReadAll(r.Body)
			select {
			case longPollStarted <- struct{}{}:
			default:
			}
			// Simulate long-poll waiting for updates or client cancellation
			select {
			case <-r.Context().Done():
				select {
				case requestCanceled <- struct{}{}:
				default:
				}
				return
			case <-time.After(10 * time.Second):
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()

	endpoint := server.URL + "/bot%s/%s"
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11", endpoint)
	if err != nil {
		t.Fatalf("failed to init mock bot: %v", err)
	}

	adapter := telegram.NewBotAdapter("", 0)
	adapter.SetBotForTest(bot)

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, nil, "http://localhost")

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- handler.Start(ctx)
	}()

	// Wait for long polling to actually connect to the server
	select {
	case <-longPollStarted:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("long polling did not start within timeout")
	}

	// Now cancel context - this should immediately cancel the HTTP request and exit Start
	cancelStart := time.Now()
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on cancel, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("handler.Start did not stop within 1 second; long-polling was not interrupted!")
	}

	elapsed := time.Since(cancelStart)
	if elapsed > 800*time.Millisecond {
		t.Errorf("expected instant cancellation, took %v", elapsed)
	}

	// Verify server observed client cancellation
	select {
	case <-requestCanceled:
		// Success: HTTP request was cleanly canceled at transport layer
	case <-time.After(500 * time.Millisecond):
		t.Error("HTTP server did not observe client request cancellation")
	}
}

func TestParseAddUserCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        string
		wantEmail   string
		wantBytes   int64
		wantExpire  int
		wantErr     bool
	}{
		{
			name:       "valid email only (defaults)",
			args:       "user@example.com",
			wantEmail:  "user@example.com",
			wantBytes:  0,
			wantExpire: 0,
			wantErr:    false,
		},
		{
			name:       "valid email with GB",
			args:       "user@example.com 10",
			wantEmail:  "user@example.com",
			wantBytes:  10 * 1024 * 1024 * 1024,
			wantExpire: 0,
			wantErr:    false,
		},
		{
			name:       "valid email with GB and days",
			args:       "user@example.com 50 30",
			wantEmail:  "user@example.com",
			wantBytes:  50 * 1024 * 1024 * 1024,
			wantExpire: 30,
			wantErr:    false,
		},
		{
			name:       "valid with extra spaces",
			args:       "   admin@test.org    20    60   ",
			wantEmail:  "admin@test.org",
			wantBytes:  20 * 1024 * 1024 * 1024,
			wantExpire: 60,
			wantErr:    false,
		},
		{
			name:    "empty args",
			args:    "",
			wantErr: true,
		},
		{
			name:    "spaces only",
			args:    "    ",
			wantErr: true,
		},
		{
			name:    "invalid email format without at",
			args:    "not-an-email 10 30",
			wantErr: true,
		},
		{
			name:    "invalid email format no domain",
			args:    "user@ 10 30",
			wantErr: true,
		},
		{
			name:    "non-numeric GB",
			args:    "user@example.com abc 30",
			wantErr: true,
		},
		{
			name:    "negative GB",
			args:    "user@example.com -10 30",
			wantErr: true,
		},
		{
			name:    "non-numeric days",
			args:    "user@example.com 10 xyz",
			wantErr: true,
		},
		{
			name:    "negative days",
			args:    "user@example.com 10 -5",
			wantErr: true,
		},
		{
			name:    "too many arguments",
			args:    "user@example.com 10 30 extra",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, totalBytes, expireDays, err := telegram.ParseAddUserCommand(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseAddUserCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if email != tt.wantEmail {
					t.Errorf("email = %v, want %v", email, tt.wantEmail)
				}
				if totalBytes != tt.wantBytes {
					t.Errorf("totalBytes = %v, want %v", totalBytes, tt.wantBytes)
				}
				if expireDays != tt.wantExpire {
					t.Errorf("expireDays = %v, want %v", expireDays, tt.wantExpire)
				}
			}
		})
	}
}

func TestParseCallbackData(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantDomain string
		wantAction string
		wantTarget uint
		wantErr    bool
	}{
		{
			name:       "status:refresh",
			data:       "status:refresh",
			wantDomain: "status",
			wantAction: "refresh",
			wantTarget: 0,
			wantErr:    false,
		},
		{
			name:       "status:restart",
			data:       "status:restart",
			wantDomain: "status",
			wantAction: "restart",
			wantTarget: 0,
			wantErr:    false,
		},
		{
			name:       "user:toggle:12",
			data:       "user:toggle:12",
			wantDomain: "user",
			wantAction: "toggle",
			wantTarget: 12,
			wantErr:    false,
		},
		{
			name:       "user:reset:99",
			data:       "user:reset:99",
			wantDomain: "user",
			wantAction: "reset",
			wantTarget: 99,
			wantErr:    false,
		},
		{
			name:       "user:sub:1",
			data:       "user:sub:1",
			wantDomain: "user",
			wantAction: "sub",
			wantTarget: 1,
			wantErr:    false,
		},
		{
			name:       "user:refresh:42",
			data:       "user:refresh:42",
			wantDomain: "user",
			wantAction: "refresh",
			wantTarget: 42,
			wantErr:    false,
		},
		{
			name:    "empty string",
			data:    "",
			wantErr: true,
		},
		{
			name:    "single segment",
			data:    "status",
			wantErr: true,
		},
		{
			name:    "empty domain",
			data:    ":refresh",
			wantErr: true,
		},
		{
			name:    "empty action",
			data:    "status:",
			wantErr: true,
		},
		{
			name:    "four segments",
			data:    "user:toggle:12:extra",
			wantErr: true,
		},
		{
			name:    "user action missing target id",
			data:    "user:toggle",
			wantErr: true,
		},
		{
			name:    "user action non-numeric target id",
			data:    "user:toggle:abc",
			wantErr: true,
		},
		{
			name:    "user action negative target id",
			data:    "user:toggle:-5",
			wantErr: true,
		},
		{
			name:    "user action zero target id",
			data:    "user:toggle:0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, a, id, err := telegram.ParseCallbackData(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseCallbackData() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if d != tt.wantDomain {
					t.Errorf("domain = %v, want %v", d, tt.wantDomain)
				}
				if a != tt.wantAction {
					t.Errorf("action = %v, want %v", a, tt.wantAction)
				}
				if id != tt.wantTarget {
					t.Errorf("targetID = %v, want %v", id, tt.wantTarget)
				}
			}
		})
	}
}

func TestRenderStatusCard(t *testing.T) {
	metrics := &panelDomain.SystemMetrics{
		CPUUsagePercent: 15.5,
		MemoryUsedBytes:  4 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 8 * 1024 * 1024 * 1024,
		MemoryUsagePct:   50.0,
		DiskUsedBytes:    20 * 1024 * 1024 * 1024,
		DiskTotalBytes:   100 * 1024 * 1024 * 1024,
		DiskUsagePct:     20.0,
		NetUpSpeedBps:    1024 * 1024 * 2.5,
		NetDownSpeedBps:  1024 * 1024 * 10.5,
		UptimeSeconds:    7200,
	}
	svcStatus := panelDomain.ServiceStatus{
		Active:   true,
		SubState: "running",
	}
	updateTime := time.Date(2026, 9, 20, 18, 0, 0, 0, time.Local)
	xrayVer := "1.8.24"

	text, keyboard := telegram.RenderStatusCard(metrics, svcStatus, xrayVer, updateTime)

	if !strings.Contains(text, "15.5%") {
		t.Errorf("expected text to contain CPU usage, got: %s", text)
	}
	if !strings.Contains(text, "🟢 正常运行") {
		t.Errorf("expected text to contain active status, got: %s", text)
	}
	if !strings.Contains(text, "1.8.24") {
		t.Errorf("expected text to contain version, got: %s", text)
	}
	if !strings.Contains(text, "2026-09-20 18:00:00") {
		t.Errorf("expected text to contain formatted time, got: %s", text)
	}

	// Verify keyboard layout
	if len(keyboard.InlineKeyboard) != 1 {
		t.Fatalf("expected 1 row of inline keyboard, got %d", len(keyboard.InlineKeyboard))
	}
	row := keyboard.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("expected 2 buttons in first row, got %d", len(row))
	}
	if *row[0].CallbackData != "status:refresh" {
		t.Errorf("expected button 0 callback data 'status:refresh', got %s", *row[0].CallbackData)
	}
	if *row[1].CallbackData != "status:restart" {
		t.Errorf("expected button 1 callback data 'status:restart', got %s", *row[1].CallbackData)
	}
}

func TestRenderUserCard(t *testing.T) {
	now := time.Now()

	t.Run("active user with limit and expire", func(t *testing.T) {
		user := &panelDomain.User{
			ID:         42,
			Email:      "active@example.com",
			UUID:       "550e8400-e29b-41d4-a716-446655440000",
			Enabled:    true,
			TotalBytes: 100 * 1024 * 1024 * 1024,
			UpBytes:    10 * 1024 * 1024 * 1024,
			DownBytes:  20 * 1024 * 1024 * 1024,
			ExpireTime: now.Add(48 * time.Hour).UnixMilli(),
			ResetDay:   1,
		}

		text, keyboard := telegram.RenderUserCard(user, "https://sub.example.com")
		if !strings.Contains(text, "active@example.com") {
			t.Errorf("missing email in text: %s", text)
		}
		if !strings.Contains(text, "🟢 正常") {
			t.Errorf("expected status '🟢 正常', got: %s", text)
		}
		if !strings.Contains(text, "30.00 GB / 100.00 GB") {
			t.Errorf("expected traffic format, got: %s", text)
		}
		if !strings.Contains(text, "每月 1 号") {
			t.Errorf("expected reset day info, got: %s", text)
		}

		// Verify 2 rows of buttons
		if len(keyboard.InlineKeyboard) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(keyboard.InlineKeyboard))
		}
		// Row 1: toggle (Disable) & reset
		if !strings.Contains(keyboard.InlineKeyboard[0][0].Text, "禁用") {
			t.Errorf("expected '禁用' button for active user, got: %s", keyboard.InlineKeyboard[0][0].Text)
		}
		if *keyboard.InlineKeyboard[0][0].CallbackData != "user:toggle:42" {
			t.Errorf("wrong callback data: %s", *keyboard.InlineKeyboard[0][0].CallbackData)
		}
		if *keyboard.InlineKeyboard[0][1].CallbackData != "user:reset:42" {
			t.Errorf("wrong callback data: %s", *keyboard.InlineKeyboard[0][1].CallbackData)
		}
		// Row 2: sub & refresh
		if *keyboard.InlineKeyboard[1][0].CallbackData != "user:sub:42" {
			t.Errorf("wrong callback data: %s", *keyboard.InlineKeyboard[1][0].CallbackData)
		}
		if *keyboard.InlineKeyboard[1][1].CallbackData != "user:refresh:42" {
			t.Errorf("wrong callback data: %s", *keyboard.InlineKeyboard[1][1].CallbackData)
		}
	})

	t.Run("disabled user", func(t *testing.T) {
		user := &panelDomain.User{
			ID:      7,
			Email:   "disabled@example.com",
			Enabled: false,
		}
		text, keyboard := telegram.RenderUserCard(user, "https://sub.example.com")
		if !strings.Contains(text, "🔴 已禁用") {
			t.Errorf("expected status '🔴 已禁用', got: %s", text)
		}
		if !strings.Contains(keyboard.InlineKeyboard[0][0].Text, "启用") {
			t.Errorf("expected '启用' button for disabled user, got: %s", keyboard.InlineKeyboard[0][0].Text)
		}
		if *keyboard.InlineKeyboard[0][0].CallbackData != "user:toggle:7" {
			t.Errorf("wrong callback data: %s", *keyboard.InlineKeyboard[0][0].CallbackData)
		}
	})

	t.Run("traffic exceeded user", func(t *testing.T) {
		user := &panelDomain.User{
			ID:         10,
			Email:      "exceeded@example.com",
			Enabled:    true,
			TotalBytes: 50 * 1024 * 1024 * 1024,
			UpBytes:    30 * 1024 * 1024 * 1024,
			DownBytes:  25 * 1024 * 1024 * 1024,
		}
		text, _ := telegram.RenderUserCard(user, "https://sub.example.com")
		if !strings.Contains(text, "⚠️ 流量超额") {
			t.Errorf("expected status '⚠️ 流量超额', got: %s", text)
		}
	})

	t.Run("expired user", func(t *testing.T) {
		user := &panelDomain.User{
			ID:         11,
			Email:      "expired@example.com",
			Enabled:    true,
			ExpireTime: now.Add(-1 * time.Hour).UnixMilli(),
		}
		text, _ := telegram.RenderUserCard(user, "https://sub.example.com")
		if !strings.Contains(text, "⚠️ 已过期") {
			t.Errorf("expected status '⚠️ 已过期', got: %s", text)
		}
	})

	t.Run("unlimited and never expire", func(t *testing.T) {
		user := &panelDomain.User{
			ID:         12,
			Email:      "unlimited@example.com",
			Enabled:    true,
			TotalBytes: 0,
			ExpireTime: 0,
			ResetDay:   0,
		}
		text, _ := telegram.RenderUserCard(user, "https://sub.example.com")
		if !strings.Contains(text, "无限制") {
			t.Errorf("expected '无限制', got: %s", text)
		}
		if !strings.Contains(text, "永不过期") {
			t.Errorf("expected '永不过期', got: %s", text)
		}
		if !strings.Contains(text, "不重置") {
			t.Errorf("expected '不重置', got: %s", text)
		}
	})
}

// ============================================================================
// M3: Telegram BotAPI Mock HTTP 测试基础设施与交互回归测试
// ============================================================================

type capturedRequest struct {
	Endpoint string
	Params   map[string]string
}

type tgUpdatesResponse struct {
	Ok     bool              `json:"ok"`
	Result []tgbotapi.Update `json:"result"`
}

type mockTelegramServer struct {
	server      *httptest.Server
	updateCh    chan tgbotapi.Update
	sentMsgs    chan capturedRequest
	editMsgs    chan capturedRequest
	answeredCbs chan capturedRequest
	updateSeq   int64
}

func newMockTelegramServer(t *testing.T) *mockTelegramServer {
	t.Helper()
	mts := &mockTelegramServer{
		updateCh:    make(chan tgbotapi.Update, 100),
		sentMsgs:    make(chan capturedRequest, 100),
		editMsgs:    make(chan capturedRequest, 100),
		answeredCbs: make(chan capturedRequest, 100),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`))
			return
		}
		if strings.Contains(r.URL.Path, "getMyCommands") || strings.Contains(r.URL.Path, "setMyCommands") {
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}
		if strings.Contains(r.URL.Path, "getUpdates") {
			select {
			case up := <-mts.updateCh:
				resp := tgUpdatesResponse{
					Ok:     true,
					Result: []tgbotapi.Update{up},
				}
				data, _ := json.Marshal(resp)
				_, _ = w.Write(data)
				return
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
				_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
				return
			}
		}

		params := make(map[string]string)
		if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			_ = r.ParseMultipartForm(10 << 20)
		} else {
			_ = r.ParseForm()
		}
		for k, v := range r.Form {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}

		if strings.Contains(r.URL.Path, "sendMessage") {
			mts.sentMsgs <- capturedRequest{Endpoint: "sendMessage", Params: params}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1001,"date":1600000000,"chat":{"id":123}}}`))
			return
		}
		if strings.Contains(r.URL.Path, "editMessageText") {
			mts.editMsgs <- capturedRequest{Endpoint: "editMessageText", Params: params}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1001,"date":1600000000,"chat":{"id":123}}}`))
			return
		}
		if strings.Contains(r.URL.Path, "answerCallbackQuery") {
			mts.answeredCbs <- capturedRequest{Endpoint: "answerCallbackQuery", Params: params}
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
			return
		}

		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})

	mts.server = httptest.NewServer(handler)
	return mts
}

func (s *mockTelegramServer) SendMessageUpdate(chatID int64, text string) {
	seq := atomic.AddInt64(&s.updateSeq, 1)
	words := strings.Split(text, " ")
	cmdLen := 0
	if strings.HasPrefix(text, "/") {
		cmdLen = len(words[0])
	}

	var entities []tgbotapi.MessageEntity
	if cmdLen > 0 {
		entities = append(entities, tgbotapi.MessageEntity{
			Type:   "bot_command",
			Offset: 0,
			Length: cmdLen,
		})
	}

	s.updateCh <- tgbotapi.Update{
		UpdateID: int(seq),
		Message: &tgbotapi.Message{
			MessageID: int(seq * 10),
			Chat:      &tgbotapi.Chat{ID: chatID},
			From:      &tgbotapi.User{ID: chatID, UserName: "tester"},
			Text:      text,
			Entities:  entities,
		},
	}
}

func (s *mockTelegramServer) SendCallbackUpdate(fromID int64, chatID int64, msgID int, data string) {
	seq := atomic.AddInt64(&s.updateSeq, 1)
	s.updateCh <- tgbotapi.Update{
		UpdateID: int(seq),
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   fmt.Sprintf("cb-%d", seq),
			From: &tgbotapi.User{ID: fromID, UserName: "tester"},
			Data: data,
			Message: &tgbotapi.Message{
				MessageID: msgID,
				Chat:      &tgbotapi.Chat{ID: chatID},
			},
		},
	}
}

func (s *mockTelegramServer) WaitSentMsg(t *testing.T, timeout time.Duration) capturedRequest {
	t.Helper()
	select {
	case req := <-s.sentMsgs:
		return req
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for sendMessage")
		return capturedRequest{}
	}
}

func (s *mockTelegramServer) WaitEditMsg(t *testing.T, timeout time.Duration) capturedRequest {
	t.Helper()
	select {
	case req := <-s.editMsgs:
		return req
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for editMessageText")
		return capturedRequest{}
	}
}

func (s *mockTelegramServer) WaitAnswerCb(t *testing.T, timeout time.Duration) capturedRequest {
	t.Helper()
	select {
	case req := <-s.answeredCbs:
		return req
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for answerCallbackQuery")
		return capturedRequest{}
	}
}

type mockHostMonitor struct {
	metrics *panelDomain.SystemMetrics
	err     error
}

func (m *mockHostMonitor) GetSystemMetrics(ctx context.Context) (*panelDomain.SystemMetrics, error) {
	return m.metrics, m.err
}

func (m *mockHostMonitor) GetNetworkSpeed(ctx context.Context) (uint64, uint64, error) {
	return 1024, 2048, nil
}

type mockXrayManager struct {
	mu           sync.Mutex
	status       panelDomain.ServiceStatus
	version      string
	restartFunc  func(ctx context.Context) error
	restartCalls int
}

func (m *mockXrayManager) GetServiceStatus(ctx context.Context) (panelDomain.ServiceStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, nil
}

func (m *mockXrayManager) GetVersion(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.version, nil
}

func (m *mockXrayManager) RestartService(ctx context.Context) error {
	m.mu.Lock()
	m.restartCalls++
	fn := m.restartFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return nil
}

type mockUserService struct {
	mu          sync.Mutex
	users       map[uint]*panelDomain.User
	createCalls []service.CreateUserDTO
	updateCalls []panelDomain.UpdateUserDTO
	resetCalls  []uint
}

func (m *mockUserService) CreateUser(ctx context.Context, dto service.CreateUserDTO) (*panelDomain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls = append(m.createCalls, dto)
	u := &panelDomain.User{
		ID:         uint(len(m.users) + 100),
		Email:      dto.Email,
		UUID:       "mock-uuid-1234",
		SubToken:   "sub-token-" + dto.Email,
		TotalBytes: dto.TotalBytes,
		Enabled:    true,
	}
	if dto.ExpireDays > 0 {
		u.ExpireTime = time.Now().AddDate(0, 0, dto.ExpireDays).UnixMilli()
	}
	m.users[u.ID] = u
	return u, nil
}

func (m *mockUserService) UpdateUser(ctx context.Context, id uint, dto panelDomain.UpdateUserDTO) (*panelDomain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls = append(m.updateCalls, dto)
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	if dto.Enabled != nil {
		u.Enabled = *dto.Enabled
	}
	return u, nil
}

func (m *mockUserService) ResetTraffic(ctx context.Context, id uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCalls = append(m.resetCalls, id)
	u, ok := m.users[id]
	if !ok {
		return fmt.Errorf("user not found")
	}
	u.UpBytes = 0
	u.DownBytes = 0
	return nil
}

func (m *mockUserService) GetByID(ctx context.Context, id uint) (*panelDomain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	cp := *u
	return &cp, nil
}

func (m *mockUserService) GetByEmail(ctx context.Context, email string) (*panelDomain.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("user not found")
}

type mockInboundRepo struct {
	inbounds []panelDomain.Inbound
}

func (m *mockInboundRepo) Create(ctx context.Context, inbound *panelDomain.Inbound) error { return nil }
func (m *mockInboundRepo) Update(ctx context.Context, inbound *panelDomain.Inbound) error { return nil }
func (m *mockInboundRepo) Delete(ctx context.Context, id uint) error                      { return nil }
func (m *mockInboundRepo) GetByID(ctx context.Context, id uint) (*panelDomain.Inbound, error) {
	return nil, nil
}
func (m *mockInboundRepo) GetByTag(ctx context.Context, tag string) (*panelDomain.Inbound, error) {
	return nil, nil
}
func (m *mockInboundRepo) ListAll(ctx context.Context) ([]panelDomain.Inbound, error) {
	return m.inbounds, nil
}
func (m *mockInboundRepo) AddTraffic(ctx context.Context, tag string, upBytes, downBytes int64) error {
	return nil
}

type testEnv struct {
	server      *mockTelegramServer
	bot         *tgbotapi.BotAPI
	adapter     *telegram.BotAdapter
	handler     *telegram.BotHandler
	userSvc     *mockUserService
	xrayManager *mockXrayManager
	monitor     *mockHostMonitor
	inboundRepo *mockInboundRepo
	adminChatID int64
	cancel      context.CancelFunc
	errCh       chan error
}

func setupTestEnv(t *testing.T, adminChatID int64) *testEnv {
	t.Helper()
	server := newMockTelegramServer(t)

	endpoint := server.server.URL + "/bot%s/%s"
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11", endpoint)
	if err != nil {
		t.Fatalf("failed to init mock bot: %v", err)
	}

	adapter := telegram.NewBotAdapter("", adminChatID)
	adapter.SetBotForTest(bot)

	userSvc := &mockUserService{
		users: make(map[uint]*panelDomain.User),
	}
	xrayManager := &mockXrayManager{
		status:  panelDomain.ServiceStatus{Active: true, SubState: "running"},
		version: "1.8.24",
	}
	monitor := &mockHostMonitor{
		metrics: &panelDomain.SystemMetrics{
			CPUUsagePercent:  12.5,
			MemoryUsedBytes:  1024 * 1024 * 1024,
			MemoryTotalBytes: 4 * 1024 * 1024 * 1024,
			MemoryUsagePct:   25.0,
			DiskUsedBytes:    20 * 1024 * 1024 * 1024,
			DiskTotalBytes:   100 * 1024 * 1024 * 1024,
			DiskUsagePct:     20.0,
			UptimeSeconds:    3600 * 48,
		},
	}
	inboundRepo := &mockInboundRepo{
		inbounds: []panelDomain.Inbound{
			{ID: 1, Tag: "vless-in", Enabled: true},
			{ID: 2, Tag: "vmess-in", Enabled: false},
		},
	}

	handler := telegram.NewBotHandler(
		adapter,
		nil,
		inboundRepo,
		monitor,
		xrayManager,
		userSvc,
		"https://panel.example.com",
	)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- handler.Start(ctx)
	}()

	env := &testEnv{
		server:      server,
		bot:         bot,
		adapter:     adapter,
		handler:     handler,
		userSvc:     userSvc,
		xrayManager: xrayManager,
		monitor:     monitor,
		inboundRepo: inboundRepo,
		adminChatID: adminChatID,
		cancel:      cancel,
		errCh:       errCh,
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Log("handler.Start wait timeout on cleanup")
		}
		server.server.Close()
	})

	return env
}

func TestBotHandler_NonAdminInterception(t *testing.T) {
	adminID := int64(88888)
	env := setupTestEnv(t, adminID)

	t.Run("non-admin message interception", func(t *testing.T) {
		env.server.SendMessageUpdate(99999, "/status")
		req := env.server.WaitSentMsg(t, 2*time.Second)
		if req.Params["chat_id"] != "99999" {
			t.Errorf("expected chat_id 99999, got: %s", req.Params["chat_id"])
		}
		if !strings.Contains(req.Params["text"], "无权使用此机器人") {
			t.Errorf("expected rejection message, got: %s", req.Params["text"])
		}
	})

	t.Run("non-admin callback query interception", func(t *testing.T) {
		env.server.SendCallbackUpdate(99999, 99999, 101, "status:refresh")
		req := env.server.WaitAnswerCb(t, 2*time.Second)
		if !strings.Contains(req.Params["text"], "无权操作") {
			t.Errorf("expected '无权操作' alert, got: %s", req.Params["text"])
		}
		if req.Params["show_alert"] != "true" {
			t.Errorf("expected show_alert to be true, got: %s", req.Params["show_alert"])
		}
		// 确认未触发任何编辑消息行为
		select {
		case edit := <-env.server.editMsgs:
			t.Fatalf("unexpected editMessageText for non-admin callback: %v", edit)
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("unbound admin chat ID prompt", func(t *testing.T) {
		unboundEnv := setupTestEnv(t, 0)
		unboundEnv.server.SendMessageUpdate(12345, "/start")
		req := unboundEnv.server.WaitSentMsg(t, 2*time.Second)
		if !strings.Contains(req.Params["text"], "12345") || !strings.Contains(req.Params["text"], "系统设置") {
			t.Errorf("expected prompt with chat ID for binding, got: %s", req.Params["text"])
		}

		unboundEnv.server.SendCallbackUpdate(12345, 12345, 101, "status:refresh")
		cbReq := unboundEnv.server.WaitAnswerCb(t, 2*time.Second)
		if !strings.Contains(cbReq.Params["text"], "尚未绑定管理员 Chat ID") {
			t.Errorf("expected unbound alert callback, got: %s", cbReq.Params["text"])
		}
	})
}

func TestBotHandler_StatusAndCallbacks(t *testing.T) {
	adminID := int64(88888)
	env := setupTestEnv(t, adminID)

	t.Run("status command send message", func(t *testing.T) {
		env.server.SendMessageUpdate(adminID, "/status")
		sentReq := env.server.WaitSentMsg(t, 2*time.Second)
		if !strings.Contains(sentReq.Params["text"], "🟢 正常运行") {
			t.Errorf("expected running status in card, got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["text"], "1.8.24") {
			t.Errorf("expected version 1.8.24 in card, got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["reply_markup"], "status:refresh") {
			t.Errorf("expected status:refresh button in keyboard, got: %s", sentReq.Params["reply_markup"])
		}
		if !strings.Contains(sentReq.Params["reply_markup"], "status:restart") {
			t.Errorf("expected status:restart button in keyboard, got: %s", sentReq.Params["reply_markup"])
		}
	})

	t.Run("status:refresh in-place edit callback", func(t *testing.T) {
		env.monitor.metrics.CPUUsagePercent = 45.2
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "status:refresh")

		ansReq := env.server.WaitAnswerCb(t, 2*time.Second)
		if !strings.Contains(ansReq.Params["text"], "状态指标已刷新") {
			t.Errorf("expected toast '状态指标已刷新', got: %s", ansReq.Params["text"])
		}

		editReq := env.server.WaitEditMsg(t, 2*time.Second)
		if editReq.Params["message_id"] != "1001" {
			t.Errorf("expected edit message_id 1001, got: %s", editReq.Params["message_id"])
		}
		if !strings.Contains(editReq.Params["text"], "45.2%") {
			t.Errorf("expected updated CPU in edited card, got: %s", editReq.Params["text"])
		}
	})

	t.Run("status:restart callback", func(t *testing.T) {
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "status:restart")

		ansRestartReq := env.server.WaitAnswerCb(t, 2*time.Second)
		if !strings.Contains(ansRestartReq.Params["text"], "正在重启") {
			t.Errorf("expected toast '正在重启', got: %s", ansRestartReq.Params["text"])
		}

		editRestartReq := env.server.WaitEditMsg(t, 3*time.Second)
		if editRestartReq.Params["message_id"] != "1001" {
			t.Errorf("expected edit message_id 1001, got: %s", editRestartReq.Params["message_id"])
		}
		if env.xrayManager.restartCalls == 0 {
			t.Errorf("expected RestartService to have been called")
		}
	})
}

func TestBotHandler_UserAndCallbacks(t *testing.T) {
	adminID := int64(88888)
	env := setupTestEnv(t, adminID)

	user := &panelDomain.User{
		ID:         42,
		Email:      "alice@example.com",
		UUID:       "uuid-alice-42",
		SubToken:   "token-alice-42",
		TotalBytes: 50 * 1024 * 1024 * 1024,
		UpBytes:    10 * 1024 * 1024 * 1024,
		DownBytes:  5 * 1024 * 1024 * 1024,
		Enabled:    true,
	}
	env.userSvc.users[42] = user

	t.Run("user query command", func(t *testing.T) {
		env.server.SendMessageUpdate(adminID, "/user alice@example.com")
		sentReq := env.server.WaitSentMsg(t, 2*time.Second)
		if !strings.Contains(sentReq.Params["text"], "alice@example.com") {
			t.Errorf("expected user card to contain alice@example.com, got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["text"], "🟢 正常") {
			t.Errorf("expected status '🟢 正常', got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["reply_markup"], "user:toggle:42") {
			t.Errorf("expected toggle button for user 42, got: %s", sentReq.Params["reply_markup"])
		}
	})

	t.Run("user:toggle disable and re-enable", func(t *testing.T) {
		// Toggle to disable
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "user:toggle:42")
		editToggleReq := env.server.WaitEditMsg(t, 2*time.Second)
		ansToggleReq := env.server.WaitAnswerCb(t, 2*time.Second)

		if !strings.Contains(ansToggleReq.Params["text"], "已禁用") {
			t.Errorf("expected toast '已禁用', got: %s", ansToggleReq.Params["text"])
		}
		if !strings.Contains(editToggleReq.Params["text"], "🔴 已禁用") {
			t.Errorf("expected edited text to show '🔴 已禁用', got: %s", editToggleReq.Params["text"])
		}
		if !strings.Contains(editToggleReq.Params["reply_markup"], "启用用户") {
			t.Errorf("expected toggle button to flip to '启用用户', got: %s", editToggleReq.Params["reply_markup"])
		}

		// Toggle back to enable
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "user:toggle:42")
		editEnableReq := env.server.WaitEditMsg(t, 2*time.Second)
		ansEnableReq := env.server.WaitAnswerCb(t, 2*time.Second)

		if !strings.Contains(ansEnableReq.Params["text"], "已启用") {
			t.Errorf("expected toast '已启用', got: %s", ansEnableReq.Params["text"])
		}
		if !strings.Contains(editEnableReq.Params["text"], "🟢 正常") {
			t.Errorf("expected edited text to show '🟢 正常', got: %s", editEnableReq.Params["text"])
		}
	})

	t.Run("user:reset traffic", func(t *testing.T) {
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "user:reset:42")
		editResetReq := env.server.WaitEditMsg(t, 2*time.Second)
		ansResetReq := env.server.WaitAnswerCb(t, 2*time.Second)

		if !strings.Contains(ansResetReq.Params["text"], "流量已重置") {
			t.Errorf("expected toast '流量已重置', got: %s", ansResetReq.Params["text"])
		}
		if !strings.Contains(editResetReq.Params["text"], "0.00 GB") {
			t.Errorf("expected traffic reset to 0.00 GB, got: %s", editResetReq.Params["text"])
		}
	})

	t.Run("user:sub subscription modal alert", func(t *testing.T) {
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "user:sub:42")
		ansSubReq := env.server.WaitAnswerCb(t, 2*time.Second)

		if ansSubReq.Params["show_alert"] != "true" {
			t.Errorf("expected show_alert=true for subscription modal alert")
		}
		if !strings.Contains(ansSubReq.Params["text"], "token-alice-42") {
			t.Errorf("expected sub token in alert text, got: %s", ansSubReq.Params["text"])
		}
	})

	t.Run("user:refresh in-place card update", func(t *testing.T) {
		env.server.SendCallbackUpdate(adminID, adminID, 1001, "user:refresh:42")
		editRefReq := env.server.WaitEditMsg(t, 2*time.Second)
		ansRefReq := env.server.WaitAnswerCb(t, 2*time.Second)

		if !strings.Contains(ansRefReq.Params["text"], "用户状态已刷新") {
			t.Errorf("expected toast '用户状态已刷新', got: %s", ansRefReq.Params["text"])
		}
		if editRefReq.Params["message_id"] != "1001" {
			t.Errorf("expected edit message_id 1001, got: %s", editRefReq.Params["message_id"])
		}
	})
}

func TestBotHandler_AddUser(t *testing.T) {
	adminID := int64(88888)
	env := setupTestEnv(t, adminID)

	t.Run("adduser with full parameters", func(t *testing.T) {
		env.server.SendMessageUpdate(adminID, "/adduser bob@example.com 20 30")
		sentReq := env.server.WaitSentMsg(t, 2*time.Second)

		if len(env.userSvc.createCalls) == 0 {
			t.Fatalf("expected CreateUser to be called")
		}
		dto := env.userSvc.createCalls[0]
		if dto.Email != "bob@example.com" {
			t.Errorf("expected email bob@example.com, got: %s", dto.Email)
		}
		if dto.TotalBytes != 20*1024*1024*1024 {
			t.Errorf("expected TotalBytes 20GB, got: %d", dto.TotalBytes)
		}
		if dto.ExpireDays != 30 {
			t.Errorf("expected ExpireDays 30, got: %d", dto.ExpireDays)
		}
		if len(dto.InboundTags) != 1 || dto.InboundTags[0] != "vless-in" {
			t.Errorf("expected active inbound tag vless-in, got: %v", dto.InboundTags)
		}

		if !strings.Contains(sentReq.Params["text"], "用户创建成功") {
			t.Errorf("expected success notification, got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["text"], "bob@example.com") {
			t.Errorf("expected email in text, got: %s", sentReq.Params["text"])
		}
		if !strings.Contains(sentReq.Params["text"], "20.00 GB") {
			t.Errorf("expected 20.00 GB in text, got: %s", sentReq.Params["text"])
		}
	})

	t.Run("adduser with default parameters", func(t *testing.T) {
		env.server.SendMessageUpdate(adminID, "/adduser default@example.com")
		sentReq := env.server.WaitSentMsg(t, 2*time.Second)

		if len(env.userSvc.createCalls) < 2 {
			t.Fatalf("expected CreateUser second call")
		}
		dto := env.userSvc.createCalls[1]
		if dto.Email != "default@example.com" {
			t.Errorf("expected email default@example.com, got: %s", dto.Email)
		}
		if dto.TotalBytes != 0 {
			t.Errorf("expected default TotalBytes 0, got: %d", dto.TotalBytes)
		}
		if dto.ExpireDays != 0 {
			t.Errorf("expected default ExpireDays 0, got: %d", dto.ExpireDays)
		}
		if !strings.Contains(sentReq.Params["text"], "无限制") {
			t.Errorf("expected '无限制' for default quota, got: %s", sentReq.Params["text"])
		}
	})

	t.Run("adduser invalid arguments error", func(t *testing.T) {
		env.server.SendMessageUpdate(adminID, "/adduser invalid-email-format")
		sentReq := env.server.WaitSentMsg(t, 2*time.Second)
		if !strings.Contains(sentReq.Params["text"], "参数错误") {
			t.Errorf("expected error message for invalid email, got: %s", sentReq.Params["text"])
		}
	})
}

func TestBotHandler_ActionLock_ConcurrencyToast(t *testing.T) {
	adminID := int64(88888)
	env := setupTestEnv(t, adminID)

	restartEntered := make(chan struct{})
	releaseRestart := make(chan struct{})

	env.xrayManager.mu.Lock()
	env.xrayManager.restartFunc = func(ctx context.Context) error {
		select {
		case restartEntered <- struct{}{}:
		default:
		}
		select {
		case <-releaseRestart:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	env.xrayManager.mu.Unlock()

	// 1. 发起耗时操作 status:restart
	env.server.SendCallbackUpdate(adminID, adminID, 1001, "status:restart")

	// 等待进入 RestartService，此时 actionMu 已被第一个 goroutine 锁定
	select {
	case <-restartEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for restartFunc to enter")
	}

	// 消费 status:restart 的第一条 "正在重启" answerCallbackQuery
	firstAns := env.server.WaitAnswerCb(t, 2*time.Second)
	if !strings.Contains(firstAns.Params["text"], "正在重启") {
		t.Errorf("expected '正在重启' first answer, got: %s", firstAns.Params["text"])
	}

	// 2. 在第一个回调持有锁期间，模拟并发连击发送第二个回调 query (status:refresh)
	env.server.SendCallbackUpdate(adminID, adminID, 1001, "status:refresh")

	// 第二个请求必须被 actionMu.TryLock() 拦截并立即返回防连击 Toast
	conflictAns := env.server.WaitAnswerCb(t, 2*time.Second)
	if !strings.Contains(conflictAns.Params["text"], "操作正在执行中，请勿重复点击") {
		t.Errorf("expected concurrent collision toast, got: %s", conflictAns.Params["text"])
	}
	if conflictAns.Params["show_alert"] == "true" {
		t.Errorf("toast should not have show_alert set to true")
	}

	// 3. 释放第一个操作
	close(releaseRestart)

	// 第一个操作正常完成并在原消息处刷新卡片
	editReq := env.server.WaitEditMsg(t, 3*time.Second)
	if editReq.Params["message_id"] != "1001" {
		t.Errorf("expected edit message_id 1001, got: %s", editReq.Params["message_id"])
	}
}


