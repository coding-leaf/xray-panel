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

// ReadRawConfig 读取当前原始 JSON 配置
func (c *ConfigManager) ReadRawConfig() ([]byte, error) {
	if _, err := os.Stat(c.configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: config file not found at %s", domain.ErrNotFound, c.configPath)
	}
	return os.ReadFile(c.configPath)
}

// ValidateConfig 使用 xray -test -config 校验配置合法性
func (c *ConfigManager) ValidateConfig(ctx context.Context, rawJSON []byte) error {
	// 1. JSON 基础语法校验
	var js map[string]interface{}
	if err := json.Unmarshal(rawJSON, &js); err != nil {
		return fmt.Errorf("%w: JSON syntax error: %v", domain.ErrInvalidConfig, err)
	}

	// 2. 检查 api 配置是否存在
	apiObj, ok := js["api"].(map[string]interface{})
	if !ok || apiObj["tag"] == nil {
		logger.FromContext(ctx).Warn("Xray config missing 'api' section or tag")
	}

	// 3. 写入临时文件并通过 xray -test 进行严格语法校验
	if c.xrayBinPath != "" {
		if _, err := os.Stat(c.xrayBinPath); err == nil {
			tmpDir := os.TempDir()
			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("xray_test_%d.json", os.Getpid()))
			if err := os.WriteFile(tmpFile, rawJSON, 0600); err != nil {
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

// WriteConfig 写入经过校验的配置
func (c *ConfigManager) WriteConfig(ctx context.Context, rawJSON []byte) error {
	if err := c.ValidateConfig(ctx, rawJSON); err != nil {
		return err
	}

	// 格式化美化 JSON
	var buf bytes.Buffer
	if err := json.Indent(&buf, rawJSON, "", "    "); err == nil {
		rawJSON = buf.Bytes()
	}

	// 备份旧配置
	if _, err := os.Stat(c.configPath); err == nil {
		backupPath := c.configPath + ".bak"
		_ = os.WriteFile(backupPath, rawJSON, 0644)
	}

	return os.WriteFile(c.configPath, rawJSON, 0644)
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
		if user.Flow != "" {
			v.Set("flow", user.Flow)
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
