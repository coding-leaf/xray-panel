package xray

import (
	"panel/internal/domain"
)

// InboundStreamAccessor 统一强类型解析与访问器别名
type InboundStreamAccessor = domain.InboundStreamAccessor

// NewInboundStreamAccessor 创建流配置访问器别名
var NewInboundStreamAccessor = domain.NewInboundStreamAccessor

// NewInboundStreamAccessorFromInbound 从 domain.Inbound 创建访问器别名
var NewInboundStreamAccessorFromInbound = domain.NewInboundStreamAccessorFromInbound
