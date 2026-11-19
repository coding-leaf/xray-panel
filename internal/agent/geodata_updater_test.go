package agent

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeServiceReloader struct {
	mu    sync.Mutex
	calls int
	fn    func(ctx context.Context, call int) error
}

func (f *fakeServiceReloader) ReloadAndProbe(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if f.fn != nil {
		return f.fn(ctx, call)
	}
	return nil
}

func TestHTTPGeoDataUpdater_AtomicUpdateSuccess(t *testing.T) {
	assetsDir := t.TempDir()
	oldGeoIP := bytes.Repeat([]byte("old-geoip-"), 200)
	oldGeoSite := bytes.Repeat([]byte("old-geosite-"), 200)
	if err := os.WriteFile(filepath.Join(assetsDir, "geoip.dat"), oldGeoIP, 0o644); err != nil {
		t.Fatalf("seed old geoip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "geosite.dat"), oldGeoSite, 0o644); err != nil {
		t.Fatalf("seed old geosite: %v", err)
	}

	newGeoIP := bytes.Repeat([]byte("G"), 120*1024)
	newGeoSite := bytes.Repeat([]byte("S"), 150*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/geoip.dat":
			_, _ = w.Write(newGeoIP)
		case "/geosite.dat":
			_, _ = w.Write(newGeoSite)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reloader := &fakeServiceReloader{}
	updater, err := NewHTTPGeoDataUpdater(HTTPGeoDataUpdaterConfig{
		AssetsDir:   assetsDir,
		GeoIPURLs:   []string{srv.URL + "/geoip.dat"},
		GeoSiteURLs: []string{srv.URL + "/geosite.dat"},
		HTTPClient:  srv.Client(),
	}, reloader)
	if err != nil {
		t.Fatalf("NewHTTPGeoDataUpdater: %v", err)
	}

	ipSize, siteSize, err := updater.Update(context.Background())
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if ipSize != int64(len(newGeoIP)) || siteSize != int64(len(newGeoSite)) {
		t.Fatalf("unexpected sizes: geoip=%d geosite=%d", ipSize, siteSize)
	}
	if reloader.calls != 1 {
		t.Fatalf("expected 1 ReloadAndProbe call, got %d", reloader.calls)
	}

	gotGeoIP, _ := os.ReadFile(filepath.Join(assetsDir, "geoip.dat"))
	gotGeoSite, _ := os.ReadFile(filepath.Join(assetsDir, "geosite.dat"))
	if !bytes.Equal(gotGeoIP, newGeoIP) || !bytes.Equal(gotGeoSite, newGeoSite) {
		t.Fatalf("installed files do not match downloaded payloads")
	}

	bakGeoIP, _ := os.ReadFile(filepath.Join(assetsDir, "geoip.dat.bak"))
	bakGeoSite, _ := os.ReadFile(filepath.Join(assetsDir, "geosite.dat.bak"))
	if !bytes.Equal(bakGeoIP, oldGeoIP) || !bytes.Equal(bakGeoSite, oldGeoSite) {
		t.Fatalf("backup files do not match previous assets")
	}
}

func TestHTTPGeoDataUpdater_RejectsSmallHTMLAndFallsBackToMirror(t *testing.T) {
	assetsDir := t.TempDir()
	oldGeoIP := []byte("valid-existing-geoip")
	oldGeoSite := []byte("valid-existing-geosite")
	_ = os.WriteFile(filepath.Join(assetsDir, "geoip.dat"), oldGeoIP, 0o644)
	_ = os.WriteFile(filepath.Join(assetsDir, "geosite.dat"), oldGeoSite, 0o644)

	validGeoIP := bytes.Repeat([]byte("I"), 110*1024)
	validGeoSite := bytes.Repeat([]byte("D"), 115*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-html":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>404 Not Found / Rate Limited</body></html>"))
		case "/mirror-geoip.dat":
			_, _ = w.Write(validGeoIP)
		case "/mirror-geosite.dat":
			_, _ = w.Write(validGeoSite)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Part A: Primary returns <100KB HTML, fallback mirror succeeds.
	reloader := &fakeServiceReloader{}
	updater, err := NewHTTPGeoDataUpdater(HTTPGeoDataUpdaterConfig{
		AssetsDir:   assetsDir,
		GeoIPURLs:   []string{srv.URL + "/bad-html", srv.URL + "/mirror-geoip.dat"},
		GeoSiteURLs: []string{srv.URL + "/404", srv.URL + "/mirror-geosite.dat"},
		HTTPClient:  srv.Client(),
	}, reloader)
	if err != nil {
		t.Fatalf("NewHTTPGeoDataUpdater: %v", err)
	}

	ipSize, siteSize, err := updater.Update(context.Background())
	if err != nil {
		t.Fatalf("expected fallback mirror to succeed, got: %v", err)
	}
	if ipSize != int64(len(validGeoIP)) || siteSize != int64(len(validGeoSite)) {
		t.Fatalf("unexpected fallback sizes: %d, %d", ipSize, siteSize)
	}

	// Part B: All sources return <100KB HTML -> must fail and preserve existing files without reloading.
	_ = os.WriteFile(filepath.Join(assetsDir, "geoip.dat"), oldGeoIP, 0o644)
	_ = os.WriteFile(filepath.Join(assetsDir, "geosite.dat"), oldGeoSite, 0o644)
	reloader2 := &fakeServiceReloader{}
	updater2, err := NewHTTPGeoDataUpdater(HTTPGeoDataUpdaterConfig{
		AssetsDir:   assetsDir,
		GeoIPURLs:   []string{srv.URL + "/bad-html"},
		GeoSiteURLs: []string{srv.URL + "/bad-html"},
		HTTPClient:  srv.Client(),
	}, reloader2)
	if err != nil {
		t.Fatalf("NewHTTPGeoDataUpdater #2: %v", err)
	}

	_, _, err = updater2.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "below minimum") {
		t.Fatalf("expected minimum size rejection error, got: %v", err)
	}
	if reloader2.calls != 0 {
		t.Fatalf("expected 0 reload calls when download validation fails, got %d", reloader2.calls)
	}
	gotGeoIP, _ := os.ReadFile(filepath.Join(assetsDir, "geoip.dat"))
	gotGeoSite, _ := os.ReadFile(filepath.Join(assetsDir, "geosite.dat"))
	if !bytes.Equal(gotGeoIP, oldGeoIP) || !bytes.Equal(gotGeoSite, oldGeoSite) {
		t.Fatalf("existing assets were unexpectedly modified on failed download")
	}
}

func TestHTTPGeoDataUpdater_ProbeFailureRollsBackToBak(t *testing.T) {
	assetsDir := t.TempDir()
	oldGeoIP := bytes.Repeat([]byte("old-ip-"), 50)
	oldGeoSite := bytes.Repeat([]byte("old-site-"), 50)
	if err := os.WriteFile(filepath.Join(assetsDir, "geoip.dat"), oldGeoIP, 0o644); err != nil {
		t.Fatalf("seed old geoip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "geosite.dat"), oldGeoSite, 0o644); err != nil {
		t.Fatalf("seed old geosite: %v", err)
	}

	newGeoIP := bytes.Repeat([]byte("N"), 105*1024)
	newGeoSite := bytes.Repeat([]byte("M"), 105*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/geoip.dat":
			_, _ = w.Write(newGeoIP)
		case "/geosite.dat":
			_, _ = w.Write(newGeoSite)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reloader := &fakeServiceReloader{
		fn: func(ctx context.Context, call int) error {
			if call == 1 {
				return errors.New("xray grpc probe failed: invalid dat")
			}
			return nil
		},
	}

	updater, err := NewHTTPGeoDataUpdater(HTTPGeoDataUpdaterConfig{
		AssetsDir:   assetsDir,
		GeoIPURLs:   []string{srv.URL + "/geoip.dat"},
		GeoSiteURLs: []string{srv.URL + "/geosite.dat"},
		HTTPClient:  srv.Client(),
	}, reloader)
	if err != nil {
		t.Fatalf("NewHTTPGeoDataUpdater: %v", err)
	}

	_, _, err = updater.Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got: %v", err)
	}
	if reloader.calls != 2 {
		t.Fatalf("expected 2 ReloadAndProbe calls (update + rollback), got %d", reloader.calls)
	}

	gotGeoIP, _ := os.ReadFile(filepath.Join(assetsDir, "geoip.dat"))
	gotGeoSite, _ := os.ReadFile(filepath.Join(assetsDir, "geosite.dat"))
	if !bytes.Equal(gotGeoIP, oldGeoIP) || !bytes.Equal(gotGeoSite, oldGeoSite) {
		t.Fatalf("expected both assets rolled back to previous contents")
	}
}
