package xray

import (
	"fmt"
	"strings"
	"sync"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"
)

// ProtocolAdapter 统一协议适配器接口，覆盖静态 clients 编译、动态 gRPC 账户构建与 settings 规范化调整
type ProtocolAdapter interface {
	Protocol() string
	// CompileClients 静态编译阶段：为 Inbound 构造客户端认证列表
	CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error)
	// BuildAccount 动态 gRPC 下发阶段：构造 TypedMessage 账户
	BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
	// DecorateSettings 可选：针对协议特定的 settings 调整 (如 vless decryption=none, socks udp=true)
	DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{})
}

func filterActiveUsersForInbound(inbound *domain.Inbound, users []domain.User) []domain.User {
	var result []domain.User
	for _, u := range users {
		if u.IsActive() && u.HasInbound(inbound.Tag) {
			result = append(result, u)
		}
	}
	return result
}

// ProtocolRegistry 协议适配器策略注册中心
type ProtocolRegistry struct {
	mu       sync.RWMutex
	adapters map[string]ProtocolAdapter
}

// NewProtocolRegistry 创建并初始化内置适配器的注册中心
func NewProtocolRegistry() *ProtocolRegistry {
	r := &ProtocolRegistry{
		adapters: make(map[string]ProtocolAdapter),
	}
	r.registerBuiltins()
	return r
}

func (r *ProtocolRegistry) registerBuiltins() {
	r.Register(&VLESSAdapter{})
	r.Register(&VMessAdapter{})
	r.Register(&TrojanAdapter{})
	r.Register(&ShadowsocksAdapter{})
	r.Register(&Shadowsocks2022Adapter{})
	r.Register(&SocksAdapter{})
}

var defaultProtocolRegistry = NewProtocolRegistry()

// DefaultProtocolRegistry 获取全局默认协议适配器注册中心
func DefaultProtocolRegistry() *ProtocolRegistry {
	return defaultProtocolRegistry
}

// Register 注册协议适配策略
func (r *ProtocolRegistry) Register(adapter ProtocolAdapter) {
	if adapter == nil {
		return
	}
	p := strings.ToLower(strings.TrimSpace(adapter.Protocol()))
	if p == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[p] = adapter
}

// Get 获取指定协议名的适配策略
func (r *ProtocolRegistry) Get(protocol string) (ProtocolAdapter, bool) {
	p := strings.ToLower(strings.TrimSpace(protocol))
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[p]
	return a, ok
}

// ResolveAdapter 根据 Inbound 配置智能解析对应的协议适配器 (如识别 SS2022 与传统 SS)
func (r *ProtocolRegistry) ResolveAdapter(inbound *domain.Inbound) (ProtocolAdapter, bool) {
	if inbound == nil {
		return nil, false
	}
	p := strings.ToLower(strings.TrimSpace(inbound.Protocol))
	if p == "shadowsocks" || p == "ss" {
		accessor := NewInboundStreamAccessorFromInbound(inbound)
		method := strings.ToLower(accessor.ResolveShadowsocksMethod())
		if strings.Contains(method, "2022-blake3") {
			if a, ok := r.Get("shadowsocks-2022"); ok {
				return a, true
			}
		}
		return r.Get("shadowsocks")
	}
	if p == "shadowsocks-2022" || p == "ss-2022" || p == "ss2022" {
		if a, ok := r.Get("shadowsocks-2022"); ok {
			return a, true
		}
	}
	return r.Get(p)
}

// BuildAccountMessage 使用该注册中心根据 Inbound 与 User 构造 gRPC TypedMessage
func (r *ProtocolRegistry) BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*proto.TypedMessage, error) {
	if inbound == nil || u == nil {
		return nil, fmt.Errorf("%w: inbound or user cannot be nil", domain.ErrInvalidInput)
	}

	adapter, ok := r.ResolveAdapter(inbound)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported protocol %s", domain.ErrInvalidInput, inbound.Protocol)
	}

	accessor := NewInboundStreamAccessorFromInbound(inbound)
	return adapter.BuildAccount(inbound, u, accessor)
}

// ---------------- 内置核心协议适配器 ----------------

// VLESSAdapter VLESS 协议适配器
type VLESSAdapter struct{}

func (a *VLESSAdapter) Protocol() string {
	return "vless"
}

func (a *VLESSAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	inboundFlow := accessor.ResolveVisionFlow(inbound.Protocol)
	activeUsers := filterActiveUsersForInbound(inbound, users)
	clients := make([]XrayClient, 0, len(activeUsers))
	for _, u := range activeUsers {
		clients = append(clients, XrayClient{
			ID:    u.UUID,
			Flow:  inboundFlow,
			Email: u.Email,
			Level: 0,
		})
	}
	return clients, nil
}

func (a *VLESSAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	flow := accessor.ResolveVisionFlow(inbound.Protocol)
	return proto.ToTypedMessage(&proto.VLESSAccount{
		Id:   user.UUID,
		Flow: flow,
	})
}

func (a *VLESSAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
	if settingsMap["decryption"] == nil {
		settingsMap["decryption"] = "none"
	}
}

// VMessAdapter VMess 协议适配器
type VMessAdapter struct{}

func (a *VMessAdapter) Protocol() string {
	return "vmess"
}

func (a *VMessAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	activeUsers := filterActiveUsersForInbound(inbound, users)
	clients := make([]XrayClient, 0, len(activeUsers))
	for _, u := range activeUsers {
		clients = append(clients, XrayClient{
			ID:    u.UUID,
			Email: u.Email,
			Level: 0,
		})
	}
	return clients, nil
}

func (a *VMessAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return proto.ToTypedMessage(&proto.VMessAccount{
		Id: user.UUID,
		SecuritySettings: &proto.SecurityConfig{
			Type: proto.SecurityType_AUTO,
		},
	})
}

func (a *VMessAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
}

// TrojanAdapter Trojan 协议适配器
type TrojanAdapter struct{}

func (a *TrojanAdapter) Protocol() string {
	return "trojan"
}

func (a *TrojanAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	activeUsers := filterActiveUsersForInbound(inbound, users)
	clients := make([]XrayClient, 0, len(activeUsers))
	for _, u := range activeUsers {
		clients = append(clients, XrayClient{
			Password: u.UUID,
			Email:    u.Email,
			Level:    0,
		})
	}
	return clients, nil
}

func (a *TrojanAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return proto.ToTypedMessage(&proto.TrojanAccount{
		Password: user.UUID,
	})
}

func (a *TrojanAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
}

// ShadowsocksAdapter 传统 AEAD Shadowsocks 协议适配器
type ShadowsocksAdapter struct{}

func (a *ShadowsocksAdapter) Protocol() string {
	return "shadowsocks"
}

func (a *ShadowsocksAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	ssMethod := accessor.ResolveShadowsocksMethod()
	activeUsers := filterActiveUsersForInbound(inbound, users)
	clients := make([]XrayClient, 0, len(activeUsers))
	for _, u := range activeUsers {
		clients = append(clients, XrayClient{
			Password: u.UUID,
			Method:   ssMethod,
			Email:    u.Email,
			Level:    0,
		})
	}
	return clients, nil
}

func (a *ShadowsocksAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	method := accessor.ResolveShadowsocksMethod()
	cipherType := proto.CipherType_AES_128_GCM
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "aes-256-gcm":
		cipherType = proto.CipherType_AES_256_GCM
	case "chacha20-poly1305", "chacha20-ietf-poly1305":
		cipherType = proto.CipherType_CHACHA20_POLY1305
	case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
		cipherType = proto.CipherType_XCHACHA20_POLY1305
	case "none":
		cipherType = proto.CipherType_NONE
	case "aes-128-gcm":
		cipherType = proto.CipherType_AES_128_GCM
	default:
		cipherType = proto.CipherType_AES_128_GCM
	}

	return proto.ToTypedMessage(&proto.ShadowsocksAccount{
		Password:   user.UUID,
		CipherType: cipherType,
	})
}

func (a *ShadowsocksAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
	if _, ok := settingsMap["network"]; !ok {
		settingsMap["network"] = "tcp,udp"
	}
}

// Shadowsocks2022Adapter Shadowsocks 2022 (Multi-user Sub-Key) 协议适配器
type Shadowsocks2022Adapter struct{}

func (a *Shadowsocks2022Adapter) Protocol() string {
	return "shadowsocks-2022"
}

func (a *Shadowsocks2022Adapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	activeUsers := filterActiveUsersForInbound(inbound, users)
	clients := make([]XrayClient, 0, len(activeUsers))
	for _, u := range activeUsers {
		clients = append(clients, XrayClient{
			Password: u.UUID,
			Email:    u.Email,
			Level:    0,
		})
	}
	return clients, nil
}

func (a *Shadowsocks2022Adapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return proto.ToTypedMessage(&proto.Shadowsocks2022Account{
		Key: user.UUID,
	})
}

func (a *Shadowsocks2022Adapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
	accessor := NewInboundStreamAccessorFromInbound(inbound)
	method := accessor.ResolveShadowsocksMethod()
	if !strings.Contains(method, "2022-blake3") {
		method = "2022-blake3-aes-128-gcm"
	}
	settingsMap["method"] = method
	if _, ok := settingsMap["network"]; !ok {
		settingsMap["network"] = "tcp,udp"
	}
}

// SocksAdapter Socks 协议适配器
type SocksAdapter struct{}

func (a *SocksAdapter) Protocol() string {
	return "socks"
}

func (a *SocksAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	return nil, nil
}

func (a *SocksAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return nil, fmt.Errorf("%w: socks protocol does not support dynamic account operations", domain.ErrInvalidInput)
}

func (a *SocksAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
	if _, ok := settingsMap["udp"]; !ok {
		settingsMap["udp"] = true
	}
	if _, ok := settingsMap["auth"]; !ok {
		settingsMap["auth"] = "noauth"
	}
}
