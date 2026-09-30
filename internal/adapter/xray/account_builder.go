package xray

import (
	"fmt"
	"strings"
	"sync"

	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"
)

// AccountBuilder gRPC 账户消息构建策略接口
type AccountBuilder interface {
	Protocol() string
	BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
}

// AccountRegistry 账户构建器策略注册中心
type AccountRegistry struct {
	mu       sync.RWMutex
	builders map[string]AccountBuilder
}

// NewAccountRegistry 创建注册中心
func NewAccountRegistry() *AccountRegistry {
	return &AccountRegistry{
		builders: make(map[string]AccountBuilder),
	}
}

var defaultAccountRegistry = NewAccountRegistry()

// DefaultAccountRegistry 获取全局默认注册中心
func DefaultAccountRegistry() *AccountRegistry {
	return defaultAccountRegistry
}

// RegisterAccountBuilder 注册账户构建策略
func RegisterAccountBuilder(b AccountBuilder) {
	defaultAccountRegistry.Register(b)
}

// Register 注册账户构建策略
func (r *AccountRegistry) Register(b AccountBuilder) {
	if b == nil {
		return
	}
	p := strings.ToLower(strings.TrimSpace(b.Protocol()))
	if p == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builders[p] = b
}

// Get 获取指定协议的账户构建策略
func (r *AccountRegistry) Get(protocol string) (AccountBuilder, bool) {
	p := strings.ToLower(strings.TrimSpace(protocol))
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.builders[p]
	return b, ok
}

// BuildAccountMessage 使用全局注册中心根据 Inbound 协议和用户信息构建 gRPC TypedMessage
func BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*proto.TypedMessage, error) {
	return defaultAccountRegistry.BuildAccountMessage(inbound, u)
}

// BuildAccountMessage 使用指定注册中心构建 gRPC TypedMessage
func (r *AccountRegistry) BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*proto.TypedMessage, error) {
	if inbound == nil || u == nil {
		return nil, fmt.Errorf("%w: inbound or user cannot be nil", domain.ErrInvalidInput)
	}

	builder, ok := r.Get(inbound.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported protocol %s", domain.ErrInvalidInput, inbound.Protocol)
	}

	accessor := NewInboundStreamAccessorFromInbound(inbound)
	return builder.BuildAccount(inbound, u, accessor)
}

// ---------------- 内置核心协议策略实现 ----------------

// VLESSAccountBuilder VLESS 协议账户构建器
type VLESSAccountBuilder struct{}

func (b *VLESSAccountBuilder) Protocol() string {
	return "vless"
}

func (b *VLESSAccountBuilder) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	flow := accessor.ResolveVisionFlow(inbound.Protocol)
	return proto.ToTypedMessage(&proto.VLESSAccount{
		Id:   user.UUID,
		Flow: flow,
	})
}

// VMessAccountBuilder VMess 协议账户构建器
type VMessAccountBuilder struct{}

func (b *VMessAccountBuilder) Protocol() string {
	return "vmess"
}

func (b *VMessAccountBuilder) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return proto.ToTypedMessage(&proto.VMessAccount{
		Id: user.UUID,
		SecuritySettings: &proto.SecurityConfig{
			Type: proto.SecurityType_AUTO,
		},
	})
}

// TrojanAccountBuilder Trojan 协议账户构建器
type TrojanAccountBuilder struct{}

func (b *TrojanAccountBuilder) Protocol() string {
	return "trojan"
}

func (b *TrojanAccountBuilder) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return proto.ToTypedMessage(&proto.TrojanAccount{
		Password: user.UUID,
	})
}

// ShadowsocksAccountBuilder Shadowsocks 协议账户构建器
type ShadowsocksAccountBuilder struct{}

func (b *ShadowsocksAccountBuilder) Protocol() string {
	return "shadowsocks"
}

func (b *ShadowsocksAccountBuilder) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
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

func init() {
	RegisterAccountBuilder(&VLESSAccountBuilder{})
	RegisterAccountBuilder(&VMessAccountBuilder{})
	RegisterAccountBuilder(&TrojanAccountBuilder{})
	RegisterAccountBuilder(&ShadowsocksAccountBuilder{})
}
