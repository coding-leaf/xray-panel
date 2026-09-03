package xray

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
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

func cleanHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if u, err := url.Parse(raw); err == nil {
			return u.Hostname()
		}
	}
	if strings.Contains(raw, ":") && !strings.Contains(raw, "]") {
		return strings.Split(raw, ":")[0]
	}
	return raw
}

// ApplyVlessRouteToUUID 纯函数：将用户基准 UUID 的第 3 组（bytes 7&8）替换为 routeID 的 4 位十六进制格式
// 若 routeID 为 0 或 rawUUID 格式不符合标准 5 段格式，则返回原始 UUID 不作修改
func ApplyVlessRouteToUUID(rawUUID string, routeID uint16) string {
	if routeID == 0 || rawUUID == "" {
		return rawUUID
	}
	parts := strings.Split(rawUUID, "-")
	if len(parts) != 5 {
		return rawUUID
	}
	parts[2] = fmt.Sprintf("%04x", routeID)
	return strings.Join(parts, "-")
}

// BuildShareLink 生成标准 Xray 分享链接 (vless://, vmess://, trojan://)
func BuildShareLink(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) string {
	targetHost := cleanHost(inbound.ExternalHost)
	if targetHost == "" {
		targetHost = cleanHost(hostDomain)
	}
	if targetHost == "" {
		targetHost = inbound.Listen
		if targetHost == "0.0.0.0" || targetHost == "" {
			targetHost = "127.0.0.1"
		}
	}

	targetPort := inbound.Port
	if inbound.ExternalPort > 0 {
		targetPort = inbound.ExternalPort
	} else if defaultPort > 0 {
		targetPort = defaultPort
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

	// 提取 TLS / REALITY 关键配置
	tlsSNI := ""
	tlsALPN := ""
	if tlsSettings, ok := streamMap["tlsSettings"].(map[string]interface{}); ok {
		if serverName, ok := tlsSettings["serverName"].(string); ok && serverName != "" {
			tlsSNI = serverName
		}
		if alpnArr, ok := tlsSettings["alpn"].([]interface{}); ok && len(alpnArr) > 0 {
			var alpns []string
			for _, a := range alpnArr {
				alpns = append(alpns, fmt.Sprintf("%v", a))
			}
			tlsALPN = strings.Join(alpns, ",")
		}
	}

	realitySNI := ""
	realityShortID := ""
	realityPBK := ""
	realitySPX := ""
	realityFP := "chrome"
	if realitySettings, ok := streamMap["realitySettings"].(map[string]interface{}); ok {
		if pbk, ok := realitySettings["publicKey"].(string); ok && pbk != "" {
			realityPBK = pbk
		} else if privKey, ok := realitySettings["privateKey"].(string); ok && privKey != "" {
			realityPBK = DerivePublicKeyFromPrivate(privKey)
		}

		// 自适应单数 serverName 与复数 serverNames
		if sn, ok := realitySettings["serverName"].(string); ok && sn != "" {
			realitySNI = sn
		} else if serverNames, ok := realitySettings["serverNames"].([]interface{}); ok && len(serverNames) > 0 {
			realitySNI = fmt.Sprintf("%v", serverNames[0])
		}

		// 自适应单数 shortId 与复数 shortIds
		if sid, ok := realitySettings["shortId"].(string); ok && sid != "" {
			realityShortID = sid
		} else if shortIds, ok := realitySettings["shortIds"].([]interface{}); ok && len(shortIds) > 0 {
			for _, sidRaw := range shortIds {
				sidStr := fmt.Sprintf("%v", sidRaw)
				if sidStr != "" {
					realityShortID = sidStr
					break
				}
			}
		}

		if spx, ok := realitySettings["spiderX"].(string); ok && spx != "" {
			realitySPX = spx
		}
		if fp, ok := realitySettings["fingerprint"].(string); ok && fp != "" {
			realityFP = fp
		}
	}

	// 提取 WebSocket / gRPC / xHTTP 传输层参数
	wsPath := ""
	wsHost := ""
	if wsSettings, ok := streamMap["wsSettings"].(map[string]interface{}); ok {
		if path, ok := wsSettings["path"].(string); ok && path != "" {
			wsPath = path
		}
		if headers, ok := wsSettings["headers"].(map[string]interface{}); ok {
			if h, ok := headers["Host"].(string); ok && h != "" {
				wsHost = h
			} else if h, ok := headers["host"].(string); ok && h != "" {
				wsHost = h
			}
		}
		if wsHost == "" {
			if h, ok := wsSettings["host"].(string); ok && h != "" {
				wsHost = h
			}
		}
	}

	grpcServiceName := ""
	if grpcSettings, ok := streamMap["grpcSettings"].(map[string]interface{}); ok {
		if serviceName, ok := grpcSettings["serviceName"].(string); ok && serviceName != "" {
			grpcServiceName = serviceName
		}
	}

	xhttpPath := ""
	xhttpMode := ""
	if xhttpSettings, ok := streamMap["xhttpSettings"].(map[string]interface{}); ok {
		if p, ok := xhttpSettings["path"].(string); ok && p != "" {
			xhttpPath = p
		}
		if m, ok := xhttpSettings["mode"].(string); ok && m != "" {
			xhttpMode = m
		}
	}

	remark := inbound.Remark
	if remark == "" {
		remark = inbound.Tag
	}

	switch strings.ToLower(inbound.Protocol) {
	case "vless":
		v := url.Values{}
		v.Set("type", network)
		v.Set("security", security)
		v.Set("encryption", "none")

		if security == "reality" {
			v.Set("fp", realityFP)
			if realityPBK != "" {
				v.Set("pbk", realityPBK)
			}
			if realitySNI != "" {
				v.Set("sni", realitySNI)
			}
			if realityShortID != "" {
				v.Set("sid", realityShortID)
			}
			if realitySPX != "" {
				v.Set("spx", realitySPX)
			}
		} else if security == "tls" {
			v.Set("fp", "chrome")
			if tlsSNI != "" {
				v.Set("sni", tlsSNI)
			}
			if tlsALPN != "" {
				v.Set("alpn", tlsALPN)
			}
		}

		// TCP 下的 flow 属性 (xtls-rprx-vision)
		if network == "tcp" && (security == "reality" || security == "tls") {
			var settingsMap map[string]interface{}
			_ = json.Unmarshal([]byte(inbound.SettingsJSON), &settingsMap)
			flow := ""
			if f, ok := settingsMap["flow"].(string); ok && f != "" {
				flow = f
			} else if user.Flow != "" {
				flow = user.Flow
			} else {
				flow = "xtls-rprx-vision"
			}
			if flow != "" {
				v.Set("flow", flow)
			}
		}

		if network == "xhttp" {
			if xhttpPath != "" {
				v.Set("path", xhttpPath)
			}
			if xhttpMode != "" {
				v.Set("mode", xhttpMode)
			}
		} else if network == "ws" {
			if wsPath != "" {
				v.Set("path", wsPath)
			}
			if wsHost != "" {
				v.Set("host", wsHost)
			}
		} else if network == "grpc" {
			if grpcServiceName != "" {
				v.Set("serviceName", grpcServiceName)
			}
		}

		effectiveUUID := user.UUID
		if inbound.RouteID > 0 {
			effectiveUUID = ApplyVlessRouteToUUID(user.UUID, inbound.RouteID)
		}
		return fmt.Sprintf("vless://%s@%s:%d?%s#%s", effectiveUUID, targetHost, targetPort, v.Encode(), url.QueryEscape(remark))

	case "trojan":
		v := url.Values{}
		v.Set("type", network)
		v.Set("security", security)
		if security == "tls" {
			if tlsSNI != "" {
				v.Set("sni", tlsSNI)
			}
			if tlsALPN != "" {
				v.Set("alpn", tlsALPN)
			}
		}
		if network == "ws" {
			if wsPath != "" {
				v.Set("path", wsPath)
			}
			if wsHost != "" {
				v.Set("host", wsHost)
			}
		} else if network == "grpc" {
			if grpcServiceName != "" {
				v.Set("serviceName", grpcServiceName)
			}
		}
		return fmt.Sprintf("trojan://%s@%s:%d?%s#%s", user.UUID, targetHost, targetPort, v.Encode(), url.QueryEscape(remark))

	case "shadowsocks":
		var settingsMap map[string]interface{}
		_ = json.Unmarshal([]byte(inbound.SettingsJSON), &settingsMap)
		method, _ := settingsMap["method"].(string)
		if method == "" {
			method = "aes-128-gcm"
		}
		auth := fmt.Sprintf("%s:%s", method, user.UUID)
		encodedAuth := base64.URLEncoding.EncodeToString([]byte(auth))
		return fmt.Sprintf("ss://%s@%s:%d#%s", encodedAuth, targetHost, targetPort, url.QueryEscape(remark))

	case "vmess":
		vmessObj := map[string]interface{}{
			"v":    "2",
			"ps":   remark,
			"add":  targetHost,
			"port": targetPort,
			"id":   user.UUID,
			"aid":  0,
			"net":  network,
			"type": "none",
			"tls":  security,
		}
		if security == "tls" && tlsSNI != "" {
			vmessObj["sni"] = tlsSNI
		}
		if network == "ws" {
			if wsPath != "" {
				vmessObj["path"] = wsPath
			}
			if wsHost != "" {
				vmessObj["host"] = wsHost
			}
		} else if network == "grpc" {
			if grpcServiceName != "" {
				vmessObj["path"] = grpcServiceName
			}
		}
		rawJSON, _ := json.Marshal(vmessObj)
		return "vmess://" + base64.StdEncoding.EncodeToString(rawJSON)

	default:
		return ""
	}
}

// BuildShareLinksForInbound 为单个入站生成一组分享链接 (若配置了 SubRoutes 则导出所有启用的分流线路，否则导出单节点)
func BuildShareLinksForInbound(inbound *domain.Inbound, user *domain.User, hostDomain string, defaultPort int) []string {
	subRoutes := inbound.GetSubRoutes()
	if len(subRoutes) == 0 {
		link := BuildShareLink(inbound, user, hostDomain, defaultPort)
		if link != "" {
			return []string{link}
		}
		return nil
	}

	var links []string
	for _, sr := range subRoutes {
		if !sr.Enabled {
			continue
		}
		tempInbound := *inbound
		if sr.Name != "" {
			tempInbound.Remark = sr.Name
		}
		tempInbound.RouteID = sr.RouteID
		link := BuildShareLink(&tempInbound, user, hostDomain, defaultPort)
		if link != "" {
			links = append(links, link)
		}
	}
	return links
}
