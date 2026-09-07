package sub

import (
	"encoding/json"
	"fmt"

	"panel/internal/protocol"
)

// SingBoxConfig Sing-box 标准订阅配置结构
type SingBoxConfig struct {
	Outbounds []map[string]interface{} `json:"outbounds"`
}

// ExportSingBox 将多节点配置转换为 Sing-box JSON 订阅 (基于 protocol.Registry 策略模式)
func ExportSingBox(nodes []*protocol.NodeConfig) (string, error) {
	outbounds := make([]map[string]interface{}, 0, len(nodes)+5)
	nodeTags := make([]string, 0, len(nodes))

	for idx, node := range nodes {
		if node == nil {
			continue
		}
		tag := node.Name
		if tag == "" {
			tag = fmt.Sprintf("Node-%d", idx+1)
		}

		outbound, err := protocol.ToSingBox(node)
		if err != nil {
			return "", fmt.Errorf("failed to convert node %s to sing-box outbound: %w", tag, err)
		}
		if outbound != nil {
			outbound["tag"] = tag
			outbounds = append(outbounds, outbound)
			nodeTags = append(nodeTags, tag)
		}
	}

	fallbackTags := nodeTags
	if len(fallbackTags) == 0 {
		fallbackTags = []string{"direct"}
	}

	allOutbounds := make([]map[string]interface{}, 0, len(outbounds)+5)

	// 1. Selector 出站
	allOutbounds = append(allOutbounds, map[string]interface{}{
		"type":      "selector",
		"tag":       "select",
		"outbounds": append([]string{"auto"}, fallbackTags...),
		"default":   "auto",
	})

	// 2. URLTest 自动测速出站
	allOutbounds = append(allOutbounds, map[string]interface{}{
		"type":      "urltest",
		"tag":       "auto",
		"outbounds": fallbackTags,
		"url":       "http://cp.cloudflare.com/generate_204",
		"interval":  "3m",
	})

	// 3. 各代理节点
	allOutbounds = append(allOutbounds, outbounds...)

	// 4. 基础直连与拦截出站
	allOutbounds = append(allOutbounds,
		map[string]interface{}{
			"type": "direct",
			"tag":  "direct",
		},
		map[string]interface{}{
			"type": "block",
			"tag":  "block",
		},
		map[string]interface{}{
			"type": "dns",
			"tag":  "dns-out",
		},
	)

	cfg := SingBoxConfig{
		Outbounds: allOutbounds,
	}

	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal sing-box json: %w", err)
	}

	return string(data), nil
}
