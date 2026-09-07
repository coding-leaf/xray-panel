package sub

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"panel/internal/protocol"
)

// 类型别名与常用函数透出，保证调用方友好
type NodeConfig = protocol.NodeConfig
type SubFormatter = protocol.SubFormatter
type ClashConverter = protocol.ClashConverter
type SingBoxConverter = protocol.SingBoxConverter

var (
	Register        = protocol.Register
	Get             = protocol.Get
	FormatLink      = protocol.FormatLink
	ToClash         = protocol.ToClash
	ToSingBox       = protocol.ToSingBox
	DefaultRegistry = protocol.DefaultRegistry
)

// 订阅导出格式常量
const (
	FormatBase64  = "base64"
	FormatRaw     = "raw"
	FormatClash   = "clash"
	FormatSingBox = "sing-box"
)

// ExporterFunc 订阅格式导出器函数定义
type ExporterFunc func(nodes []*protocol.NodeConfig) (string, error)

type exporterRegistry struct {
	mu        sync.RWMutex
	exporters map[string]ExporterFunc
}

var globalExporters = &exporterRegistry{
	exporters: make(map[string]ExporterFunc),
}

// RegisterExporter 动态注册导出格式策略
func RegisterExporter(format string, fn ExporterFunc) {
	if fn == nil {
		return
	}
	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" {
		return
	}
	globalExporters.mu.Lock()
	defer globalExporters.mu.Unlock()
	globalExporters.exporters[fmtLower] = fn
}

// UnregisterExporter 注销指定的导出格式 (主要用于测试)
func UnregisterExporter(format string) {
	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" {
		return
	}
	globalExporters.mu.Lock()
	defer globalExporters.mu.Unlock()
	delete(globalExporters.exporters, fmtLower)
}

// ListSupportedFormats 返回当前已注册的所有导出格式
func ListSupportedFormats() []string {
	globalExporters.mu.RLock()
	defer globalExporters.mu.RUnlock()
	formats := make([]string, 0, len(globalExporters.exporters))
	for f := range globalExporters.exporters {
		formats = append(formats, f)
	}
	sort.Strings(formats)
	return formats
}

func init() {
	RegisterExporter(FormatBase64, ExportBase64)
	RegisterExporter("b64", ExportBase64)
	RegisterExporter(FormatRaw, ExportRaw)
	RegisterExporter("plain", ExportRaw)
	RegisterExporter("links", ExportRaw)
	RegisterExporter(FormatClash, ExportClash)
	RegisterExporter("clash-meta", ExportClash)
	RegisterExporter("mihomo", ExportClash)
	RegisterExporter(FormatSingBox, ExportSingBox)
	RegisterExporter("singbox", ExportSingBox)
}

// ExportSubscription 统一导出多节点订阅配置 (基于策略模式分发)
func ExportSubscription(nodes []*protocol.NodeConfig, format string) (string, error) {
	fmtLower := strings.ToLower(strings.TrimSpace(format))
	if fmtLower == "" {
		fmtLower = FormatBase64
	}

	globalExporters.mu.RLock()
	exporter, ok := globalExporters.exporters[fmtLower]
	globalExporters.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unsupported subscription format: %s", format)
	}

	return exporter(nodes)
}
