package protocol

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry 订阅格式化器注册中心管理器
type Registry struct {
	mu         sync.RWMutex
	formatters map[string]SubFormatter
}

// NewRegistry 创建新的 Registry 实例
func NewRegistry() *Registry {
	return &Registry{
		formatters: make(map[string]SubFormatter),
	}
}

// defaultRegistry 全局单例管理器
var defaultRegistry = NewRegistry()

// DefaultRegistry 返回全局默认注册中心单例
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Register 向全局注册中心注册格式化器
func Register(f SubFormatter) {
	defaultRegistry.Register(f)
}

// Register 向当前 Registry 注册格式化器
func (r *Registry) Register(f SubFormatter) {
	if f == nil {
		return
	}
	proto := strings.ToLower(strings.TrimSpace(f.Protocol()))
	if proto == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.formatters[proto] = f
}

// Unregister 从 Registry 注销格式化器（主要用于测试）
func (r *Registry) Unregister(protocol string) {
	proto := strings.ToLower(strings.TrimSpace(protocol))
	if proto == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.formatters, proto)
}

// Get 从全局注册中心获取指定协议的格式化器
func Get(protocol string) (SubFormatter, bool) {
	return defaultRegistry.Get(protocol)
}

// Get 从当前 Registry 获取指定协议的格式化器
func (r *Registry) Get(protocol string) (SubFormatter, bool) {
	proto := strings.ToLower(strings.TrimSpace(protocol))
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.formatters[proto]
	return f, ok
}

// FormatLink 使用全局注册中心格式化节点分享链接
func FormatLink(node *NodeConfig) (string, error) {
	return defaultRegistry.FormatLink(node)
}

// FormatLink 使用当前 Registry 格式化节点分享链接
func (r *Registry) FormatLink(node *NodeConfig) (string, error) {
	if node == nil {
		return "", ErrNilNode
	}
	f, ok := r.Get(node.Protocol)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedProtocol, node.Protocol)
	}
	return f.FormatLink(node)
}

// ToClash 使用全局注册中心将节点转为 Clash / Mihomo proxy map
func ToClash(node *NodeConfig) (map[string]interface{}, error) {
	return defaultRegistry.ToClash(node)
}

// ToClash 使用当前 Registry 将节点转为 Clash / Mihomo proxy map
func (r *Registry) ToClash(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	f, ok := r.Get(node.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, node.Protocol)
	}
	converter, ok := f.(ClashConverter)
	if !ok {
		return nil, fmt.Errorf("protocol %s does not support clash export", node.Protocol)
	}
	return converter.ToClash(node)
}

// ToSingBox 使用全局注册中心将节点转为 Sing-box outbound map
func ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	return defaultRegistry.ToSingBox(node)
}

// ToSingBox 使用当前 Registry 将节点转为 Sing-box outbound map
func (r *Registry) ToSingBox(node *NodeConfig) (map[string]interface{}, error) {
	if node == nil {
		return nil, ErrNilNode
	}
	f, ok := r.Get(node.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, node.Protocol)
	}
	converter, ok := f.(SingBoxConverter)
	if !ok {
		return nil, fmt.Errorf("protocol %s does not support sing-box export", node.Protocol)
	}
	return converter.ToSingBox(node)
}

// ListSupportedProtocols 返回全局注册中心支持的所有协议列表
func ListSupportedProtocols() []string {
	return defaultRegistry.ListSupportedProtocols()
}

// ListSupportedProtocols 返回当前 Registry 支持的所有协议列表
func (r *Registry) ListSupportedProtocols() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	protocols := make([]string, 0, len(r.formatters))
	for proto := range r.formatters {
		protocols = append(protocols, proto)
	}
	sort.Strings(protocols)
	return protocols
}
