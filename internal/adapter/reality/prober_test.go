package reality_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"panel/internal/adapter/reality"
)

func TestProbe_Success(t *testing.T) {
	// Start local TLS server forcing TLS 1.3
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "test-server")
		w.Header().Set("X-Custom-Header", "probe-ok")
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"},
	}
	ts.StartTLS()
	defer ts.Close()

	host, portStr, err := net.SplitHostPort(ts.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	prober := reality.NewDefaultRealityProber()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := prober.Probe(ctx, host, port, "example.com")
	if err != nil {
		t.Fatalf("prober.Probe returned unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("prober.Probe returned nil result")
	}
	if res.TCPError != nil {
		t.Fatalf("expected TCPError to be nil, got: %v", res.TCPError)
	}
	if res.TLSVersion != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3 (0x%04x), got 0x%04x", tls.VersionTLS13, res.TLSVersion)
	}
	if res.Cert == nil {
		t.Errorf("expected peer certificate, got nil")
	}
	if res.Headers == nil || res.Headers.Get("Server") != "test-server" {
		t.Errorf("expected header Server='test-server', got '%s'", res.Headers.Get("Server"))
	}
}

func TestProbe_Timeout(t *testing.T) {
	prober := reality.NewDefaultRealityProber()
	// Use an unrouteable private IP with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := prober.Probe(ctx, "10.255.255.1", 443, "timeout.example.com")
	if err != nil {
		// Handled via error return is also acceptable
		return
	}
	if res == nil {
		t.Fatalf("expected non-nil result or error")
	}
	if res.TCPError == nil {
		t.Errorf("expected TCPError on timeout, got nil")
	}
}

func TestProbe_ConnectionRefused(t *testing.T) {
	// Find an unused port by binding and immediately closing
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on port 0: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	prober := reality.NewDefaultRealityProber()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := prober.Probe(ctx, host, port, "example.com")
	if err != nil {
		return
	}
	if res == nil || res.TCPError == nil {
		t.Errorf("expected TCPError for closed port, got res=%v", res)
	}
}
