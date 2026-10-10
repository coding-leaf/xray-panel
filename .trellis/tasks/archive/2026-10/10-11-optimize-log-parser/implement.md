# Implementation Plan: Optimize log parsing with zero-allocation tokenizer

## 1. 实施阶段与步骤

- [ ] **Step 1: 先行编写基准测试与现有边界测试（Benchmark & Test Baseline）**
  - 在 `internal/adapter/xray/log_reader_test.go` 中增加当前正则解析器的 Benchmark 函数：
    - `BenchmarkParseAccessLogLine`
    - `BenchmarkParseErrorLogLine`
    - `BenchmarkParseLogLineFast`
  - 补充测试用例：覆盖毫秒时间戳、IPv6 地址、`from ` 前缀、单 Tag 路由、带 PID 的 Error 日志等所有边缘情况。
  - 运行当前基准测试并记录 baseline 性能数据。

- [ ] **Step 2: 实现纯 Tokenizer/Scanner 解析器**
  - 修改 `internal/adapter/xray/log_reader.go`：
    - 移除全局 `accessLogRe` 与 `errorLogRe`；
    - 实现高效的时间戳检测与游标解析函数（如 `parseAccessLogLineScanner`、`parseErrorLogLineScanner`）；
    - 实现真正的 `[]byte` 零分配版本 `ParseLogLineFast`；
    - 实现 `ParseAccessLogLine` 与 `ParseErrorLogLine`。

- [ ] **Step 3: 单元测试回归与 Benchmark 对比**
  - 运行所有现有及新增的单元测试：`mise x -- go test -v ./internal/adapter/xray`
  - 运行基准测试对比性能：`mise x -- go test -bench=. -benchmem ./internal/adapter/xray`
  - 验证全绿无回归，性能达到预期。

- [ ] **Step 4: 全局工程质量门禁 (Quality Gate)**
  - 执行 `mise x -- go test ./...`
  - 执行 `mise x -- go vet ./...`
  - 执行 `mise x -- go build .`

## 2. 验证命令

```bash
# 1. 运行 adapter 层单测与基准测试
mise x -- go test -v ./internal/adapter/xray
mise x -- go test -bench=BenchmarkParse -benchmem ./internal/adapter/xray

# 2. 全局门禁
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
```
