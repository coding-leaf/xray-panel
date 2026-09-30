package xray

import (
	"panel/internal/adapter/xray/proto"
	"panel/internal/domain"
)

// AccountBuilder gRPC 账户消息构建策略接口 (兼容旧契约)
type AccountBuilder interface {
	Protocol() string
	BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
}

// accountBuilderAdapter 将纯 AccountBuilder 包装为 ProtocolAdapter
type accountBuilderAdapter struct {
	builder AccountBuilder
}

func (a *accountBuilderAdapter) Protocol() string {
	return a.builder.Protocol()
}

func (a *accountBuilderAdapter) BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error) {
	return a.builder.BuildAccount(inbound, user, accessor)
}

func (a *accountBuilderAdapter) CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error) {
	return nil, nil
}

func (a *accountBuilderAdapter) DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{}) {
}

// AccountRegistry 账户构建器策略注册中心 (对 ProtocolRegistry 的适配封装，保证完全向后兼容)
type AccountRegistry struct {
	protoRegistry *ProtocolRegistry
}

// NewAccountRegistry 创建注册中心
func NewAccountRegistry() *AccountRegistry {
	return &AccountRegistry{
		protoRegistry: NewProtocolRegistry(),
	}
}

var defaultAccountRegistry = &AccountRegistry{
	protoRegistry: DefaultProtocolRegistry(),
}

// DefaultAccountRegistry 获取全局默认注册中心
func DefaultAccountRegistry() *AccountRegistry {
	return defaultAccountRegistry
}

// RegisterAccountBuilder 注册账户构建策略到默认中心
func RegisterAccountBuilder(b AccountBuilder) {
	defaultAccountRegistry.Register(b)
}

// Register 注册账户构建策略
func (r *AccountRegistry) Register(b AccountBuilder) {
	if b == nil {
		return
	}
	if pa, ok := b.(ProtocolAdapter); ok {
		r.protoRegistry.Register(pa)
	} else {
		r.protoRegistry.Register(&accountBuilderAdapter{builder: b})
	}
}

// Get 获取指定协议的账户构建策略
func (r *AccountRegistry) Get(protocol string) (AccountBuilder, bool) {
	return r.protoRegistry.Get(protocol)
}

// BuildAccountMessage 使用全局注册中心根据 Inbound 协议和用户信息构建 gRPC TypedMessage
func BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*proto.TypedMessage, error) {
	return DefaultProtocolRegistry().BuildAccountMessage(inbound, u)
}

// BuildAccountMessage 使用指定注册中心构建 gRPC TypedMessage
func (r *AccountRegistry) BuildAccountMessage(inbound *domain.Inbound, u *domain.User) (*proto.TypedMessage, error) {
	return r.protoRegistry.BuildAccountMessage(inbound, u)
}

// 兼容别名
type VLESSAccountBuilder = VLESSAdapter
type VMessAccountBuilder = VMessAdapter
type TrojanAccountBuilder = TrojanAdapter
type ShadowsocksAccountBuilder = ShadowsocksAdapter
type Shadowsocks2022AccountBuilder = Shadowsocks2022Adapter
