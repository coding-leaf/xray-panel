# 测试缺陷与质量门禁全面治理 PRD

## Goal

响应并彻底治理测试团队反馈的问题清单，消除 UserService 用户生命周期管理中的静默吞错（`_ =`），建立面向 SQLite、Xray gRPC 内存和 `config.json` 物理配置的 Saga 反向补偿回滚机制与故障注入单元测试；治理服务启动阶段的静默吞错；补齐 CI 门禁中的前端类型检查（`vue-tsc`）与真实 Xray-core 集成测试环境，修正旧版本号硬编码。

## Requirements

### P0 核心架构与业务一致性
1. **消除 UserService 静默吞错**：
   - 彻底清除 `UserService.CreateUser`、`UpdateUser`、`DeleteUser`、`ResetTraffic`、`ResetSubToken` 中的 `_ = s.xrayManager...` 与 `_ = s.configSvc.SyncUserToFile...`。
   - 所有外部操作产生的错误必须显式向上返回，并携带精准的上下文（使用 `%w` 包裹）。
2. **应用层 Saga 反向补偿机制（一滚全滚）**：
   - **CreateUser**：若 Xray 注入失败或配置文件落盘失败，必须反向补偿：逆序调用 `RemoveUser` 清理已加入的 Xray 入站，并调用 `userRepo.Delete` 彻底删除刚插入的 SQLite 记录，杜绝残留孤儿数据，允许用户后续使用同名邮箱重试创建。
   - **UpdateUser**：在执行变动前获取前置状态快照 `oldUserSnapshot`。若后续 Xray 刷新或配置文件落盘失败，必须反向补偿：将 Xray 节点状态与 SQLite 数据精确回滚至修改前快照。
   - **ResetSubToken**：若更新 Xray 节点凭证或写盘失败，反向补偿写回原 UUID 和 SubToken，还原 Xray 运行态挂载，防止凭证半更新导致客户端断网且无法恢复。
   - **DeleteUser**：重构删除步骤顺序为“Xray 热剔除 ➔ 物理文件剔除 ➔ 数据库彻底删除”，前置步骤发生致命网络错误时阻止破坏性删除并报错（对 Xray 用户不存在保持幂等容忍）。
   - **批量操作（BatchRenew、BatchResetTraffic、BatchSetStatus）**：消除循环内静默 `continue` 或吞错，聚合收集所有失败项的 ID 与错误原因并返回，使上层能感知部分失败。

### P1 启动容灾与故障注入测试
1. **服务启动阶段错误治理**：
   - 在 `main.go` 启动时，移除 `_ = configSvc.SyncFromFile` 与 `_ = configSvc.RecompileAndApply` 的静默吞错。
   - 对启动阶段同步或内核编译重载失败进行显式错误判断与结构化告警日志（`slog.Error`），防止主程序带着未知的脏状态静默启动。
2. **UserService 故障注入单元测试覆盖**：
   - 为 `UserService` 引入轻量级接口抽象，解除对 `*ConfigService` 具体结构体的强耦合，使其具备无副作用的可测试性。
   - 编写具备调用轨迹追踪和指定故障注入能力的 Mock 套件。
   - 编写针对 `CreateUser`、`UpdateUser`、`ResetSubToken` 在 Xray 报错和写盘报错场景下的故障注入测试，断言系统正确触发反向补偿与数据库回滚。

### P2 CI 质量门禁加固与技术债务
1. **CI 前端类型检查门禁**：
   - 在 `.github/workflows/ci.yml` 中接入 `npm run typecheck`（`vue-tsc --noEmit`），确保在构建前拦截所有深层 TypeScript 类型错误。
2. **CI 真实 Xray 集成测试环境**：
   - 在 CI 的 `test` 任务中，下载官方对应架构的 Xray-core 二进制并配置至系统 PATH，使 `internal/adapter/xray/xray_integration_test.go` 中的 `TestXrayCore_GRPC_RealBinary` 不再被跳过，真正在 CI 容器中跑通。
3. **消除旧版本号硬编码**：
   - 修正 CI 构建命令中硬编码的 `-X 'main.Version=v1.3.0-ci'`，使其与当前源码主版本事实源（`v2.6.2`）对齐。

## Acceptance Criteria

### P0 & P1（核心一致性与测试）
- [ ] `UserService` 内部所有 CRUD、Reset 及 Batch 操作中不再存在针对 `xrayManager` 和 `configSvc` 的 `_ =`。
- [ ] 针对 `CreateUser` 编写故障注入测试：Xray 注入失败时，返回错误且 SQLite 中已写入的用户被干净删除。
- [ ] 针对 `CreateUser` 编写故障注入测试：配置文件持久化失败时，返回错误，Xray 内存被调用 `RemoveUser` 回滚，SQLite 记录被删除。
- [ ] 针对 `UpdateUser` 编写故障注入测试：Xray 失败或写盘失败时，返回错误且 SQLite 中的用户信息被完全还原为原始快照。
- [ ] 针对 `ResetSubToken` 编写故障注入测试：更新失败后原 UUID 与 Token 未被破坏。
- [ ] `main.go` 启动时的配置同步与编译不再静默吞错，失败时有明确的日志告警与安全处理。
- [ ] 全套后端单元测试通过：`mise x -- go test ./...` 绿灯，`mise x -- go vet ./...` 无警告。

### P2（CI 质量门禁）
- [ ] CI 配置文件 `.github/workflows/ci.yml` 中包含前端 `npm run typecheck` 步骤。
- [ ] CI 配置文件中包含安装 Xray-core 二进制的步骤，集成测试可以在 Linux CI 环境真实运行。
- [ ] CI 中的构建注入版本号更新为最新版规范（如读取代码版本或 `v2.6.2-ci`）。
- [ ] 前端类型检查与构建通过：`mise x -- npm --prefix web run typecheck` 和 `mise x -- npm --prefix web run build` 均绿灯。
