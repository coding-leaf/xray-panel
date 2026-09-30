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
   - 统一错误返回格式，禁止在 Handler 中直写复杂业务运算。

---

## 命名约定

- 文件名一律使用小写加下划线：`user_repo.go`, `handler_inbound.go`, `reality_sync.go`；
- 测试文件必须与被测文件同包并在同一目录下：`xxx_test.go`；
- 接口定义通常由消费者定义（KISS 原则：不要为了形式主义给每个 struct 凭空套一层只有一个实现的 interface）。
