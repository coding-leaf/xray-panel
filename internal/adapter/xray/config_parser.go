package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"panel/internal/domain"
	"panel/internal/pkg/jsonc"
	"panel/internal/pkg/logger"
)

type ConfigManager struct {
	configPath  string
	xrayBinPath string
	mu          sync.RWMutex
}

func NewConfigManager(configPath, xrayBinPath string) *ConfigManager {
	return &ConfigManager{
		configPath:  configPath,
		xrayBinPath: xrayBinPath,
	}
}

func (c *ConfigManager) UpdateConfig(configPath, xrayBinPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if configPath != "" {
		c.configPath = configPath
	}
	if xrayBinPath != "" {
		c.xrayBinPath = xrayBinPath
	}
}

// ReadRawConfig 读取当前原始 JSON 配置
func (c *ConfigManager) ReadRawConfig() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: config file not found at %s", domain.ErrNotFound, c.configPath)
	}
	return os.ReadFile(c.configPath)
}

// ValidateConfig 使用 jsonc 清洗与 xray -test -config 校验配置合法性
func (c *ConfigManager) ValidateConfig(ctx context.Context, rawJSON []byte) error {
	// 1. 先进行 JSONC 注释清洗
	cleanedJSON := jsonc.StripJSONC(rawJSON)

	// 2. JSON 基础语法校验
	var js map[string]interface{}
	if err := json.Unmarshal(cleanedJSON, &js); err != nil {
		return fmt.Errorf("%w: JSON syntax error: %v", domain.ErrInvalidConfig, err)
	}

	// 3. 检查 api 配置是否存在
	apiObj, ok := js["api"].(map[string]interface{})
	if !ok || apiObj["tag"] == nil {
		logger.FromContext(ctx).Warn("Xray config missing 'api' section or tag")
	}

	// 4. 写入隔离临时文件并通过 xray -test 进行严格语法校验 (避免并发 PID 冲突)
	if c.xrayBinPath != "" {
		if _, err := os.Stat(c.xrayBinPath); err == nil {
			tmpFile, err := os.CreateTemp("", "xray_test_*.json")
			if err != nil {
				return fmt.Errorf("create temp config failed: %w", err)
			}
			tmpFilePath := tmpFile.Name()
			defer os.Remove(tmpFilePath)

			if _, err := tmpFile.Write(cleanedJSON); err != nil {
				_ = tmpFile.Close()
				return fmt.Errorf("write temp config failed: %w", err)
			}
			_ = tmpFile.Close()

			cmd := exec.CommandContext(ctx, c.xrayBinPath, "-test", "-config", tmpFilePath)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				errMsg := strings.TrimSpace(stderr.String())
				if errMsg == "" {
					errMsg = strings.TrimSpace(stdout.String())
				}
				return fmt.Errorf("%w: xray validation failed: %s", domain.ErrInvalidConfig, errMsg)
			}
		}
	}

	return nil
}

// WriteConfig 写入经过清洗与校验的配置 (带读写锁互斥与 POSIX 同目录原子替换)
func (c *ConfigManager) WriteConfig(ctx context.Context, rawJSON []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ValidateConfig(ctx, rawJSON); err != nil {
		return err
	}

	cleanedJSON := jsonc.StripJSONC(rawJSON)

	// 格式化美化 JSON
	var buf bytes.Buffer
	if err := json.Indent(&buf, cleanedJSON, "", "    "); err == nil {
		cleanedJSON = buf.Bytes()
	}

	// 确保目标目录存在
	dir := filepath.Dir(c.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir failed: %w", err)
	}

	// 备份现有旧配置
	if oldRaw, err := os.ReadFile(c.configPath); err == nil && len(oldRaw) > 0 {
		backupPath := c.configPath + ".bak"
		_ = os.WriteFile(backupPath, oldRaw, 0644)
	}

	// 采用同目录临时文件 + sync + POSIX atomic rename，保证并发读绝不读到 0 字节截断
	tmpFile, err := os.CreateTemp(dir, ".xray_config_*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config failed: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(cleanedJSON); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp config failed: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp config failed: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp config failed: %w", err)
	}

	_ = os.Chmod(tmpPath, 0644)
	if err := os.Rename(tmpPath, c.configPath); err != nil {
		return fmt.Errorf("atomic rename config failed: %w", err)
	}

	return nil
}

// GetLogPaths 从配置文件中获取 access 和 error 日志路径
func (c *ConfigManager) GetLogPaths() (accessLog, errorLog string) {
	raw, err := c.ReadRawConfig()
	if err != nil {
		return "", ""
	}
	cleaned := jsonc.StripJSONC(raw)
	var js struct {
		Log struct {
			Access string `json:"access"`
			Error  string `json:"error"`
		} `json:"log"`
	}
	if err := json.Unmarshal(cleaned, &js); err == nil {
		return js.Log.Access, js.Log.Error
	}
	return "", ""
}

func (c *ConfigManager) ReadLastLinesFiltered(filePath string, maxLines int, filter domain.LogFilter) ([]string, error) {
	return ReadLastLinesFiltered(filePath, maxLines, filter)
}

func (c *ConfigManager) ParseAccessLog(line string) *domain.AccessLogEntry {
	return ParseAccessLogLine(line)
}

func (c *ConfigManager) ParseErrorLog(line string) *domain.ErrorLogEntry {
	return ParseErrorLogLine(line)
}

// GetCertificatePaths 获取配置中引用的所有 TLS 证书路径
func (c *ConfigManager) GetCertificatePaths() []string {
	raw, err := c.ReadRawConfig()
	if err != nil {
		return nil
	}
	cleaned := jsonc.StripJSONC(raw)
	var js struct {
		Inbounds []struct {
			StreamSettings struct {
				TLSSettings struct {
					Certificates []struct {
						CertificateFile string `json:"certificateFile"`
					} `json:"certificates"`
				} `json:"tlsSettings"`
			} `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cleaned, &js); err != nil {
		return nil
	}

	var paths []string
	seen := make(map[string]bool)
	for _, inb := range js.Inbounds {
		for _, cert := range inb.StreamSettings.TLSSettings.Certificates {
			if cert.CertificateFile != "" && !seen[cert.CertificateFile] {
				seen[cert.CertificateFile] = true
				paths = append(paths, cert.CertificateFile)
			}
		}
	}
	return paths
}
