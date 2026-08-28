package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"panel/internal/adapter/xray"
)

type GeoDataStatus struct {
	GeoIPExists     bool      `json:"geoipExists"`
	GeoIPSize       int64     `json:"geoipSize"`
	GeoIPModTime    time.Time `json:"geoipModTime"`
	GeoSiteExists   bool      `json:"geositeExists"`
	GeoSiteSize     int64     `json:"geositeSize"`
	GeoSiteModTime  time.Time `json:"geositeModTime"`
	TargetDirectory string    `json:"targetDirectory"`
	CoreVersion     string    `json:"coreVersion"`
	Platform        string    `json:"platform"`
}

type GeoDataService struct {
	xrayBinPath string
	manager     *xray.Manager
}

func NewGeoDataService(xrayBinPath string, manager *xray.Manager) *GeoDataService {
	return &GeoDataService{
		xrayBinPath: xrayBinPath,
		manager:     manager,
	}
}

func (s *GeoDataService) getGeoDataDir() string {
	dir := filepath.Dir(s.xrayBinPath)
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/usr/local/share/xray"); err == nil {
			return "/usr/local/share/xray"
		}
	}
	return dir
}

func (s *GeoDataService) GetStatus(ctx context.Context) (*GeoDataStatus, error) {
	dataDir := s.getGeoDataDir()
	status := &GeoDataStatus{
		TargetDirectory: dataDir,
		Platform:        runtime.GOOS + "/" + runtime.GOARCH,
	}

	geoIPPath := filepath.Join(dataDir, "geoip.dat")
	if fi, err := os.Stat(geoIPPath); err == nil {
		status.GeoIPExists = true
		status.GeoIPSize = fi.Size()
		status.GeoIPModTime = fi.ModTime()
	}

	geoSitePath := filepath.Join(dataDir, "geosite.dat")
	if fi, err := os.Stat(geoSitePath); err == nil {
		status.GeoSiteExists = true
		status.GeoSiteSize = fi.Size()
		status.GeoSiteModTime = fi.ModTime()
	}

	if s.manager != nil {
		status.CoreVersion, _ = s.manager.GetVersion(ctx)
	}

	return status, nil
}

func (s *GeoDataService) UpdateGeoData(ctx context.Context) error {
	// 在 Linux 环境下优先调用官方成熟运维脚本：install-release.sh @ install-geodata
	if runtime.GOOS == "linux" {
		cmd := exec.CommandContext(ctx, "bash", "-c", "curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh | bash -s -- install-geodata")
		if output, err := cmd.CombinedOutput(); err == nil {
			if s.manager != nil {
				_ = s.manager.RestartService(ctx)
			}
			return nil
		} else {
			_ = output // 出错时回退到纯 Go HTTP 下载引擎
		}
	}

	// 纯 Go HTTP 下载引擎（跨平台 Windows / Linux 备用）
	dataDir := s.getGeoDataDir()
	_ = os.MkdirAll(dataDir, 0755)

	client := &http.Client{Timeout: 60 * time.Second}

	// 1. 下载 geoip.dat
	if err := downloadFile(ctx, client, "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat", filepath.Join(dataDir, "geoip.dat")); err != nil {
		// 备用镜像源
		_ = downloadFile(ctx, client, "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat", filepath.Join(dataDir, "geoip.dat"))
	}

	// 2. 下载 geosite.dat (或 dlc.dat)
	if err := downloadFile(ctx, client, "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat", filepath.Join(dataDir, "geosite.dat")); err != nil {
		_ = downloadFile(ctx, client, "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat", filepath.Join(dataDir, "geosite.dat"))
	}

	// 自动重启核心使新规则文件生效
	if s.manager != nil {
		_ = s.manager.RestartService(ctx)
	}

	return nil
}

func downloadFile(ctx context.Context, client *http.Client, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	f.Close()

	return os.Rename(tmpPath, destPath)
}
