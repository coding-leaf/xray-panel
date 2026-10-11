# PRD: 用户删除持久化顺序治理、Saga独立超时补偿与端到端重启一致性回归

## 1. 背景与问题定义 (Problem Statement)
测试与代码审计发现当前系统在用户生命周期（尤其是删除路径）存在严重的跨系统三态不一致与幽灵数据缺陷：
1. **P0 删除持久化顺序错位导致幽灵用户复活**：
   `UserService.DeleteUser` 原执行顺序为：`RemoveUser (gRPC)` ➔ `SyncUserToFile` ➔ `userRepo.Delete (DB)`。由于 `SyncUserToFile` 内部通过 `s.userRepo.ListAll` 读取用户重新编译 `config.json`，在第 2 步执行时该用户仍存在于数据库中，导致该用户被重新写入 `config.json`。面板重启调用 `SyncFromFile` 时，该用户从物理文件被重新加载并写回数据库，导致被删用户“死而复生”。
2. **P0 最后一个有效用户删除后编译器错误保留旧 clients**：
   在 `compiler.go` 中，当活跃用户数降为 0 时，若 `settingsMap["clients"]` 中原存在 clients，编译器错误命中了“保留原本存在于 settingsJson 的 clients”分支，导致物理配置文件中无法清空 clients。
3. **P1 Saga 反向补偿依赖原请求 Context**：
   用户创建、更新、删除等路径的补偿逻辑直接复用入参 `ctx`。当外部调用由于超时或客户端主动断开产生 `context.Canceled` 时，后续 Saga 补偿（gRPC/DB 回滚）会被运行时直接中止，导致补偿机制形同虚设。
4. **P1 删除用户多节点部分失败缺乏运行态补偿**：
   用户关联多节点时，若第 1 个节点移除成功但后续节点失败，流程直接中断退出，已移除的节点未被反向 `AddUser` 恢复，产生节点脱钩状态。
5. **P1 启动配置加载失败缺乏明确降级防御**：
   `main.go` 在启动应用配置失败时仅打印日志，缺乏明确的降级防御与状态指示。
6. **P2 CI 内核测试版本陈旧**：
   CI 中 Xray-core 下载安装版本需更新支持最新目标内核版本。

---

## 2. 需求与功能范围 (Requirements)

### 2.1 P0: 删除用户持久化顺序重构与双重防复活
- **执行顺序重构**：重构 `DeleteUser` 状态机执行流：
  1. 移除 Xray 运行态节点（记录已移除节点标签用于失败补偿）；
  2. 从 SQLite 数据库删除用户记录（保留内存快照）；
  3. 执行 `SyncUserToFile` 写回物理配置文件（由于数据库中已无该用户，编译输出必然干净）；
  4. 清理内存速度追踪器。
- **Saga 完整恢复链**：
  - 若第 1 步多节点移除过程中某节点报错，立即对已移除节点执行反向 `AddUser` 并阻断；
  - 若第 2 步数据库删除失败，立即对已移除节点执行反向 `AddUser` 并阻断；
  - 若第 3 步物理文件同步失败，立即通过独立补偿上下文向数据库重写回该用户快照，并向 Xray 反向 `AddUser` 恢复，返回错误，确保三态原子性。

### 2.2 P0: 编译器客户端保留机制修正与面板管理隔离
- 重构 `internal/adapter/xray/compiler.go` 中 `clients` 组装逻辑：
  - 严禁在面板活跃用户清零时盲目保留 `settingsJson` 中旧面板用户凭证；
  - 当入站由面板协议适配器管理且活跃用户为 0 时，准确下发空 clients 列表（`[]XrayClient{}`），彻底从 `config.json` 抹除被删用户。

### 2.3 P1: Saga 独立限时超时上下文 (Decoupled Compensating Context)
- 全面治理 `UserService` 中的 Saga 补偿上下文，由原本脆弱的 `ctx` 升级为：
  ```go
  compCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
  defer cancel()
  ```
- 覆盖 `CreateUser`、`UpdateUser`、`ResetSubToken`、`DeleteUser` 等所有存在补偿回滚的路径，抵御客户端超时断开导致的补偿失效。

### 2.4 P1: 启动配置应用失败自愈与降级指示
- 规范化 `main.go` 启动流程：当启动编译配置失败时，尝试回滚至最近一份已记录的良好快照（Snapshot），若依然失败则记录显式降级状态标识。

### 2.5 P2: CI Xray 内核测试环境增强
- 更新 `.github/workflows/ci.yml` 中的 Xray-core 下载版本至最新生产稳定版本（v25+），确保与现代 Xray 特性及日志格式严格契约兼容。

### 2.6 核心回归测试：删除用户重启一致性集成测试
- 新增真实组件端到端集成测试（`internal/service/user_delete_consistency_test.go`）：
  - 使用真实的 SQLite（内存/临时文件）、真实 `ConfigService`、真实 `XrayCompiler` 与临时 `config.json` 物理文件；
  - 步骤：
    1. 创建一个具有有效 UUID 的用户；
    2. 校验数据库与 `config.json` 物理文件均包含该用户；
    3. 调用 `DeleteUser` 删除该用户；
    4. 断言 SQLite 与 `config.json` 物理文件均已彻底清除该用户；
    5. 调用 `configSvc.SyncFromFile` 模拟面板冷重启；
    6. 断言用户在 SQLite 中绝不重新出现（0 用户）。

---

## 3. 验收标准 (Acceptance Criteria)

- [ ] **AC-1 (端到端重启一致性)**：集成回归测试真实验证“创建 -> 落盘 -> 删除 -> 物理与DB双清空 -> SyncFromFile 重启不复活”100% 绿灯。
- [ ] **AC-2 (编译器 Clients 零残留)**：当入站唯一用户被删除时，编译后的 `config.json` 中该入站的 `clients` 为 `[]`，无任何历史残留。
- [ ] **AC-3 (删除失败 Saga 完整补偿)**：单测验证当删除过程中写盘失败时，SQLite 数据库用户被成功恢复，且 Xray 已移除的节点被成功重新注入。
- [ ] **AC-4 (独立超时 Context 鲁棒性)**：单测验证在 `ctx` 被提前 `cancel` 的极端场景下，Saga 补偿逻辑依旧能凭借独立 Context 成功执行清理与回滚。
- [ ] **AC-5 (质量门禁)**：`go test ./...`、`go vet ./...`、`go build .`、`npm run typecheck` 全绿通过。
