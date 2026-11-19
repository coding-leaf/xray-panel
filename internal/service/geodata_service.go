package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"panel/internal/domain"
	"panel/internal/pkg/geodata"
)

const (
	defaultGeoDataHTTPTimeout = 180 * time.Second
	defaultGeoDataJobTimeout  = 6 * time.Minute
)

type CoreController interface {
	GetVersion(ctx context.Context) (string, error)
	RestartService(ctx context.Context) error
}

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

type GeoDataProgress struct {
	IsUpdating     bool   `json:"isUpdating"`
	Step           string `json:"step"` // "idle", "downloading_geoip", "downloading_geosite", "restarting_core", "done", "error"
	Percentage     int    `json:"percentage"`
	Message        string `json:"message"`
	DownloadedSize int64  `json:"downloadedSize"`
	TotalSize      int64  `json:"totalSize"`
	SpeedBps       int64  `json:"speedBps"`
	Error          string `json:"error,omitempty"`
}

type GeoDataService struct {
	xrayBinPath string
	manager     CoreController
	settingRepo domain.SettingRepository
	progressMu  sync.RWMutex
	progress    GeoDataProgress
}

func NewGeoDataService(xrayBinPath string, manager CoreController, settingRepo domain.SettingRepository) *GeoDataService {
	return &GeoDataService{
		xrayBinPath: xrayBinPath,
		manager:     manager,
		settingRepo: settingRepo,
		progress: GeoDataProgress{
			Step:       "idle",
			Percentage: 0,
			Message:    "空闲",
		},
	}
}

func (s *GeoDataService) getGeoDataDir(ctx context.Context) string {
	if s.settingRepo != nil && ctx != nil {
		if customDir, err := s.settingRepo.Get(ctx, "xray_geodata_dir"); err == nil {
			customDir = strings.TrimSpace(customDir)
			if customDir != "" {
				return customDir
			}
		}
	}
	dir := filepath.Dir(s.xrayBinPath)
	if runtime.GOOS == "linux" {
		if _, err := os.Stat(geodata.DefaultAssetsDir); err == nil {
			return geodata.DefaultAssetsDir
		}
	}
	return dir
}

func (s *GeoDataService) GetStatus(ctx context.Context) (*GeoDataStatus, error) {
	dataDir := s.getGeoDataDir(ctx)
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

func (s *GeoDataService) GetProgress() GeoDataProgress {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	return s.progress
}

func (s *GeoDataService) setProgress(p GeoDataProgress) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	s.progress = p
}

// TriggerUpdate 异步触发更新任务
func (s *GeoDataService) TriggerUpdate() error {
	s.progressMu.Lock()
	if s.progress.IsUpdating {
		s.progressMu.Unlock()
		return fmt.Errorf("规则库正在更新中，请稍候...")
	}
	s.progress = GeoDataProgress{
		IsUpdating: true,
		Step:       "downloading_geoip",
		Percentage: 5,
		Message:    "正在连接镜像源并准备下载 GeoIP 规则库...",
	}
	s.progressMu.Unlock()

	go s.runUpdateWorker()
	return nil
}

func (s *GeoDataService) runUpdateWorker() {
	ctx, cancel := context.WithTimeout(context.Background(), defaultGeoDataJobTimeout)
	defer cancel()

	dataDir := s.getGeoDataDir(ctx)
	_ = os.MkdirAll(dataDir, 0o755)

	client := geodata.NewHTTPClient(defaultGeoDataHTTPTimeout)

	// 1. 下载 geoip.dat (0% -> 48%)
	s.setProgress(GeoDataProgress{
		IsUpdating: true,
		Step:       "downloading_geoip",
		Percentage: 10,
		Message:    "正在下载 geoip.dat (IP 归属地规则)...",
	})

	if err := s.downloadWithProgress(ctx, client, geodata.DefaultGeoIPURLs, filepath.Join(dataDir, "geoip.dat"), 10, 48, "geoip.dat"); err != nil {
		s.setProgress(GeoDataProgress{
			IsUpdating: false,
			Step:       "error",
			Percentage: 0,
			Message:    "下载 GeoIP 失败: " + err.Error(),
			Error:      err.Error(),
		})
		return
	}

	// 2. 下载 geosite.dat (48% -> 95%)
	s.setProgress(GeoDataProgress{
		IsUpdating: true,
		Step:       "downloading_geosite",
		Percentage: 50,
		Message:    "正在下载 geosite.dat (域名分类规则)...",
	})

	if err := s.downloadWithProgress(ctx, client, geodata.DefaultGeoSiteURLs, filepath.Join(dataDir, "geosite.dat"), 50, 95, "geosite.dat"); err != nil {
		s.setProgress(GeoDataProgress{
			IsUpdating: false,
			Step:       "error",
			Percentage: 0,
			Message:    "下载 GeoSite 失败: " + err.Error(),
			Error:      err.Error(),
		})
		return
	}

	// 3. 重启核心使新规则生效 (95% -> 100%)
	s.setProgress(GeoDataProgress{
		IsUpdating: true,
		Step:       "restarting_core",
		Percentage: 96,
		Message:    "规则库下载完成，正在平滑重载 Xray 核心...",
	})

	if s.manager != nil {
		restartCtx, restartCancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.manager.RestartService(restartCtx)
		restartCancel()
	}

	time.Sleep(500 * time.Millisecond)

	s.setProgress(GeoDataProgress{
		IsUpdating: false,
		Step:       "done",
		Percentage: 100,
		Message:    "GeoData 规则库已成功更新并热重载生效！",
	})
}

func (s *GeoDataService) downloadWithProgress(
	ctx context.Context,
	client *http.Client,
	urls []string,
	destPath string,
	startPct, endPct int,
	name string,
) error {
	tmpPath := destPath + ".tmp"
	lastTime := time.Now()
	var lastBytes int64
	var speedBps int64

	onProgress := func(downloaded, total int64) {
		now := time.Now()
		dur := now.Sub(lastTime)
		if dur < 300*time.Millisecond {
			return
		}
		delta := downloaded - lastBytes
		if delta < 0 {
			// Mirror fallback restarted download from byte 0.
			delta = downloaded
		}
		if dur.Seconds() > 0 {
			speedBps = int64(float64(delta) / dur.Seconds())
		}
		lastTime = now
		lastBytes = downloaded

		pct := startPct
		if total > 0 {
			progressRatio := float64(downloaded) / float64(total)
			pct = startPct + int(progressRatio*float64(endPct-startPct))
		} else {
			// 未知长度时平滑递增
			pct = startPct + int(float64(downloaded)/(1024*1024*20)*float64(endPct-startPct))
		}
		if pct > endPct {
			pct = endPct
		}

		msg := fmt.Sprintf("正在下载 %s (%.1f MB", name, float64(downloaded)/(1024*1024))
		if total > 0 {
			msg += fmt.Sprintf(" / %.1f MB", float64(total)/(1024*1024))
		}
		if speedBps > 0 {
			msg += fmt.Sprintf(", %.1f MB/s", float64(speedBps)/(1024*1024))
		}
		msg += ")..."

		s.setProgress(GeoDataProgress{
			IsUpdating:     true,
			Step:           "downloading_" + name,
			Percentage:     pct,
			Message:        msg,
			DownloadedSize: downloaded,
			TotalSize:      total,
			SpeedBps:       speedBps,
		})
	}

	if _, err := geodata.DownloadWithFallback(ctx, client, urls, tmpPath, geodata.DefaultMinFileSize, onProgress); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
