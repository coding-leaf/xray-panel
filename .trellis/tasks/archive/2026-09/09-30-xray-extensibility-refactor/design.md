# Technical Design: 架构整理与通用类抽取以应对未来 Xray 特性追新

## 1. 架构总览与分层设计 (Architecture Overview)

本次重构旨在理顺分层职责、消除重复代码（DRY），并为追新 Xray 协议与特性建立高内聚、易扩展的技术底座：

```
+-------------------------------------------------------------+
|                  internal/delivery/http                     |
+-------------------------------------------------------------+
        |                                       |
        v                                       v
+-----------------------+              +-----------------------+
|   internal/service    |              |   internal/protocol   |
|   (SubService 等)     |------------->| (NodeConfig/Registry) |
+-----------------------+              +-----------------------+
        |                                       |
        v                                       v
+-----------------------+              +-----------------------+
| internal/adapter/xray |              | internal/pkg/crypto   |
| - StreamAccessor      |<-------------| (Curve25519/UUIDMask) |
| - AccountBuilder Reg  |              +-----------------------+
| - Compiler / GRPC Cli |
+-----------------------+
```

---

## 2. 模块细化设计 (Component Specifications)

### 2.1 基础设施下沉：`internal/pkg/crypto`
- **目标**：将所有纯密码学与密钥生成逻辑下沉至底层基础设施包，消除领域与外部适配器对协议上层的反向依赖。
- **文件**：`internal/pkg/crypto/reality.go`
- **定义**：
  ```go
  type RealityKeyPair struct {
      PrivateKey string `json:"privateKey"`
      PublicKey  string `json:"publicKey"`
      ShortID    string `json:"shortId"`
  }

  func GenerateRealityKeyPair() (*RealityKeyPair, error)
  func DerivePublicKeyFromPrivate(privStr string) string
  func ApplyVlessRouteToUUID(rawUUID string, routeID uint16) string
  ```
- **依赖消除**：`internal/protocol/node_converter.go` 移除 `golang.org/x/crypto/curve25519` 引用，改为导入 `panel/internal/pkg/crypto`。

### 2.2 核心统一抽象：`internal/adapter/xray/stream_accessor.go`
- **目标**：强类型安全解析 `StreamSettings` 与 `SettingsJSON`，为配置编译（`compiler.go`）、热下发（`grpc_client.go`）与节点转换（`node_converter.go`）提供统一的事实源。
- **核心结构**：
  ```go
  type InboundStreamAccessor struct {
      Network         string
      Security        string
      RealitySettings *XrayRealitySettings
      TLSSettings     *XrayTLSSettings
      WSSettings      *XrayWSSettings
      GRPCSettings    *XrayGRPCSettings
      XHTTPSettings   *XrayXHTTPSettings
      
      // 原始 settings 解析缓存
      settingsMap     map[string]interface{}
  }

  func NewInboundStreamAccessor(streamSettingsJSON string, settingsJSON string) *InboundStreamAccessor
  ```
- **核心方法**：
  - `ResolveVisionFlow() string`：
    统一执行 Vision Flow 规则：仅当 `(Network == "" || Network == "tcp") && (Security == "reality" || Security == "tls")` 时有效，优先读取用户显式配置，缺省返回 `xtls-rprx-vision`，其余场景一律返回空。
  - `ResolveShadowsocksMethod() string`：
    统一提取 `method` 或 `cipher`，缺失时默认 `aes-128-gcm`。
  - `GetRealityPublicKey() string`：
    自动获取 `publicKey`，若未填写但存在 `privateKey` 则自动调用 `crypto.DerivePublicKeyFromPrivate` 推导。
  - `GetRealityServerName() string`：
    统一提取 `serverName` 或首个 `serverNames`。
  - `GetRealityShortID() string`：
    统一提取 `shortId` 或首个 `shortIds`。

### 2.3 gRPC 账户构建策略模式：`internal/adapter/xray/account`
- **目标**：消除 `grpc_client.go` 中的单体 `switch protocol`，支持新协议零侵入添加。
- **核心接口与注册中心**：
  ```go
  type AccountBuilder interface {
      Protocol() string
      BuildAccount(inbound *domain.Inbound, user *domain.User, accessor *InboundStreamAccessor) (*proto.TypedMessage, error)
  }

  type AccountRegistry struct {
      builders map[string]AccountBuilder
  }

  func RegisterAccountBuilder(b AccountBuilder)
  func BuildAccountMessage(inbound *domain.Inbound, user *domain.User) (*proto.TypedMessage, error)
  ```
- **内置策略**：
  - `VLESSAccountBuilder`：利用 `accessor.ResolveVisionFlow()` 注入 flow；
  - `VMessAccountBuilder`：构建 `proto.VMessAccount`（AUTO 安全加密）；
  - `TrojanAccountBuilder`：构建 `proto.TrojanAccount`；
  - `ShadowsocksAccountBuilder`：利用 `accessor.ResolveShadowsocksMethod()` 映射 Protobuf `CipherType`。

### 2.4 订阅格式化通用原语：`internal/protocol/helpers.go`
- **目标**：消除各协议 `FormatLink`、`ToClash`、`ToSingBox` 中的重复代码。
- **新增通用原语**：
  - `ValidateBaseNode(node *NodeConfig) error`：
    统一校验 `node == nil`, `Address == ""`, `Port <= 0 || Port > 65535`, `UUID == ""`，消除每个协议文件开头的冗余样板代码。
  - `AttachClashTLS(proxy map[string]interface{}, node *NodeConfig)`：
    统一注入 TLS / REALITY / Fingerprint / Skip-Cert-Verify 等通用 Clash 属性。
  - `AttachSingBoxTLS(outbound map[string]interface{}, node *NodeConfig)`：
    统一注入 Sing-box 的 TLS / REALITY 结构体。
  - `BuildNodeQueryParams(node *NodeConfig, customReserved ...string) url.Values`：
    统一安全抽取未被保留的额外动态参数并放入 URL Query。
- **协议精简**：
  - 重构 `vless.go`, `trojan.go`, `vmess.go`, `hysteria2.go`, `shadowsocks.go`，直接复用上述原语。

### 2.5 前端死代码清洗与模型收敛
- **目标**：清理前端历史遗留的未用代码，保持整洁。
- **文件**：`web/src/views/users/services/subscription.ts`
  - 剔除 `safeBtoa`、`generateNodeLink` 等无人调用的私有生成逻辑；
  - 仅保留 `getDirectTokenSubUrl` 与 `getNodeTags`，注释清晰标注职责。

---

## 3. 测试与验证策略 (Testing & Verification)

1. **单元测试伴随**：
   - 为 `crypto/reality.go` 编写 `reality_test.go`（测试密钥生成格式、公私钥对应关系、UUID 掩码替换）；
   - 为 `stream_accessor.go` 编写 `stream_accessor_test.go`（测试 TCP/WS/gRPC/xHTTP 及 Vision Flow 各种组合条件）；
   - 为 `account_builder` 编写 `account_test.go`（验证 VLESS、VMess、Trojan、Shadowsocks 的 TypedMessage 正确性）；
   - 保留并更新现有的 `protocol_test.go`、`compiler_test.go`、`node_converter_test.go`，确保所有既有订阅输出 100% 回归通过。
2. **全流程质量门禁**：
   - `go test ./...`
   - `go vet ./...`
   - `go build .`
   - `npm run build`
