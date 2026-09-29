# Logging Guidelines

> 结构化日志、等级使用现状与敏感凭据防线

本文件描述 **当前代码实际采用的日志方式**，而非理想方案。修改日志相关代码时必须与之对齐，
避免引入项目尚未使用的第三方日志库或抽象。

---

## 1. 日志库与构造

- **唯一日志包**：`internal/pkg/logger/logger.go`；底层为标准库 `log/slog`，
  全仓无 zap / zerolog / logrus / lumberjack 依赖。
- **唯一初始化点**：`main.go:39-40` 调用 `logger.Init(cfg.LogLevel, cfg.LogJSON)`。
- `logger.Init` 行为（`internal/pkg/logger/logger.go:18-45`）：
  - 将字符串等级映射为 `slog.Level`（`debug` / `info` / `warn` / `error`，默认 `info`）—— `logger.go:19-29`；
  - 无条件开启 `AddSource: true`，日志始终携带 `file:line` —— `logger.go:31-34`；
  - 按 `isJSON` 选择 `slog.NewJSONHandler` 或 `slog.NewTextHandler`，**两者都写 `os.Stdout`** —— `logger.go:36-41`；
  - 通过 `slog.SetDefault(defaultLogger)` 安装为全局默认 logger —— `logger.go:16,43-44`。

---

## 2. 注入与共享方式

- **包级 `slog` 单例是主导模式**：约 50 处直接调用 `slog.Info/Warn/Error`，
  分布在 `main.go`、`internal/delivery/http/server.go`、`internal/delivery/cron/*`、
  `internal/adapter/telegram/*`。
- **无 `*slog.Logger` 结构体字段注入**：不存在 `logger *slog.Logger` 的 DI 构造函数模式，
  `logger` 仅以包形式导入。
- `logger.FromContext(ctx)` 为少数派，仅 5 处生产调用：
  `internal/service/alert_service.go:76,203`、`internal/adapter/xray/config_parser.go:67`、
  `internal/adapter/telegram/notifier.go:97`、`internal/service/reality_monitor_service.go:194`。
  它在 ctx 中存在 `request_id` 时为其追加该字段（`logger.go:47-56`）。

> **已知现状（非缺陷，但需知晓）**：`request_id` 由 HTTP 中间件注入
> （`internal/delivery/http/middleware/logger.go:23`），但大多数层直接调用 `slog.*`，
> 因此**多数日志不携带 `request_id`**，请求级串联只在那 5 个 `FromContext` 站点生效。

---

## 3. 等级使用现状

| 等级 | 现状 |
|------|------|
| `Debug` | **全仓未使用**（无任何 `slog.Debug(` 调用点） |
| `Info` | 约 34 处，记录生命周期与正常流转 |
| `Warn` | 约 5 处，降级但可继续的场景 |
| `Error` | 约 11–12 处，失败且需关注 |
| `Fatal` | **从不使用**；以 `slog.Error(...)` + `os.Exit(1)` 代替（`main.go:52-53`） |

**Info 示例**
- `main.go:42` — 应用启动 `"Starting Xray Decoupled Panel"`（携带 version / port / xray_grpc / db_path）
- `internal/delivery/http/middleware/logger.go:32` — 每请求 access log `"HTTP Request"`
- `internal/delivery/cron/traffic_sync.go:65` — 任务启动 `"Traffic sync job started"`

**Warn 示例**
- `main.go:95` — `"Failed to persist JWT secret to database"`
- `internal/delivery/cron/reality_sync.go:49` — `"Reality sync job initial inspection completed with error"`
- `internal/adapter/telegram/bot.go:85` — `"Failed to sync telegram bot cloud commands"`

**Error 示例**
- `main.go:52` — `"Failed to init SQLite"`
- `internal/delivery/http/middleware/logger.go:47` — `"HTTP Panic Recovered"`
- `internal/delivery/http/server.go:85` — `"HTTP server failed to listen or serve"`

---

## 4. 标准字段与错误属性写法

- 采用结构化 key/value（`slog.String` / `slog.Int` / `slog.Duration` / `slog.Any`）。
- 已出现的字段名：`error`, `version`, `port`, `xray_grpc`, `db_path`, `name`, `username`,
  `request_id`, `method`, `path`, `status`, `client_ip`, `latency`, `interval`, `email`,
  `inbound`, `server_name`（`main.go:42-47`、`internal/delivery/http/middleware/logger.go:32-39`）。
- **错误属性统一写字符串**：
  ```go
  slog.Error("Failed to init SQLite", slog.String("error", err.Error()))
  ```
  例外：panic 恢复路径使用 `slog.Any("error", err)`（`middleware/logger.go:48`）。
- 消息文案使用英文短语，首字母大写，不自造中文日志消息。

---

## 5. 记录内容与敏感信息防线

**会记录**
- 生命周期与编排：启动、各服务起停、优雅停机、清理阶段（`main.go:42-236`）；
- HTTP 访问元数据：`request_id` / `method` / `path` / `status` / `client_ip` / `latency`（`middleware/logger.go:32-39`）；
- panic 与恢复（含恢复值与 path）—— `middleware/logger.go:43-57`；
- 后台任务：流量同步、Reality 同步的起停与失败（`internal/delivery/cron/*`）；
- Telegram Bot 生命周期与云指令同步（`internal/adapter/telegram/*`）；
- 告警投递失败，含用户 `email` 与 inbound / server_name（`alert_service.go:76`、`reality_monitor_service.go:194-198`）。

**敏感信息处理（现状）**
- **无通用脱敏 helper**：不存在 `redact` / `mask` / `sanitiz` 之类的日志脱敏函数。
- JWT secret 的**值从不进入 slog**：仅记录结果字符串
  `"Generated and persisted secure high-entropy JWT secret"`（`main.go:97`），失败仅记录 `err.Error()`。
- Telegram Bot Token 从不记录，日志只输出 bot `username`（`notifier.go:40`、`bot.go:87`）。
- 密钥防泄漏真正落在 **API 层而非日志层**：`GetAllSettings` 返回前删除 `jwt_secret`
  （`internal/service/setting_service.go:55`），`UpdateSettings` 跳过该字段（`setting_service.go:61`），
  并由测试锁定（`handler_setting_test.go:48-104`、`setting_service_test.go:73-116`）。
- **明文记录项（刻意可见，非 bug）**：`client_ip` 未脱敏
  （`middleware/logger.go:37`，经 `GetRealClientIP` `middleware/limiter.go:115-134`）；
  用户 `email` 未脱敏（`alert_service.go:76`）。

**不记录**
- 无请求体日志；无密码 / JWT / Token / 提件码进入 slog；无 `Debug` 级 payload 输出。

> **红线**：新增日志时不得把 secret、Token、密码、提件码明文写入属性值。

---

## 6. 输出目标与配置

- **输出**：始终 `os.Stdout`（`logger.go:38,40`）；无文件 sink、无 `MultiWriter`。
- **轮转**：无（无 lumberjack / `WriteSyncer` / 日志文件句柄）。
  `internal/adapter/xray/log_reader.go:72`、`internal/service/log_service.go:109` 操作的是
  **Xray 核心自身日志**，与面板应用日志无关。
- **颜色**：无 ANSI 着色，无 `ReplaceAttr`。
- **配置来源**（`internal/config/config.go:39-40`）：
  - `LogLevel` ← 参数 `-log-level`，默认读取环境变量 `LOG_LEVEL`，缺省 `"info"`；
  - `LogJSON` ← 参数 `-log-json`，默认 `false`（无环境变量覆盖）；
  - 可接受等级：`debug` / `info`（默认）/ `warn` / `error`。

> 注意：`internal/adapter/xray/schema.go:20`、`compiler.go:39`、`internal/sub/clash.go:17`
> 中的 `LogLevel` 属于 Xray-core / Clash 配置，**不是**面板 Go logger 的配置。

---

## 7. 证据锚点

`internal/pkg/logger/logger.go:16,18-45,47-56`；`main.go:39-40,42-47,52-53,90,95,97`；
`internal/config/config.go:18-19,39-40`；`internal/delivery/http/middleware/logger.go:15-41,43-57`；
`internal/delivery/http/middleware/limiter.go:115-134`；`internal/delivery/http/server.go:83,85`；
`internal/delivery/cron/traffic_sync.go:65,212`；`reality_sync.go:40,47,49,61,64,69`；
`internal/adapter/telegram/bot.go:85,87`；`notifier.go:40,79,97`；
`internal/service/alert_service.go:76,203`；`reality_monitor_service.go:194-198`；
`internal/adapter/xray/config_parser.go:67`；`internal/service/setting_service.go:55,61`。
