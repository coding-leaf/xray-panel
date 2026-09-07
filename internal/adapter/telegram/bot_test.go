package telegram_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"panel/internal/adapter/telegram"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestBotHandler_LifecycleNilBot(t *testing.T) {
	adapter := telegram.NewBotAdapter("", 0)
	_ = adapter.Init()

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, "http://localhost")

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

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, "http://localhost")

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

	handler := telegram.NewBotHandler(adapter, nil, nil, nil, nil, "http://localhost")

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

