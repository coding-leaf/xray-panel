package sub

import (
	"fmt"

	"panel/internal/protocol"

	"gopkg.in/yaml.v2"
)

// ClashConfig Clash / Mihomo 标准配置结构
type ClashConfig struct {
	Port        int                      `yaml:"port"`
	SocksPort   int                      `yaml:"socks-port"`
	AllowLan    bool                     `yaml:"allow-lan"`
	Mode        string                   `yaml:"mode"`
	LogLevel    string                   `yaml:"log-level"`
	Proxies     []map[string]interface{} `yaml:"proxies"`
	ProxyGroups []map[string]interface{} `yaml:"proxy-groups"`
	Rules       []string                 `yaml:"rules"`
}

// ExportClash 将多节点配置转换为 Clash / Mihomo YAML 订阅 (基于 protocol.Registry 策略模式)
func ExportClash(nodes []*protocol.NodeConfig) (string, error) {
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

	cfg := ClashConfig{
		Port:      7890,
		SocksPort: 7891,
		AllowLan:  false,
		Mode:      "rule",
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
