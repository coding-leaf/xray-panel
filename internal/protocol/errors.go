package protocol

import "errors"

var (
	// ErrNilNode 节点配置为空错误
	ErrNilNode = errors.New("node config cannot be nil")
	// ErrMissingAddress 缺少节点地址
	ErrMissingAddress = errors.New("node address is required")
	// ErrInvalidPort 节点端口非法
	ErrInvalidPort = errors.New("invalid node port, must be between 1 and 65535")
	// ErrMissingUUID 缺少用户凭证
	ErrMissingUUID = errors.New("node credentials/uuid is required")
	// ErrUnsupportedProtocol 不受支持的协议
	ErrUnsupportedProtocol = errors.New("unsupported protocol")
)
