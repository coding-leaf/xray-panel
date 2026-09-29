# Plan: 后端数据流与并发性能优化 - 实施计划

- **关联 Spec**: backend-dataflow-perf
- **实施执行人 / Agent**: Dev
- **当前状态**: Draft
- **Change Tier**: Tier 2 (局部结构精简与并发性能优化)

---

## 1. 变更文件清单 (Files that change)

### 核心实现文件
* `internal/domain/user_speed.go` (Modify: 增加批量聚合更新 `BatchUpdateUserRuntimeSpeeds`、顺带扫描淘汰过期重置项)
* `internal/delivery/cron/traffic_sync.go` (Modify: 调用端切换为单次批量聚合更新，根除 N 次锁循环颠簸)
* `internal/delivery/http/middleware/limiter.go` (Modify: 改造 `ipRateLimiter` 为 16 分段哈希锁结构 `shardedIPRateLimiter`)
* `internal/adapter/xray/log_reader.go` (Modify: 引入 `sync.Pool` 复用 64KB 缓冲，反向切分零堆内存分配)
* `internal/service/monitor_service.go` (Modify: 增加 2 秒并发安全轻量级读写缓存，防高频全表扫库)

### 测试与验证文件
* `internal/domain/user_speed_test.go` (Modify: 补充并发批量更新与淘汰竞态测试)
* `internal/delivery/http/middleware/limiter_test.go` (Modify: 补充分段哈希锁并发压力测试)
* `internal/adapter/xray/log_reader_test.go` (Modify: 补充并发读取 buffer pool 复用测试)
* `internal/service/monitor_service_test.go` (New: 新增 Dashboard 缓存命中与失效过期逻辑测试)

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

### Milestone 1: 用户速率单次批量锁与内存清理 (M1)
* **涉及文件**: `internal/domain/user_speed.go`, `internal/delivery/cron/traffic_sync.go`, `internal/domain/user_speed_test.go`
* **Step 1.1 (Fail-repro First)**: 
  - 在 `user_speed_test.go` 编写 `TestBatchUpdateUserRuntimeSpeeds_Concurrency`，100 并发 goroutine 同时执行批量更新与读取；
  - 验证命令: `go test -race -v -run TestBatchUpdateUserRuntimeSpeeds_Concurrency ./internal/domain` (未实现前编译失败/变红)
* **Step 1.2 (核心逻辑实现)**:
  - 在 `user_speed.go` 定义 `UserTrafficDeltaUpdate` 结构体；
  - 实现 `BatchUpdateUserRuntimeSpeeds(deltas []UserTrafficDeltaUpdate, intervalSec int64, nowMs int64)`，单次获取 `Lock()`，原子执行增量更新、0流量清零及超时 `userResetTimes` 惰性淘汰；
  - 保持旧 API `SetUserRuntimeSpeed` 100% 兼容；
  - 局部验证命令: `go test -race -v -run TestBatchUpdateUserRuntimeSpeeds ./internal/domain` (变绿)
* **Step 1.3 (调用端装配)**:
  - 在 `traffic_sync.go` 的定时同步末尾，将针对每个用户单独调用的循环替换为单次 `BatchUpdateUserRuntimeSpeeds`；
  - 局部验证命令: `go test -race -v ./internal/delivery/cron` (通过)

### Milestone 2: 16 分段哈希限流锁与日志 64KB Buffer Pool 复用 (M2)
* **涉及文件**: `internal/delivery/http/middleware/limiter.go`, `internal/adapter/xray/log_reader.go`, `internal/delivery/http/middleware/limiter_test.go`, `internal/adapter/xray/log_reader_test.go`
* **Step 2.1 (限流器分段锁优化)**:
  - 在 `limiter.go` 中引入 16 分片结构 `shardedIPRateLimiter`，利用 FNV-1a 哈希将 IP 分散到 16 把互斥锁；
  - 编写 `TestShardedIPRateLimiter_Concurrent`，模拟 1000 并发请求对不同 IP 进行限流打点；
  - 局部验证命令: `go test -race -v -run Test.*RateLimiter ./internal/delivery/http/middleware`
* **Step 2.2 (日志扫描 Buffer Pool 与内存复用)**:
  - 在 `log_reader.go` 声明 `var logBufferPool = sync.Pool{ New: func() any { b := make([]byte, 64*1024); return &b } }`；
  - 在 `ReadLastLinesFiltered` 中通过 pool 获取与归还 64KB 缓冲区，改用 `bytes.LastIndexByte` 逆向定位换行符，避免无节制 `string(buf)` 堆拷贝与 `strings.Split` 分配；
  - 编写 `TestReadLastLinesFiltered_BufferPoolConcurrent` 验证并发读取安全无数据踩踏；
  - 局部验证命令: `go test -race -v -run TestReadLastLines ./internal/adapter/xray`

### Milestone 3: Dashboard 2秒并发安全轻量缓存 (M3)
* **涉及文件**: `internal/service/monitor_service.go`, `internal/service/monitor_service_test.go`
* **Step 3.1 (测试先行)**:
  - 新建 `internal/service/monitor_service_test.go`，使用 mock `UserRepository` 和 `InboundRepository` 记录 `ListAll` 被调用次数；
  - 编写 `TestMonitorService_DashboardCache`，并发发起 20 个请求，断言底层 Repo 仅被查询 1 次；等待 2.1s 后再次发起，断言重新查询；
  - 验证命令: `go test -race -v -run TestMonitorService_DashboardCache ./internal/service` (未加缓存前失败)
* **Step 3.2 (缓存实现与双检锁)**:
  - 在 `MonitorService` 内部增加 `cacheMu sync.RWMutex`、`cachedData *DashboardData`、`cacheExpiresAt time.Time`、`cacheTTL time.Duration` (默认 2s)；
  - `GetDashboardData` 采用读写锁双重检查机制（DCL）：先 RLock 检查未过期直接返回克隆指针；过期则 Lock 重新计算并存入；
  - 局部验证命令: `go test -race -v -run TestMonitorService_DashboardCache ./internal/service` (变绿)

### Milestone 4: 全局并发安全与回归验证 (M4)
* **涉及文件**: 全局相关模块测试
* **Step 4.1 (并发竞态与回归门禁)**:
  - 运行全量竞态检测自动化测试: `go test -race ./...` (必须 100% 全绿通过)
* **Step 4.2 (静态代码与死锁隐患检查)**:
  - 执行 `go vet ./...` (零告警)
* **Step 4.3 (构建与二进制完整性校验)**:
  - 执行 `go build .` (编译顺利通过，零符号缺失)

---

## 3. 回滚保护与应急预案 (Rollback & Protection)
1. **源码级瞬时回滚**: 所有改动均不涉及 DB Migration 和对外 API 结构破坏，如遇线上问题可执行 `git revert <commit-hash>` 秒级回滚。
2. **Dashboard 缓存穿透降级**: 若缓存偶发陈旧数据导致运维排查延迟，可为 `MonitorService` 增加 `DisableCache bool` 开关，直接透传穿透查询。
3. **分段锁降级兜底**: 16 分段锁仅是哈希隔离，如遇异常仅需将分片数收敛为 1 或直接复用原 `ipRateLimiter` 逻辑。

---

## 4. 全局质量门禁核验 (Global Quality Gate)
* **代码风格与静态检查**: `go vet ./...`
* **并发竞态与单元测试**: `go test -race ./...`
* **后端完整编译检查**: `go build .`
* **核验预期**: 全量测试 100% 通过，无竞态告警，无 goroutine 泄漏。

---

## 5. 实施偏差记录 (Deviations Log)
* 规范声明：spec/intent 中提及的部分路径为概念层别名（例如 `internal/adapter/http/middleware/limiter.go` 实际物理路径为 `internal/delivery/http/middleware/limiter.go`，`internal/adapter/system/log_reader.go` 实际为 `internal/adapter/xray/log_reader.go`），plan 已精准对齐实际工程代码树。

---

## 6. 阶段准出签批 (Gate 3 Sign-off)
- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **验证人 / 日期**: [待人类签批] / 2026-09-20 12:07
