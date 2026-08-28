package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	manager     *xray.Manager
	progressMu  sync.RWMutex
	progress    GeoDataProgress
}

func NewGeoDataService(xrayBinPath string, manager *xray.Manager) *GeoDataService {
	return &GeoDataService{
		xrayBinPath: xrayBinPath,
		manager:     manager,
		progress: GeoDataProgress{
			Step:       "idle",
			Percentage: 0,
			Message:    "空闲",
		},
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
	dataDir := s.getGeoDataDir()
	_ = os.MkdirAll(dataDir, 0755)

	client := &http.Client{Timeout: 120 * time.Second}

	// 1. 下载 geoip.dat (0% -> 48%)
	s.setProgress(GeoDataProgress{
		IsUpdating: true,
		Step:       "downloading_geoip",
		Percentage: 10,
		Message:    "正在下载 geoip.dat (IP 归属地规则)...",
	})

	geoipURLs := []string{
		"https://github.com/v2fly/geoip/releases/latest/download/geoip.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat",
	}

	var geoipErr error
	for _, u := range geoipURLs {
		geoipErr = s.downloadWithProgress(client, u, filepath.Join(dataDir, "geoip.dat"), 10, 48, "geoip.dat")
		if geoipErr == nil {
			break
		}
	}
	if geoipErr != nil {
		s.setProgress(GeoDataProgress{
			IsUpdating: false,
			Step:       "error",
			Percentage: 0,
			Message:    "下载 GeoIP 失败: " + geoipErr.Error(),
			Error:      geoipErr.Error(),
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

	geositeURLs := []string{
		"https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat",
		"https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat",
	}

	var geositeErr error
	for _, u := range geositeURLs {
		geositeErr = s.downloadWithProgress(client, u, filepath.Join(dataDir, "geosite.dat"), 50, 95, "geosite.dat")
		if geositeErr == nil {
			break
		}
	}
	if geositeErr != nil {
		s.setProgress(GeoDataProgress{
			IsUpdating: false,
			Step:       "error",
			Percentage: 0,
			Message:    "下载 GeoSite 失败: " + geositeErr.Error(),
			Error:      geositeErr.Error(),
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.manager.RestartService(ctx)
		cancel()
	}

	time.Sleep(500 * time.Millisecond)

	s.setProgress(GeoDataProgress{
		IsUpdating: false,
		Step:       "done",
		Percentage: 100,
		Message:    "GeoData 规则库已成功更新并热重载生效！",
	})
}

type progressWriter struct {
	total       int64
	downloaded  int64
	lastTime    time.Time
	lastBytes   int64
	speedBps    int64
	onProgress  func(downloaded, total, speed int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)

	now := time.Now()
	dur := now.Sub(pw.lastTime)
	if dur >= 300*time.Millisecond {
		delta := pw.downloaded - pw.lastBytes
		if dur.Seconds() > 0 {
			pw.speedBps = int64(float64(delta) / dur.Seconds())
		}
		pw.lastTime = now
		pw.lastBytes = pw.downloaded
		if pw.onProgress != nil {
			pw.onProgress(pw.downloaded, pw.total, pw.speedBps)
		}
	}
	return n, nil
}

func (s *GeoDataService) downloadWithProgress(client *http.Client, url, destPath string, startPct, endPct int, name string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Xray-Panel-GeoUpdater/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	totalSize := resp.ContentLength
	pw := &progressWriter{
		total:     totalSize,
		lastTime:  time.Now(),
		onProgress: func(downloaded, total, speed int64) {
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
			if speed > 0 {
				msg += fmt.Sprintf(", %.1f MB/s", float64(speed)/(1024*1024))
			}
			msg += ")..."

			s.setProgress(GeoDataProgress{
				IsUpdating:     true,
				Step:           "downloading_" + name,
				Percentage:     pct,
				Message:        msg,
				DownloadedSize: downloaded,
				TotalSize:      total,
				SpeedBps:       speed,
			})
		},
	}

	if _, err := io.Copy(f, io.TeeReader(resp.Body, pw)); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	f.Close()

	return os.Rename(tmpPath, destPath)
}
