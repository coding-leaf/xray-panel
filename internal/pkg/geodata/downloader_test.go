package geodata

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClient_Timeouts(t *testing.T) {
	client := NewHTTPClient(60 * time.Second)
	if client.Timeout != 60*time.Second {
		t.Fatalf("expected timeout 60s, got %v", client.Timeout)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if tr.TLSHandshakeTimeout != 15*time.Second {
		t.Fatalf("expected TLSHandshakeTimeout=15s, got %v", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("expected ResponseHeaderTimeout=30s, got %v", tr.ResponseHeaderTimeout)
	}
	if tr.DialContext == nil {
		t.Fatal("expected DialContext to be configured")
	}

	defaultClient := NewHTTPClient(0)
	if defaultClient.Timeout != 180*time.Second {
		t.Fatalf("expected default timeout 180s for non-positive input, got %v", defaultClient.Timeout)
	}
}

func TestDownloadFile_SuccessAndProgress(t *testing.T) {
	payload := bytes.Repeat([]byte("G"), int(DefaultMinFileSize)+4096)
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	tmpPath := filepath.Join(t.TempDir(), "geoip.dat.tmp")
	var lastWritten, lastTotal int64
	progressCalls := 0
	written, err := DownloadFile(context.Background(), srv.Client(), srv.URL+"/geoip.dat", tmpPath, DefaultMinFileSize, func(w, total int64) {
		progressCalls++
		lastWritten = w
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	if gotUA != DefaultUserAgent {
		t.Fatalf("expected User-Agent %q, got %q", DefaultUserAgent, gotUA)
	}
	if written != int64(len(payload)) {
		t.Fatalf("expected written=%d, got %d", len(payload), written)
	}
	if progressCalls == 0 || lastWritten != written || lastTotal != written {
		t.Fatalf("unexpected progress callback state: calls=%d written=%d total=%d", progressCalls, lastWritten, lastTotal)
	}
	gotBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read tmpPath failed: %v", err)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Fatalf("downloaded payload mismatch")
	}
}

func TestDownloadFile_RejectsSmallHTMLAndNon200AndCleansTmp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/html":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>Rate Limited</body></html>"))
		case "/err":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	tmpHTML := filepath.Join(dir, "small.tmp")
	if _, err := DownloadFile(context.Background(), srv.Client(), srv.URL+"/html", tmpHTML, DefaultMinFileSize, nil); err == nil || !strings.Contains(err.Error(), "below minimum") {
		t.Fatalf("expected below minimum error for HTML error page, got: %v", err)
	}
	if _, err := os.Stat(tmpHTML); !os.IsNotExist(err) {
		t.Fatalf("expected small tmp file to be cleaned up, stat err=%v", err)
	}

	tmpErr := filepath.Join(dir, "err.tmp")
	if _, err := DownloadFile(context.Background(), srv.Client(), srv.URL+"/err", tmpErr, DefaultMinFileSize, nil); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected HTTP 502 error, got: %v", err)
	}
	if _, err := os.Stat(tmpErr); !os.IsNotExist(err) {
		t.Fatalf("expected non-200 tmp file not to exist, stat err=%v", err)
	}
}

func TestDownloadWithFallback_MirrorFallbackAndCtxCancel(t *testing.T) {
	validPayload := bytes.Repeat([]byte("S"), int(DefaultMinFileSize)+1024)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/bad-small":
			_, _ = w.Write([]byte("<html>oops</html>"))
		case "/good":
			_, _ = w.Write(validPayload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	tmpPath := filepath.Join(dir, "geosite.dat.tmp")
	written, err := DownloadWithFallback(
		context.Background(),
		srv.Client(),
		[]string{srv.URL + "/404", srv.URL + "/bad-small", srv.URL + "/good"},
		tmpPath,
		DefaultMinFileSize,
		nil,
	)
	if err != nil {
		t.Fatalf("expected fallback mirror to succeed, got: %v", err)
	}
	if written != int64(len(validPayload)) {
		t.Fatalf("expected %d bytes, got %d", len(validPayload), written)
	}

	// Canceled context stops immediately without hitting mirrors.
	hitsBefore := hits
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DownloadWithFallback(ctx, srv.Client(), []string{srv.URL + "/good", srv.URL + "/good"}, filepath.Join(dir, "canceled.tmp"), DefaultMinFileSize, nil)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}
	if hits != hitsBefore {
		t.Fatalf("expected 0 HTTP requests on pre-canceled ctx, got %d", hits-hitsBefore)
	}

	// Empty URL list fails explicitly.
	if _, err := DownloadWithFallback(context.Background(), srv.Client(), nil, filepath.Join(dir, "empty.tmp"), DefaultMinFileSize, nil); err == nil {
		t.Fatal("expected error for empty URL list")
	}
}
