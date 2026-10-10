# Technical Design: Zero-Allocation Log Tokenizer

## 1. 架构与设计目标

本重构属于 `internal/adapter/xray` 层面的性能优化，聚焦于替换 `log_reader.go` 中的硬编码正则表达式。

- **目标 1：消除正则引擎回溯与运行时分配**：移除 `accessLogRe` 与 `errorLogRe`。
- **目标 2：零拷贝/极低分配**：
  - `ParseLogLineFast`：直接在 `[]byte` 上使用 `bytes` 操作截取提取元数据，真正实现 0 内存堆分配。
  - `ParseAccessLogLine` / `ParseErrorLogLine`：仅在生成返回的结构体（`*AccessLogEntry` / `*ErrorLogEntry`）及其 string 字段时进行必要分配，解析逻辑过程 0 额外中间对象。
- **目标 3：100% 行为等价与向后兼容**。

---

## 2. 状态扫描器设计 (Tokenizer / Scanner)

### 2.1 Access 日志格式剖析与定界状态
标准日志模式：
`<Time>[.ms] [from ][tcp:|udp:]<FromIP> <Action> <Target> [<Route>][ email: <Email>]`

```
2026/09/12 10:00:01 127.0.0.1:1000 accepted tcp:example.com:443 [vless-in -> direct] email: alice@test.com
|-----------------| |------------| |------| |-----------------| |-------------------|        |--------------|
      Time              FromIP      Action        Target                 Route                      Email
```

#### 解析步骤：
1. **时间戳提取**：
   - 验证长度至少 19 字节，检查前 19 字符是否符合 `YYYY/MM/DD HH:MM:SS` 格式（数字与斜杠/冒号校验）。
   - 处理可选毫秒：若第 19 位为 `.`，跳过后续数字直到遇到空格；否则若为 `' '`，直接推进游标。
2. **FromIP 提取**：
   - 跳过前置空格；若紧接着以 `from ` 开头，跳过 `from `。
   - 若紧接着以 `tcp:` 或 `udp:` 开头，跳过该前缀。
   - 读取 FromIP 字符串，直到遇到空格。支持 IPv4 (`127.0.0.1:1234`) 与 IPv6 (`[::1]:1234`)。
3. **Action 判定**：
   - 检查下一段是否为 `accepted` 或 `rejected`。若不是，则不符合常规 access 格式（可回退到 DNS 特殊行判定）。
4. **Target 提取**：
   - 读取 Target，直到遇到包含方括号 `[` 的路由前缀。Target 前缀若为 `tcp:` 或 `udp:`，则记录对应协议并去除前缀，否则默认协议为 TCP。
5. **Route 提取**：
   - 寻找配对的 `]`，中间内容即为 Route。根据 `->` 或 `>>` 拆解为 `InboundTag` 和 `OutboundTag`。
6. **可选 Email 提取**：
   - 检查 `]` 之后是否存在 ` email: `，若存在则截取剩余部分作为 Email。
7. **DNS 特殊行兼容**：
   - 若不匹配上述常规模式，快速检查是否包含 `"DOH"`、`"answer:"` 或 `"dns-out"`，保留现有 DNS 解析逻辑。

### 2.2 Error 日志格式剖析
标准日志模式：
`<Time>[.ms] [<Level>] [ [<PID>] ] <Module>: <Message>`

```
2026/09/12 10:00:01 [Warning] [12345] Core: failed to process inbound
|-----------------| |-------|  |---|   |--|  |-----------------------|
      Time            Level     PID   Module          Message
```

#### 解析步骤：
1. 校验并提取时间戳及可选毫秒；
2. 提取第一个 `[` 与 `]` 之间的日志级别 `Level`（若为 `WARNING` 统一规范化为 `WARN`）；
3. 检查后续是否有可选的 `[` 数字 `]`（PID），若有则跳过；
4. 寻找冒号 `:`，冒号左侧为 `Module`，冒号右侧通过 `strings.TrimSpace` 提取为 `Message`；
5. 根据 `Message` 匹配现有的 SmartTip 规则。

---

## 3. 性能预期与验证

- **Benchmark 维度**：
  - `BenchmarkParseAccessLogLine` (原正则 vs 新扫描器)
  - `BenchmarkParseErrorLogLine` (原正则 vs 新扫描器)
  - `BenchmarkParseLogLineFast` (原正则/拷贝 vs 纯字节扫描)
- 预期吞吐量提升 5x - 10x，B/op（每次分配字节数）降低 50% - 80%，Allocs/op 从 10+ 次降为 1-2 次（仅返回结构体与必要 string）。
