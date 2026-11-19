# Directory Structure & Layer Boundaries

> xray-panel 后端目录分层与职责边界

---

## 分层与调用规则

1. **`internal/domain` (纯领域模型与规则)**
   - 包含核心数据结构（如 `User`, `Inbound`, `Outbound`, `Ticket`）与纯函数规则；
   - **绝对禁止包含任何网络 I/O、数据库操作或外部框架/适配器依赖**（严禁将外部协议或核心特有 DTO，例如 Xray RealitySettings、XHTTPSettings、Vision 流控等泄漏进 domain；外部专用流解析器如 `InboundStreamAccessor` 必须收敛在 `internal/adapter/xray`）；
   - 必须保持 100% 单元测试可测性。

2. **`internal/service` (业务用例编排)**
   - 组装 domain 模型与 adapter 接口；
   - 处理跨领域事务、锁控制、动态生效（如通知 Xray-core gRPC 与更新数据库）；
   - 依赖 Repository 接口而非具体数据库实现。

3. **`internal/adapter` (技术适配器)**
   - 具体实现存储（GORM SQLite）、Xray 通信（gRPC）、Telegram Bot 轮询；
   - 负责网络协议转换与外部错误隔离；
   - **Xray 协议适配中心**：所有协议配置编译（`compiler.go`）与动态 gRPC 下发（`account_builder.go`）必须通过统一的 `ProtocolAdapter` 策略中心派发，杜绝在各处硬编码协议 switch 分支；Shadowsocks 协议支持区分传统 AEAD 与 SS2022 Multi-user (Sub-Key) 模式。

4. **`internal/delivery/http` & `cron` (接入交付)**
   - HTTP Handler 负责反序列化、入参校验（Validator）并调用 Service；
   - 统一错误返回格式，禁止在 Handler 中直写复杂业务运算；
   - Agent 节点控制面路由（`/api/agent/*`）必须独立使用节点级 Bearer Token 中间件（按 SHA-256 校验，仅注入当前 `node_id`），严禁与管理员 `JWTAuth` 边界混用。

5. **`cmd/agent` & `internal/agent` (独立极瘦节点代理)**
   - 独立编译为低内存节点端二进制（`go build ./cmd/agent`），驱动官方原版 Xray 二进制并主动 HTTPS 短轮询 Master；
   - **依赖隔离红线**：`go list -deps ./cmd/agent` **严禁**引入 `gin`、`gorm`、`glebarez/sqlite`、`telegram` 或任何 `web/dist` 前端嵌入；
   - **单机兼容与幂等红线**：`Inbound.NodeID == 0` 固定代表 Master 本机（Local Node），本机编译/巡检必须过滤 `NodeID == 0`；远端流量上报必须通过本地 spool + `(node_id, batch_id)` 保证跨网重试严格幂等。

---

## 命名约定

- 文件名一律使用小写加下划线：`user_repo.go`, `handler_inbound.go`, `reality_sync.go`；
- 测试文件必须与被测文件同包并在同一目录下：`xxx_test.go`；
- 接口定义通常由消费者定义（KISS 原则：不要为了形式主义给每个 struct 凭空套一层只有一个实现的 interface）。
