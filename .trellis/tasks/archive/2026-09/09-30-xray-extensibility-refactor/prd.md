# PRD: 架构整理与通用类抽取以应对未来 Xray 特性追新

## Goal

针对 Xray-core 持续演进的现实背景，全面重构与收敛当前仓库中碎片化的配置解析、流控判定、账户下发和订阅生成逻辑；抽取可复用强类型访问器、策略化 gRPC 账户构建器、下沉通用密码学基础设施，瘦身多协议订阅转换器，并清理前端废弃死逻辑，实现高度内聚、开闭原则良好、易于追新扩展的现代架构。

---

## Background & Pain Points


1. **配置解析碎片化与 DRY 严重违背**：
   - `domain.Inbound` 的 `StreamSettings` 与 `SettingsJSON` 为未类型化的 JSON 字符串；
   - `compiler.go`（配置编译）、`grpc_client.go`（gRPC 热下发）、`node_converter.go`（订阅转换）三处分别手写 `map[string]interface{}` 反序列化；
   - Vision Flow（`xtls-rprx-vision` 条件：TCP + REALITY/TLS）与 Shadowsocks Cipher 提取逻辑在三处重复编写，任何一处改动都容易导致遗漏与不一致。
2. **gRPC 账户构建耦合严重**：
   - `grpc_client.go` 内包含单体 `BuildAccountMessage`，以大 `switch` 混杂了 VLESS/VMess/Trojan/Shadowsocks 各种协议的 protobuf 构造，破坏单一职责与开闭原则。
3. **密码学与密钥工具职责越界**：
   - Curve25519 密钥对生成（`GenerateRealityKeyPair`）和公钥推导硬塞在 `internal/protocol/node_converter.go` 中，且 `internal/adapter/xray` 反向依赖 `protocol`。
4. **多协议订阅格式化代码冗余**：
   - `vless.go`, `trojan.go`, `vmess.go`, `hysteria2.go` 等文件中机械重复节点校验、TLS/REALITY 注入、Clash/Sing-box 适配以及 URI Query 构建逻辑。
5. **前端脱节死代码**：
   - `web/src/views/users/services/subscription.ts` 中手写了 70 余行 `generateNodeLink`，实际上并无任何组件使用，与后端脱节且容易误导维护。

---

## Detailed Requirements

### 1. 强类型 InboundStreamAccessor / InboundSettings 统一抽取
- 在 `internal/adapter/xray`（或 `internal/domain` 关联模型中）定义强类型的 `InboundStreamAccessor` 与 `InboundSettingsAccessor`：
  - 自动安全解析 `StreamSettings` 与 `SettingsJSON`，屏蔽底层 `map[string]interface{}` 与 JSON 反序列化细节；
  - 提供统一的 `ResolveVisionFlow()` 方法：自动校验 TCP + (REALITY 或 TLS)，安全识别 custom flow 与 fallback；
  - 提供统一的 `ResolveShadowsocksMethod()` 方法：统一 cipher/method 命名映射与兜底；
  - 提供强类型的 `GetReality()`、`GetTLS()`、`GetTransport()` 提取器；
  - `compiler.go`、`grpc_client.go`、`node_converter.go` 全面切换至此 Accessor，消除重复的解包与流控判定代码。

### 2. Xray gRPC 账户构建策略模式（AccountBuilder Registry）
- 从 `grpc_client.go` 剥离账户构建逻辑：
  - 定义 `AccountBuilder` 接口：
    ```go
    type AccountBuilder interface {
        Protocol() string
        BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
    }
    ```
  - 抽取并实现 `VLESSAccountBuilder`, `VMessAccountBuilder`, `TrojanAccountBuilder`, `ShadowsocksAccountBuilder`；
  - 提供线程安全的 `AccountBuilderRegistry`，支持动态注册与按协议分发；
  - `grpc_client.go` 仅保留统一入口调用，自身彻底聚焦 gRPC 连接生命周期与 Stats 采集。

### 3. 下沉基础设施包 `internal/pkg/crypto`
- 新建 `internal/pkg/crypto/reality.go`：
  - 迁移基于 Curve25519 的 `GenerateRealityKeyPair`、`DerivePublicKeyFromPrivate` 与 16 进制 ShortID 生成；
  - 迁移 VLESS 路由 UUID 掩码工具 `ApplyVlessRouteToUUID`；
  - `internal/adapter/xray` 与 `internal/protocol` 改为依赖 `internal/pkg/crypto`，消除反向与跨层依赖。

### 4. 瘦身 `internal/protocol` 订阅转换层
- 在 `internal/protocol/helpers.go` 提炼四大通用原语：
  - `ValidateBaseNode(node *NodeConfig) error`：统一校验 Address、Port、UUID/Password 等并返回标准错误；
  - `AttachClashTLS(proxy map[string]interface{}, node *NodeConfig)`：统一注入 TLS 与 REALITY 扩展块；
  - `AttachSingBoxTLS(outbound map[string]interface{}, node *NodeConfig)`：统一注入 Sing-box TLS 配置；
  - `BuildNodeQueryParams(node *NodeConfig, extraReserved ...string) url.Values`：统一安全构建 URI Query 参数；
- 改造 `vless.go`, `trojan.go`, `vmess.go`, `hysteria2.go`, `shadowsocks.go` 协议格式化器，复用通用原语，消除大量重复代码。

### 5. 前端废弃死逻辑清洗
- 清理 `web/src/views/users/services/subscription.ts` 中废弃未用的 `generateNodeLink` 及相关无用辅助函数，保留实际使用的 `getDirectTokenSubUrl` 与 `getNodeTags`；
- 确保前端严格通过 `npm run build` / `npm run typecheck`。

---

## Acceptance Criteria

- [x] **质量门禁全绿**：
  - 后端：`mise x -- go test ./...` 全部通过（包含存量与新增的单元测试）；
  - 后端：`mise x -- go vet ./...` 零警告；
  - 后端：`mise x -- go build .` 正常产出二进制；
  - 前端：`cd web && mise x -- npm run build` 成功完成打包且零 TS 错误。
- [x] **DRY 消除验证**：
  - `compiler.go`、`grpc_client.go`、`node_converter.go` 中不再包含重复手写的 Vision Flow 判定与 SS Cipher 解构逻辑；
  - Curve25519 密码学逻辑完整位于 `internal/pkg/crypto`，`protocol` 不再直接导入 `golang.org/x/crypto/curve25519`；
  - `internal/protocol` 各协议文件代码行数显著下降，无重复的 TLS/Clash/Sing-box 拼装；
  - `web/src/views/users/services/subscription.ts` 中死代码被完全剔除。
- [x] **功能与行为兼容性验证**：
  - 所有节点的订阅生成（Base64, Clash, Sing-box）输出结果与重构前严格一致；
  - VLESS 单端口多出口路由（routeId 替换 UUID）、用户白名单权限隔离功能不受任何影响；
  - Xray 动态添加/删除用户（AddUser / RemoveUser gRPC）行为与原有逻辑 100% 保持一致。
