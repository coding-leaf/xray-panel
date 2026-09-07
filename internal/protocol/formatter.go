package protocol

// SubFormatter 订阅格式化器接口（策略模式）
type SubFormatter interface {
	// Protocol 返回支持的协议标识，如 "vless", "trojan", "hysteria2", "vmess"
	Protocol() string
	// FormatLink 将节点转为对应的标准分享链接（如 vless://...）
	FormatLink(node *NodeConfig) (string, error)
}

// ClashConverter 可选扩展接口：策略模式生成 Clash / Mihomo 代理配置
type ClashConverter interface {
	ToClash(node *NodeConfig) (map[string]interface{}, error)
}

// SingBoxConverter 可选扩展接口：策略模式生成 Sing-box 出站配置
type SingBoxConverter interface {
	ToSingBox(node *NodeConfig) (map[string]interface{}, error)
}
