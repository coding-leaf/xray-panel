# 解耦 domain 泄露与统一 Xray 协议适配层 (Decouple Domain & Unify Protocol Adapters & SS2022)

## Goal

彻底清理上一轮重构中遗留的架构边界违背：将泄漏到 `internal/domain` 的 Xray 具体配置模型（Reality、XHTTP、Vision 等）迁回适配层；扩展 Proto 与策略支持 Shadowsocks 2022（Multi-user Sub-Key 模式）；并将静态 Inbound 编译（`compiler.go`）与动态 gRPC 账户构建统一归入统一的协议适配策略（`ProtocolAdapter`）。

## Requirements

### 1. 领域模型边界清理 (Domain Purity)
- `internal/domain/stream_accessor.go` 应被移除或精简为纯粹的领域通用属性访问（若需保留强类型 Accessor，属于 Xray 特有解析逻辑的 `InboundStreamAccessor` 必须完全回归 `internal/adapter/xray` 层）。
- `internal/domain/inbound.go` 不得依赖 Xray 专属类型。
- `internal/protocol/node_converter.go` 如需从 Inbound 提取参数，使用解耦后的通用方法或在适配/转换层独立完成，保证 `internal/domain` 不含 `RealitySettingsDTO`、`XHTTPSettingsDTO`、`xtls-rprx-vision` 等 Xray 专用概念。

### 2. Shadowsocks 2022 (SS2022) 协议全链路落地
- 在 `internal/adapter/xray/proto` 中扩展 SS2022 proto 定义与 TypedMessage 映射：
  - 新增/映射 `xray.proxy.shadowsocks_2022.Account` (含 `key` 字段) 与 `xray.proxy.shadowsocks_2022.MultiUserServerConfig`。
- 在 `internal/adapter/xray` 中实现 SS2022 动态 gRPC 下发构建器（Multi-user 模式下使用 User Sub-Key 构建 TypedMessage）。
- 在订阅层 `internal/protocol` 支持 `2022-blake3-*` 系列 cipher 的导出与节点生成兼容。

### 3. 静态配置编译与动态 gRPC 下发统一归一 (Protocol Adapter Unification)
- 消除 `compiler.go` 中针对 VLESS / VMess / Trojan / Shadowsocks / SS2022 的重复 switch 分支：
  - 建立统一的协议适配体系（例如 `ProtocolAdapter` 接口，同时承载：协议名、静态 clients 生成/Inbound Settings 校验调整、动态 gRPC Account TypedMessage 构建）。
  - `compiler.go` 生成静态 `clients` 与组装 settings 时，委托给对应的协议适配器处理。
  - 动态 `account_builder.go` 与静态编译器共享相同的协议识别与加密方法解析逻辑，彻底消灭“双轨协议逻辑”。

## Acceptance Criteria

- [x] `internal/domain` 不包含任何 Xray 专属数据结构（无 `RealitySettingsDTO`、`XHTTPSettingsDTO` 等）；`Inbound` 实体保持无外部 Xray 副作用。
- [x] Xray Proto 层完整支持 `xray.proxy.shadowsocks_2022.Account`，TypedMessage 编解码单元测试通过。
- [x] 无论是传统 Shadowsocks 还是 Shadowsocks 2022，均能正确支持静态 `compiler` 编译以及动态 gRPC `AddUser` / `RemoveUser`。
- [x] 静态编译（`compiler.go`）与动态下发共用统一的 ProtocolAdapter 策略注册体系，无硬编码的协议分支分散。
- [x] 所有的单元测试、静态检查与构建全部通过：
  - `mise x -- go test ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build .`
  - `cd web && mise x -- npm run build`
