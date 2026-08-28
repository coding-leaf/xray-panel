package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"panel/internal/domain"
	"panel/internal/pkg/jsonc"
	"panel/internal/pkg/logger"
)

type ConfigManager struct {
	configPath  string
	xrayBinPath string
}

func NewConfigManager(configPath, xrayBinPath string) *ConfigManager {
	return &ConfigManager{
		configPath:  configPath,
		xrayBinPath: xrayBinPath,
	}
}

func (c *ConfigManager) UpdateConfig(configPath, xrayBinPath string) {
	if configPath != "" {
		c.configPath = configPath
	}
	if xrayBinPath != "" {
		c.xrayBinPath = xrayBinPath
	}
}

// ReadRawConfig 读取当前原始 JSON 配置
func (c *ConfigManager) ReadRawConfig() ([]byte, error) {
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

	// 4. 写入临时文件并通过 xray -test 进行严格语法校验
	if c.xrayBinPath != "" {
		if _, err := os.Stat(c.xrayBinPath); err == nil {
			tmpDir := os.TempDir()
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("xray_test_%d.json", os.Getpid()))
			if err := os.WriteFile(tmpFile, cleanedJSON, 0600); err != nil {
				return fmt.Errorf("write temp config failed: %w", err)
			}
			defer os.Remove(tmpFile)

			cmd := exec.CommandContext(ctx, c.xrayBinPath, "-test", "-config", tmpFile)
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

// WriteConfig 写入经过清洗与校验的配置
func (c *ConfigManager) WriteConfig(ctx context.Context, rawJSON []byte) error {
	if err := c.ValidateConfig(ctx, rawJSON); err != nil {
		return err
	}

	cleanedJSON := jsonc.StripJSONC(rawJSON)

	// 格式化美化 JSON
	var buf bytes.Buffer
	if err := json.Indent(&buf, cleanedJSON, "", "    "); err == nil {
		cleanedJSON = buf.Bytes()
	}

	// 备份旧配置
	if _, err := os.Stat(c.configPath); err == nil {
		backupPath := c.configPath + ".bak"
		_ = os.WriteFile(backupPath, cleanedJSON, 0644)
	}

	return os.WriteFile(c.configPath, cleanedJSON, 0644)
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

// BuildShareLink 生成标准 Xray 分享链接 (vless://, vmess://, trojan://)
func BuildShareLink(inbound *domain.Inbound, user *domain.User, hostDomain string) string {
	if hostDomain == "" {
		hostDomain = inbound.Listen
		if hostDomain == "0.0.0.0" || hostDomain == "127.0.0.1" || hostDomain == "" {
			hostDomain = "example.com"
		}
	}

	var streamMap map[string]interface{}
	_ = json.Unmarshal([]byte(inbound.StreamSettings), &streamMap)

	network := "tcp"
	if netVal, ok := streamMap["network"].(string); ok && netVal != "" {
		network = netVal
	}

	security := "none"
	if secVal, ok := streamMap["security"].(string); ok && secVal != "" {
		security = secVal
	}

	switch strings.ToLower(inbound.Protocol) {
	case "vless":
		v := url.Values{}
		v.Set("type", network)
		v.Set("security", security)
		flow := ""
		if network == "tcp" && (security == "reality" || security == "tls") {
			var settingsMap map[string]interface{}
			_ = json.Unmarshal([]byte(inbound.SettingsJSON), &settingsMap)
			if f, ok := settingsMap["flow"].(string); ok && f != "" {
				flow = f
			} else if user.Flow != "" {
				flow = user.Flow
			} else {
				flow = "xtls-rprx-vision"
			}
		}
		if flow != "" {
			v.Set("flow", flow)
		}

		if security == "reality" {
			if realitySettings, ok := streamMap["realitySettings"].(map[string]interface{}); ok {
				if pbk, ok := realitySettings["publicKey"].(string); ok {
					v.Set("pbk", pbk)
				}
				if serverNames, ok := realitySettings["serverNames"].([]interface{}); ok && len(serverNames) > 0 {
					v.Set("sni", fmt.Sprintf("%v", serverNames[0]))
				}
				if shortIds, ok := realitySettings["shortIds"].([]interface{}); ok && len(shortIds) > 0 {
					v.Set("sid", fmt.Sprintf("%v", shortIds[0]))
				}
			}
		}

		if network == "xhttp" {
			if xhttpSettings, ok := streamMap["xhttpSettings"].(map[string]interface{}); ok {
				if path, ok := xhttpSettings["path"].(string); ok {
					v.Set("path", path)
				}
				if mode, ok := xhttpSettings["mode"].(string); ok {
					v.Set("mode", mode)
				}
			}
		} else if network == "ws" {
			if wsSettings, ok := streamMap["wsSettings"].(map[string]interface{}); ok {
				if path, ok := wsSettings["path"].(string); ok {
					v.Set("path", path)
				}
			}
		} else if network == "grpc" {
			if grpcSettings, ok := streamMap["grpcSettings"].(map[string]interface{}); ok {
				if serviceName, ok := grpcSettings["serviceName"].(string); ok {
					v.Set("serviceName", serviceName)
				}
			}
		}

		remark := inbound.Remark
		if remark == "" {
			remark = inbound.Tag
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", user.UUID, hostDomain, inbound.Port, v.Encode(), url.QueryEscape(remark))

	case "trojan":
		v := url.Values{}
		v.Set("type", network)
		v.Set("security", security)
		remark := inbound.Remark
		if remark == "" {
			remark = inbound.Tag
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", user.UUID, hostDomain, inbound.Port, v.Encode(), url.QueryEscape(remark))

	default:
		return ""
	}
}
