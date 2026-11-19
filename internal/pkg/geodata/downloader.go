package geodata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultAssetsDir is the standard Linux Xray assets directory.
	DefaultAssetsDir = "/usr/local/share/xray"
	// DefaultMinFileSize guards against HTML error or rate-limit pages masquerading as .dat files.
	DefaultMinFileSize = int64(100 * 1024)
	// DefaultUserAgent is sent on GeoData mirror requests.
	DefaultUserAgent = "Xray-Panel-GeoUpdater/1.0"
)

var (
	// DefaultGeoIPURLs lists primary and fallback download mirrors for geoip.dat.
	DefaultGeoIPURLs = []string{
		"https://github.com/v2fly/geoip/releases/latest/download/geoip.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	}
	// DefaultGeoSiteURLs lists primary and fallback download mirrors for geosite.dat.
	DefaultGeoSiteURLs = []string{
		"https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
	}
)

// NewHTTPClient builds an HTTP client with explicit dial, TLS, and response header timeouts.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

type progressWriter struct {
	written    int64
	total      int64
	onProgress func(written, total int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.onProgress != nil {
		pw.onProgress(pw.written, pw.total)
	}
	return n, nil
}

// DownloadFile downloads a single URL to tmpPath, enforces HTTP 200 OK and optional minSize,
// syncs and chmods the file to 0644, and removes tmpPath on any failure.
func DownloadFile(
	ctx context.Context,
	client *http.Client,
	url, tmpPath string,
	minSize int64,
	onProgress func(written, total int64),
) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = NewHTTPClient(180 * time.Second)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d (%s)", resp.StatusCode, resp.Status)
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open temp file: %w", err)
	}

	var src io.Reader = resp.Body
	if onProgress != nil {
		src = io.TeeReader(resp.Body, &progressWriter{
			total:      resp.ContentLength,
			onProgress: onProgress,
		})
	}

	written, copyErr := io.Copy(f, src)
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("stream body: %w", copyErr)
	}
	if syncErr != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("sync temp file: %w", syncErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("close temp file: %w", closeErr)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("chmod temp file: %w", err)
	}
	if minSize > 0 && written < minSize {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("downloaded size %d bytes is below minimum %d bytes", written, minSize)
	}
	return written, nil
}

// DownloadWithFallback tries each URL in order until one succeeds or ctx is canceled.
func DownloadWithFallback(
	ctx context.Context,
	client *http.Client,
	urls []string,
	tmpPath string,
	minSize int64,
	onProgress func(written, total int64),
) (int64, error) {
	if len(urls) == 0 {
		return 0, errors.New("no download URLs provided")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	for _, u := range urls {
		if ctxErr := ctx.Err(); ctxErr != nil {
			errs = append(errs, ctxErr)
			break
		}
		written, err := DownloadFile(ctx, client, u, tmpPath, minSize, onProgress)
		if err == nil {
			return written, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", u, err))
		if ctx.Err() != nil {
			break
		}
	}
	return 0, fmt.Errorf("download failed across %d source(s): %w", len(urls), errors.Join(errs...))
}
