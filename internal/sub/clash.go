package sub

import (
	"fmt"
	"log/slog"
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

const clashEmptyNotice = "# Notice: No Clash-compatible proxies available (latest Xray protocols like XHTTP/SplitHTTP are not supported by Clash). Please use v2rayN, Shadowrocket, Happ, or sing-box.\n"

func isClashUnsupportedNode(node *protocol.NodeConfig) bool {
	if node == nil {
		return true
	}
	net := strings.ToLower(strings.TrimSpace(node.GetParam("type")))
	if net == "" {
		net = strings.ToLower(strings.TrimSpace(node.GetParam("net")))
	}
	return net == "xhttp" || net == "splithttp"
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

		if isClashUnsupportedNode(node) {
			slog.Debug("Skipping Clash-incompatible node transport", slog.String("node", name), slog.String("transport", node.GetParam("type")))
			continue
		}

		proxyMap, err := protocol.ToClash(node)
		if err != nil {
			slog.Warn("Failed to convert node to Clash proxy, skipping", slog.String("node", name), slog.String("protocol", node.Protocol), slog.String("error", err.Error()))
			continue
		}
		if proxyMap != nil {
			proxyMap["name"] = name
			proxies = append(proxies, proxyMap)
			proxyNames = append(proxyNames, name)
		}
	}

	var selectProxies []string
	var autoProxies []string
	if len(proxyNames) > 0 {
		selectProxies = make([]string, 0, len(proxyNames)+1)
		selectProxies = append(selectProxies, "AUTO")
		selectProxies = append(selectProxies, proxyNames...)
		autoProxies = proxyNames
	} else {
		selectProxies = []string{"DIRECT"}
		autoProxies = []string{"DIRECT"}
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
				"proxies": selectProxies,
			},
			{
				"name":     "AUTO",
				"type":     "url-test",
				"url":      "http://www.gstatic.com/generate_204",
				"interval": 300,
				"proxies":  autoProxies,
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

	if len(proxyNames) == 0 {
		return clashEmptyNotice + string(data), nil
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
