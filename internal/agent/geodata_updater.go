package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultXrayAssetsDir        = "/usr/local/share/xray"
	defaultMinGeoDataFileSize   = int64(100 * 1024) // 100KB guard against HTML error/rate-limit pages
	defaultGeoDownloadTimeout   = 60 * time.Second
	geodataRollbackProbeTimeout = 15 * time.Second
)

var (
	defaultGeoIPURLs = []string{
		"https://github.com/v2fly/geoip/releases/latest/download/geoip.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	}
	defaultGeoSiteURLs = []string{
		"https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
	}
)

// GeoDataUpdater downloads and atomically installs updated geoip.dat and geosite.dat files.
type GeoDataUpdater interface {
	Update(ctx context.Context) (geoipSize, geositeSize int64, err error)
}

// ServiceReloader reloads Xray and probes its local gRPC endpoint after asset updates.
type ServiceReloader interface {
	ReloadAndProbe(ctx context.Context) error
}

// HTTPGeoDataUpdaterConfig configures HTTPGeoDataUpdater.
type HTTPGeoDataUpdaterConfig struct {
	AssetsDir   string
	GeoIPURLs   []string
	GeoSiteURLs []string
	MinFileSize int64
	HTTPClient  *http.Client
}

// HTTPGeoDataUpdater downloads geoip.dat and geosite.dat via HTTP with fallback mirrors,
// enforces minimum size validation, backs up existing files, atomically swaps them,
// and rolls back automatically if ReloadAndProbe fails.
type HTTPGeoDataUpdater struct {
	mu          sync.Mutex
	assetsDir   string
	geoipURLs   []string
	geositeURLs []string
	minFileSize int64
	client      *http.Client
	reloader    ServiceReloader
}

// NewHTTPGeoDataUpdater constructs an HTTPGeoDataUpdater with sensible defaults.
func NewHTTPGeoDataUpdater(cfg HTTPGeoDataUpdaterConfig, reloader ServiceReloader) (*HTTPGeoDataUpdater, error) {
	if reloader == nil {
		return nil, errors.New("service reloader is required")
	}
	assetsDir := strings.TrimSpace(cfg.AssetsDir)
	if assetsDir == "" {
		assetsDir = defaultXrayAssetsDir
	}
	geoipURLs := append([]string(nil), cfg.GeoIPURLs...)
	if len(geoipURLs) == 0 {
		geoipURLs = append([]string(nil), defaultGeoIPURLs...)
	}
	geositeURLs := append([]string(nil), cfg.GeoSiteURLs...)
	if len(geositeURLs) == 0 {
		geositeURLs = append([]string(nil), defaultGeoSiteURLs...)
	}
	minSize := cfg.MinFileSize
	if minSize <= 0 {
		minSize = defaultMinGeoDataFileSize
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultGeoDownloadTimeout}
	}

	return &HTTPGeoDataUpdater{
		assetsDir:   assetsDir,
		geoipURLs:   geoipURLs,
		geositeURLs: geositeURLs,
		minFileSize: minSize,
		client:      client,
		reloader:    reloader,
	}, nil
}

// Update downloads geoip.dat and geosite.dat to temporary files, validates sizes,
// backs up existing files to .bak, atomically renames both, and reloads/probes Xray.
// If reload or probe fails, it restores the .bak files and reloads Xray again.
func (u *HTTPGeoDataUpdater) Update(ctx context.Context) (int64, int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if err := os.MkdirAll(u.assetsDir, 0o755); err != nil {
		return 0, 0, fmt.Errorf("create xray assets dir %s: %w", u.assetsDir, err)
	}

	geoipPath := filepath.Join(u.assetsDir, "geoip.dat")
	geositePath := filepath.Join(u.assetsDir, "geosite.dat")
	geoipTmp := geoipPath + ".tmp"
	geositeTmp := geositePath + ".tmp"
	geoipBak := geoipPath + ".bak"
	geositeBak := geositePath + ".bak"

	defer func() {
		_ = os.Remove(geoipTmp)
		_ = os.Remove(geositeTmp)
	}()

	geoipSize, err := u.downloadWithFallback(ctx, "geoip.dat", u.geoipURLs, geoipTmp)
	if err != nil {
		return 0, 0, err
	}
	geositeSize, err := u.downloadWithFallback(ctx, "geosite.dat", u.geositeURLs, geositeTmp)
	if err != nil {
		return 0, 0, err
	}

	hasGeoIPBak, err := backupIfExists(geoipPath, geoipBak)
	if err != nil {
		return 0, 0, fmt.Errorf("backup geoip.dat: %w", err)
	}
	hasGeoSiteBak, err := backupIfExists(geositePath, geositeBak)
	if err != nil {
		return 0, 0, fmt.Errorf("backup geosite.dat: %w", err)
	}

	if err := os.Rename(geoipTmp, geoipPath); err != nil {
		return 0, 0, fmt.Errorf("install geoip.dat: %w", err)
	}
	if err := os.Rename(geositeTmp, geositePath); err != nil {
		rbErr := restoreAssets(geoipPath, geoipBak, hasGeoIPBak, geositePath, geositeBak, hasGeoSiteBak)
		if rbErr != nil {
			return 0, 0, fmt.Errorf("install geosite.dat (%w); rollback failed: %v", err, rbErr)
		}
		return 0, 0, fmt.Errorf("install geosite.dat: %w", err)
	}

	if err := u.reloader.ReloadAndProbe(ctx); err != nil {
		rbErr := u.rollback(ctx, geoipPath, geoipBak, hasGeoIPBak, geositePath, geositeBak, hasGeoSiteBak)
		if rbErr != nil {
			return 0, 0, fmt.Errorf("reload/probe after geodata update failed (%w); rollback failed: %v", err, rbErr)
		}
		return 0, 0, fmt.Errorf("reload/probe after geodata update failed (%w); rolled back to previous assets", err)
	}

	return geoipSize, geositeSize, nil
}

func (u *HTTPGeoDataUpdater) downloadWithFallback(ctx context.Context, name string, urls []string, tmpPath string) (int64, error) {
	var errs []error
	for _, url := range urls {
		size, err := u.downloadToTemp(ctx, url, tmpPath)
		if err == nil {
			return size, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", url, err))
	}
	return 0, fmt.Errorf("download %s failed across %d source(s): %w", name, len(urls), errors.Join(errs...))
}

func (u *HTTPGeoDataUpdater) downloadToTemp(ctx context.Context, url, tmpPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open temp file: %w", err)
	}

	written, copyErr := io.Copy(f, resp.Body)
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
	if written < u.minFileSize {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("downloaded size %d bytes is below minimum %d bytes", written, u.minFileSize)
	}
	return written, nil
}

func (u *HTTPGeoDataUpdater) rollback(
	ctx context.Context,
	geoipPath, geoipBak string, hasGeoIPBak bool,
	geositePath, geositeBak string, hasGeoSiteBak bool,
) error {
	if err := restoreAssets(geoipPath, geoipBak, hasGeoIPBak, geositePath, geositeBak, hasGeoSiteBak); err != nil {
		return err
	}
	rbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), geodataRollbackProbeTimeout)
	defer cancel()
	if err := u.reloader.ReloadAndProbe(rbCtx); err != nil {
		return fmt.Errorf("reload after asset restore: %w", err)
	}
	return nil
}

func backupIfExists(srcPath, bakPath string) (bool, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer src.Close()

	dst, err := os.OpenFile(bakPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return false, err
	}
	if err := dst.Sync(); err != nil {
		_ = dst.Close()
		return false, err
	}
	if err := dst.Close(); err != nil {
		return false, err
	}
	if err := os.Chmod(bakPath, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func restoreAssets(
	geoipPath, geoipBak string, hasGeoIPBak bool,
	geositePath, geositeBak string, hasGeoSiteBak bool,
) error {
	var errs []error
	if err := restoreSingleAsset(geoipPath, geoipBak, hasGeoIPBak); err != nil {
		errs = append(errs, fmt.Errorf("restore geoip.dat: %w", err))
	}
	if err := restoreSingleAsset(geositePath, geositeBak, hasGeoSiteBak); err != nil {
		errs = append(errs, fmt.Errorf("restore geosite.dat: %w", err))
	}
	return errors.Join(errs...)
}

func restoreSingleAsset(targetPath, bakPath string, hasBak bool) error {
	if !hasBak {
		if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.Rename(bakPath, targetPath); err != nil {
		return err
	}
	return os.Chmod(targetPath, 0o644)
}
