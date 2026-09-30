# 技术方案设计 (Technical Design)

## 1. 架构边界与职责划分

```
                  ┌──────────────────────────────────────────────┐
                  │               internal/domain                │
                  │   Inbound, User, Outbound, RealityEvaluator  │
                  │   (纯粹领域实体，无 Xray 专用 DTO 或 flow 规则)   │
                  └──────────────────────▲───────────────────────┘
                                         │ 引用
                  ┌──────────────────────┴───────────────────────┐
                  │             internal/protocol                │
                  │   NodeConfig, Formatters (vless, vmess, ss)  │
                  │   NodeConverter (从通用 Inbound 提取参数)    │
                  └──────────────────────▲───────────────────────┘
                                         │ 引用
                  ┌──────────────────────┴───────────────────────┐
                  │            internal/adapter/xray             │
                  │  - InboundStreamAccessor (Xray 强类型解析)   │
                  │  - ProtocolAdapter (统一协议策略中心)        │
                  │      ├─ CompileClients(inbound, users)       │
                  │      └─ BuildAccount(inbound, user)          │
                  │  - XrayCompiler (调用 ProtocolAdapter 编排)  │
                  │  - Proto (含 SS2022 Account / TypedMessage)  │
                  └──────────────────────────────────────────────┘
```

### 1.1 domain 层净化
- 将 `InboundStreamAccessor` 及 `StreamSettingsDTO`、`RealitySettingsDTO` 等移入 `internal/adapter/xray/stream_accessor.go`。
- 在 `internal/protocol/node_converter.go` 中，将原先调用 `inbound.GetStreamAccessor()` 改为由 `internal/protocol` 自身轻量读取 JSON 字段（或使用 `internal/protocol` 专属的解包工具），解除对 `InboundStreamAccessor` 的跨层依赖。
- 从 `internal/domain/inbound.go` 中剔除 `GetStreamAccessor()` 方法，保证 domain 不受任何底层适配器侵染。

### 1.2 ProtocolAdapter 统一契约
设计统一的协议适配器接口，一揽子解决静态编译与动态 gRPC：
```go
package xray

type ProtocolAdapter interface {
    Protocol() string
    // CompileClients 静态编译阶段：为 Inbound 构造客户端认证列表或调整 settingsMap
    CompileClients(inbound *domain.Inbound, users []domain.User, accessor *InboundStreamAccessor) ([]XrayClient, error)
    // BuildAccount 动态 gRPC 下发阶段：构造 TypedMessage
    BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
    // DecorateSettings 可选：针对协议特定的 settings 调整 (如 vless decryption=none, socks udp=true)
    DecorateSettings(inbound *domain.Inbound, settingsMap map[string]interface{})
}
```
通过 `ProtocolRegistry` 统一注册：
- `VLESSAdapter`
- `VMessAdapter`
- `TrojanAdapter`
- `ShadowsocksAdapter` (传统 AEAD)
- `Shadowsocks2022Adapter` (SS2022 multi-user)
- `SocksAdapter`

### 1.3 SS2022 规范与 Proto 支持
- 在 `internal/adapter/xray/proto/proxy.proto` 中增加：
  ```protobuf
  message Shadowsocks2022Account {
    string key = 1;
  }
  ```
- 更新 `typed_message.go`：
  - 映射 `TypeShadowsocks2022Account = "xray.proxy.shadowsocks_2022.Account"`。
  - 支持 `*Shadowsocks2022Account` 的 TypedMessage 封装与解包。
- 区分判断：
  - 当协议为 `shadowsocks` 时，根据 `method` / `cipher` 是否包含 `2022-blake3`（如 `2022-blake3-aes-128-gcm`、`2022-blake3-aes-256-gcm`、`2022-blake3-chacha20-poly1305`），自动路由到 `Shadowsocks2022Adapter`，否则路由到传统 `ShadowsocksAdapter`。
  - SS2022 Multi-user 静态编译生成 `clients`: `[{ "password": user.UUID }]`（Xray 规范中 multi-user 模式下每个 user 的 sub-key 存放在 `password` 或 `key` 字段，且 `method` 必须为空）。

## 2. 兼容性与迁移保证
- 数据库与前端 API 保持 100% 兼容。
- 外部调用者无需关心 SS2022 内部 gRPC 是 2022 还是旧版，统一由适配层透明分发。
- 确保测试用例覆盖老版 Shadowsocks 与 SS2022 两种场景。
