# Optimize log parsing with zero-allocation tokenizer

## Goal

将 Xray 日志解析模块（`internal/adapter/xray/log_reader.go`）中的硬编码低效正则表达式（`accessLogRe` 与 `errorLogRe`）替换为基于分词扫描（Tokenizer/Scanner）的高性能零堆分配解析方案，大幅降低高并发与大体积日志读取过滤时的 CPU 与 GC 堆分配开销，并建立详尽的基准测试（Benchmark）与回归测试。

## Requirements

1. **Access 日志解析器优化 (`ParseAccessLogLine` & `ParseLogLineFast`)**：
   - 使用基于定位锚点（时间戳固定长度、空格、`accepted`/`rejected`、路由标签方括号 `[...]`、`email:`）的 Tokenizer 替代 `accessLogRe` 正则表达式。
   - 完全兼容现有各种边界场景：
     - 带/不带毫秒的时间戳 (`2026/09/12 10:00:01` 或 `2026/09/12 10:00:01.123`)；
     - 来源地址：支持带/不带 `from `、带/不带 `tcp:` 或 `udp:` 前缀、IPv4 及 IPv6 地址与端口；
     - 目标地址与协议：支持 `tcp:...`、`udp:...` 自动识别与协议剥离；
     - 路由规则：支持 `[inbound -> outbound]`、`[inbound >> outbound]` 以及单 tag `[inbound]`；
     - 匿名/带邮箱：支持带 `email: user@domain` 与省略 email 的匿名连接；
     - 特殊协议日志：保留对 DoH / `answer:` / `dns-out` 等 DNS 规则行的解析支持。
   - 修复 `ParseLogLineFast` 中的设计割裂，消除 `string(trimmed)` 拷贝以及对正则的间接依赖，真正实现对 `[]byte` 切片的零堆分配元信息提取。

2. **Error 日志解析器优化 (`ParseErrorLogLine`)**：
   - 使用确定性字符串切分与状态扫描替代 `errorLogRe` 正则表达式。
   - 完全兼容各种级别（如 `WARNING` 映射为 `WARN`、`INFO`、`ERROR`）、可选进程 PID（`[12345]` 格式）、模块名与冒号消息、以及内置的 SmartTip 规则。

3. **基准测试与性能门禁 (Benchmark & Regressions)**：
   - 新增 `BenchmarkParseAccessLogLine` 与 `BenchmarkParseErrorLogLine` 以及 `BenchmarkParseLogLineFast`。
   - 确保性能相比原正则版本有显著提升（吞吐量大幅提升，单次操作堆分配量降至最低或仅保留目标结构体本身）。
   - 现有的 `log_reader_test.go` 所有单元测试与边界测试 100% 通过，并补齐带毫秒、IPv6、PID 错误日志等边界用例。

## Acceptance Criteria

- [ ] `ParseAccessLogLine`、`ParseLogLineFast`、`ParseErrorLogLine` 不再依赖 `accessLogRe` 与 `errorLogRe` 正则匹配。
- [ ] 现有测试套件 `log_reader_test.go` 完全通过，且新增用例覆盖 IPv6、带毫秒、带 PID 等所有原正则所覆盖的边缘场景。
- [ ] 编写 Benchmark，验证解析性能（吞吐与内存分配）对比正则有数量级优势。
- [ ] 项目全局质量门禁通过：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .` 均全绿通过。

## Constraints

- 严格保持对业务层对外暴露的 `*AccessLogEntry` 与 `*ErrorLogEntry` 结构体字段契约一致，不破坏上层 Service 调用逻辑。
- 零第三方依赖引入，纯标准库 `strings` / `bytes` 实现。
