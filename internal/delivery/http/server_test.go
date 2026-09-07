package http_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	deliveryHTTP "panel/internal/delivery/http"
)

func getFreePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	return port
}

func TestServer_GracefulShutdown(t *testing.T) {
	port := getFreePort(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	srv := deliveryHTTP.NewServer(port, mux, deliveryHTTP.WithShutdownTimeout(2*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to be reachable
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var resp *http.Response
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		var err error
		resp, err = client.Get("http://127.0.0.1:" + port + "/ping")
		if err == nil {
			break
		}
	}
	if resp == nil {
		cancel()
		t.Fatal("failed to reach HTTP server")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("expected pong, got %s", string(body))
	}

	// Trigger graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on graceful shutdown, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server failed to shutdown within timeout")
	}
}

func TestServer_BindError(t *testing.T) {
	// Bind a port first
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	_, port, _ := net.SplitHostPort(l.Addr().String())

	mux := http.NewServeMux()
	srv := deliveryHTTP.NewServer(port, mux)

	ctx := context.Background()
	err = srv.Start(ctx)
	if err == nil {
		t.Fatal("expected bind error when port is already in use, got nil")
	}
}

func TestServer_NilServer(t *testing.T) {
	srv := deliveryHTTP.NewServerWithHTTPServer(nil)
	err := srv.Start(context.Background())
	if err == nil {
		t.Fatal("expected error with nil http.Server")
	}
}

func TestServer_KeepAliveDraining(t *testing.T) {
	port := getFreePort(t)
	mux := http.NewServeMux()

	requestStarted := make(chan struct{}, 1)
	requestFinished := make(chan struct{}, 1)

	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		// Simulate in-flight slow processing
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow-completed"))
		select {
		case requestFinished <- struct{}{}:
		default:
		}
	})

	srv := deliveryHTTP.NewServer(port, mux, deliveryHTTP.WithShutdownTimeout(2*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to start
	client := &http.Client{Timeout: 3 * time.Second}
	time.Sleep(50 * time.Millisecond)

	// Launch active long-lived request
	respCh := make(chan string, 1)
	go func() {
		resp, err := client.Get("http://127.0.0.1:" + port + "/slow")
		if err != nil {
			respCh <- "err: " + err.Error()
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		respCh <- string(body)
	}()

	// Wait until handler is actively running
	select {
	case <-requestStarted:
	case <-time.After(1 * time.Second):
		cancel()
		t.Fatal("in-flight request did not hit server in time")
	}

	// Trigger shutdown WHILE request is in flight
	cancel()

	// Verify the in-flight request was drained cleanly without being abruptly killed
	select {
	case body := <-respCh:
		if body != "slow-completed" {
			t.Fatalf("expected in-flight request to drain successfully, got: %s", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request was not drained in time")
	}

	// Verify server exited cleanly
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected nil error on clean drain shutdown, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("server failed to shutdown cleanly after draining")
	}
}

func TestServer_UnexpectedStop(t *testing.T) {
	port := getFreePort(t)
	mux := http.NewServeMux()
	rawServer := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	srv := deliveryHTTP.NewServerWithHTTPServer(rawServer)

	ctx := context.Background() // not canceled
	errCh := make(chan error, 1)

	go func() {
		errCh <- srv.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Force premature close without canceling ctx
	_ = rawServer.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error when server terminates unexpectedly before ctx is canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not return after Close()")
	}
}

