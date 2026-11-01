package sub

import (
	"fmt"
	"strings"

	"panel/internal/protocol"

	"gopkg.in/yaml.v3"
)

// ClashConfig Clash / Mihomo 标准配置结构
type ClashConfig struct {
	Port        int                      `yaml:"port,omitempty"`
	SocksPort   int                      `yaml:"socks-port,omitempty"`
	MixedPort   int                      `yaml:"mixed-port,omitempty"`
	AllowLan    bool                     `yaml:"allow-lan"`
	Mode        string                   `yaml:"mode"`
	LogLevel    string                   `yaml:"log-level"`
	Proxies     []map[string]interface{} `yaml:"proxies"`
	ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	Rules       []string                 `yaml:"rules"`
}

// ClashParams 定义 Clash 导出可定制参数
type ClashParams struct {
	MixedPort int    `yaml:"mixed-port"`
	AllowLan  bool   `yaml:"allow-lan"`
	Mode      string `yaml:"mode"`
}

// ExportClashWithParams 将多节点配置转换为带定制参数的 Clash / Mihomo YAML 订阅
func ExportClashWithParams(nodes []*protocol.NodeConfig, params ClashParams) (string, error) {
	proxies := make([]map[string]interface{}, 0, len(nodes))
	proxyNames := make([]string, 0, len(nodes))

	for idx, node := range nodes {
		if node == nil {
			continue
		}
		name := node.Name
		if name == "" {
			name = fmt.Sprintf("Node-%d", idx+1)
		}

		proxyMap, err := protocol.ToClash(node)
		if err != nil {
			return "", fmt.Errorf("failed to convert node %s to clash proxy: %w", name, err)
		}
		if proxyMap != nil {
			proxyMap["name"] = name
			proxies = append(proxies, proxyMap)
			proxyNames = append(proxyNames, name)
		}
	}

	fallbackProxies := proxyNames
	if len(fallbackProxies) == 0 {
		fallbackProxies = []string{"DIRECT"}
	}

	mixedPort := params.MixedPort
	if mixedPort <= 0 {
		mixedPort = 7890
	}
	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "rule"
	}

	cfg := ClashConfig{
		Port:      mixedPort,
		SocksPort: mixedPort + 1,
		MixedPort: mixedPort,
		AllowLan:  params.AllowLan,
		Mode:      mode,
		LogLevel:  "info",
		Proxies:   proxies,
		ProxyGroups: []map[string]interface{}{
			{
				"name":    "PROXIES",
				"type":    "select",
				"proxies": fallbackProxies,
			},
			{
				"name":     "AUTO",
				"type":     "url-test",
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 300,
				"proxies":  fallbackProxies,
			},
		},
		Rules: []string{
			"MATCH,PROXIES",
		},
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal clash yaml: %w", err)
	}

	return string(data), nil
}

// ExportClash 将多节点配置转换为 Clash / Mihomo YAML 订阅 (基于 protocol.Registry 策略模式)
func ExportClash(nodes []*protocol.NodeConfig) (string, error) {
	return ExportClashWithParams(nodes, ClashParams{
		MixedPort: 7890,
		AllowLan:  false,
		Mode:      "rule",
	})
}
