# 实施计划与步骤 (Implementation Plan)

## ordered Checklist

- [x] **Milestone 1: Domain 边界清理与迁移**
  - [x] 将 `internal/domain/stream_accessor.go` 的完整定义迁移至 `internal/adapter/xray/stream_accessor.go`，并在 `internal/adapter/xray` 中维持强类型与测试。
  - [x] 彻底删除 `internal/domain/stream_accessor.go`。
  - [x] 清理 `internal/domain/inbound.go` 中的 `GetStreamAccessor()`。
  - [x] 重构 `internal/protocol/node_converter.go`：独立解析 Inbound 流配置 JSON，不依赖 `adapter/xray` 或已移除的 `domain.InboundStreamAccessor`。
  - [x] 验证：`go test ./internal/domain/... ./internal/protocol/...`

- [x] **Milestone 2: SS2022 Proto 与 TypedMessage 扩展**
  - [x] 在 `internal/adapter/xray/proto/proxy.proto` 中添加 `Shadowsocks2022Account`。
  - [x] 在 `internal/adapter/xray/proto/proxy.pb.go` 中生成/补充 `Shadowsocks2022Account` 结构与反射方法（或运行 protoc / 手工无冲突扩展 protobuf 结构）。
  - [x] 在 `internal/adapter/xray/proto/typed_message.go` 中添加 `TypeShadowsocks2022Account` 常量与编解码 case。
  - [x] 编写/扩展 `proto_test.go`，验证 SS2022 Account TypedMessage 正确打包与解析。

- [x] **Milestone 3: ProtocolAdapter 统一协议适配体系**
  - [x] 定义 `ProtocolAdapter` 接口与 `ProtocolRegistry` 注册中心（替代原先单目的的 `AccountRegistry`）。
  - [x] 实现各协议适配器：
    - `VLESSAdapter`
    - `VMessAdapter`
    - `TrojanAdapter`
    - `ShadowsocksAdapter` (传统 AEAD)
    - `Shadowsocks2022Adapter` (SS2022 支持，sub-key 动态账户与静态编译)
    - `SocksAdapter`
  - [x] 改造 `compiler.go`：消除 `compileInbound` 中的 switch-case，统一委托给 `ProtocolRegistry.Get(inb.Protocol)`。
  - [x] 改造 `account_builder.go`：复用 `ProtocolRegistry` 进行 gRPC 消息构建。
  - [x] 编写和更新 `compiler_test.go` 与 `account_builder_test.go`，增加 SS2022 静态编译与 gRPC 动态构建断言。

- [x] **Milestone 4: 质量门禁与全量验证**
  - [x] 执行 `mise x -- go test ./...`
  - [x] 执行 `mise x -- go vet ./...`
  - [x] 执行 `mise x -- go build .`
  - [x] 执行 `cd web && mise x -- npm run build`

## Validation Commands
```bash
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
cd web && mise x -- npm run build
```
