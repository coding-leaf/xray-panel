# Research — Backend Logging Conventions (read-only, 2026-09-29)

Scope: Go backend at repo root. Evidence is `file:line`.

## 1. Setup & construction

- Single logging package: `internal/pkg/logger/logger.go`.
- `logger.Init(level string, isJSON bool)` builds the logger and installs it globally
  via `slog.SetDefault` — `internal/pkg/logger/logger.go:18-45`.
  - Maps string level (`debug`/`warn`/`error`, default `info`) — `logger.go:19-29`.
  - `AddSource: true` always on — `logger.go:31-34`.
  - `slog.NewJSONHandler` or `slog.NewTextHandler`, both to `os.Stdout` — `logger.go:36-41`.
  - Package-level `var defaultLogger *slog.Logger`; `slog.SetDefault(defaultLogger)` — `logger.go:16,43-44`.
- `logger.FromContext(ctx)` returns default logger, enriched with `request_id` when present — `logger.go:47-56`.
- Only construction site: `main.go:39-40` (`logger.Init(cfg.LogLevel, cfg.LogJSON)`).

## 2. Library & format

- Standard library `log/slog` only. No zap / zerolog / logrus / lumberjack.
- Structured key/value; JSON or text selected by `LogJSON` (default text) — `internal/config/config.go:40`, `logger.go:36-41`.
- Observed keys: `error`, `version`, `port`, `xray_grpc`, `db_path`, `name`, `username`,
  `request_id`, `method`, `path`, `status`, `client_ip`, `latency`, `interval`, `email`,
  `inbound`, `server_name` (`main.go:42-47`, `middleware/logger.go:32-39`).
- Errors dominant idiom: `slog.String("error", err.Error())` (`main.go:52`);
  exception: `slog.Any("error", err)` in panic recovery (`middleware/logger.go:48`).

## 3. Levels actually used

- `Fatal`: never used. `Debug`: never used (explicit negative finding).
- Counts: Info ≈ 34, Warn ≈ 5, Error ≈ 11–12.

Info:
- `main.go:42` `"Starting Xray Decoupled Panel"`
- `internal/delivery/http/middleware/logger.go:32` `"HTTP Request"` (access log)
- `internal/delivery/cron/traffic_sync.go:65` `"Traffic sync job started"`

Warn:
- `main.go:95` `"Failed to persist JWT secret to database"`
- `internal/delivery/cron/reality_sync.go:49` `"Reality sync job initial inspection completed with error"`
- `internal/adapter/telegram/bot.go:85` `"Failed to sync telegram bot cloud commands"`
- `internal/service/alert_service.go:76` (via `logger.FromContext`)

Error:
- `main.go:52` `"Failed to init SQLite"` (+ `os.Exit(1)` at `main.go:53`)
- `internal/delivery/http/middleware/logger.go:47` `"HTTP Panic Recovered"`
- `internal/delivery/http/server.go:85` `"HTTP server failed to listen or serve"`

Context-logger sites: `alert_service.go:76,203`, `internal/adapter/xray/config_parser.go:67`,
`internal/adapter/telegram/notifier.go:97`, `reality_monitor_service.go:194`.

## 4. What is logged / redaction

- Logged: lifecycle/startup/shutdown (`main.go:42-236`); per-request access metadata
  (`middleware/logger.go:32-39`); panics (`middleware/logger.go:43-57`); cron job
  lifecycle (`traffic_sync.go`, `reality_sync.go`); telegram bot lifecycle
  (`bot.go`, `notifier.go`); alert failures incl. user **email**
  (`alert_service.go:76`) and inbound/server_name (`reality_monitor_service.go:194-198`).
- No dedicated redaction/masking helper exists.
- JWT secret value never passed to slog — only outcome strings (`main.go:90,97`).
- Telegram token never logged; only bot `username` (`notifier.go:40`, `bot.go:87`).
- `client_ip` logged unredacted (`middleware/logger.go:37`, via `GetRealClientIP`
  `middleware/limiter.go:115-134`). `email` logged unredacted.
- Secret suppression is at the API layer, not logging: `setting_service.go:55,61`
  (deletes/skips `jwt_secret`), locked by `handler_setting_test.go:48-104`,
  `setting_service_test.go:73-116`.
- No request-body logging, no Debug payload logging.

## 5. Singleton vs injected

- Package-level `slog` singleton dominates (~50 direct call sites across `main.go`,
  `server.go`, cron, telegram).
- `logger.FromContext(ctx)` only 5 production sites — most logs lack `request_id`.
- No `*slog.Logger` struct-field DI pattern.

## 6. Output, rotation, config

- Destination: always `os.Stdout` (`logger.go:38,40`). No file sink, no MultiWriter.
- Rotation: none (no lumberjack / WriteSyncer / log-file open).
  (`internal/adapter/xray/log_reader.go:72`, `internal/service/log_service.go:109`
  manipulate the *Xray core* log, unrelated to app logging.)
- Color: none. No `ReplaceAttr`.
- Config: `LogLevel` from `-log-level`, env `LOG_LEVEL`, default `"info"`
  (`internal/config/config.go:39`); `LogJSON` from `-log-json`, default `false` (`config.go:40`).
  Accepted levels: `debug`, `info` (default), `warn`, `error` (`logger.go:19-29`).
- Note: `LogLevel` fields in `internal/adapter/xray/schema.go:20`, `compiler.go:39`,
  `internal/sub/clash.go:17` are Xray/clash config, not the Go logger.

## Explicit negative findings

No Debug, no Fatal, no third-party logger, no rotation/file/color, no redaction helper,
no injected `*slog.Logger` struct fields.

## Evidence file list

- `internal/pkg/logger/logger.go:16,18-45,47-56`
- `main.go:39-40,42-47,52-53,90,95,97`
- `internal/config/config.go:18-19,39-40`
- `internal/delivery/http/middleware/logger.go:15-41,43-57`
- `internal/delivery/http/middleware/limiter.go:115-134`
- `internal/delivery/http/server.go:83,85,100,106,110`
- `internal/delivery/cron/traffic_sync.go:65,100,106,212`; `reality_sync.go:40,47,49,61,64,69`
- `internal/adapter/telegram/bot.go:85,87,109-207`; `notifier.go:40,79,97`
- `internal/service/alert_service.go:76,203`; `reality_monitor_service.go:194-198`
- `internal/adapter/xray/config_parser.go:67`; `log_reader.go:16-17,72`; `log_service.go:109`
- `internal/service/setting_service.go:55,61`
