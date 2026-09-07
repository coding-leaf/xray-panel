package sub

import (
	"encoding/base64"
	"fmt"
	"strings"

	"panel/internal/protocol"
)

// ExportRaw 导出换行分隔的标准分享链接
func ExportRaw(nodes []*protocol.NodeConfig) (string, error) {
	if len(nodes) == 0 {
		return "", nil
	}

	var links []string
	for idx, node := range nodes {
		if node == nil {
			continue
		}
		link, err := protocol.FormatLink(node)
		if err != nil {
			name := node.Name
			if name == "" {
				name = fmt.Sprintf("node[%d]", idx)
			}
			return "", fmt.Errorf("failed to format node %s: %w", name, err)
		}
		if link != "" {
			links = append(links, link)
		}
	}

	return strings.Join(links, "\n"), nil
}

// ExportBase64 导出 Base64 编码的订阅字符串
func ExportBase64(nodes []*protocol.NodeConfig) (string, error) {
	raw, err := ExportRaw(nodes)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	return base64.StdEncoding.EncodeToString([]byte(raw)), nil
}
