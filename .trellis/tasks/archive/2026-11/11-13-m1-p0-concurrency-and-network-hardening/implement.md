# M1 执行计划：P0 并发竞态清零与网络限流/探测防线

## 1. Swarm 阵型与无冲突文件边界

- **阵型**：`2 Workers (workspace:"share") + 1 Verifier (workspace:"inherit")`，全员固定 `model: "gemini-4-hydrogen-high"`（复用现有 `monitor`）。
- **`m1-xray` 独占切片**：
  - `internal/adapter/xray/grpc_client.go`、`internal/adapter/xray/grpc_client_test.go`
  - `internal/adapter/xray/supervisor.go`、`internal/adapter/xray/supervisor_test.go`
  - `internal/service/config_service.go`、`internal/service/config_service_test.go`
  - `internal/protocol/node_converter.go`、`internal/protocol/node_converter_test.go`
  - `internal/service/sub_service.go`、`internal/service/sub_service_test.go`
- **`m1-net` 独占切片**：
  - `internal/delivery/http/middleware/limiter.go`、`internal/delivery/http/middleware/limiter_test.go`
  - `internal/adapter/reality/prober.go`、`internal/adapter/reality/prober_test.go`
  - `deploy/nginx-sample.conf`
- **`m1-verifier` 审查职责**：
  - 审查两个 worker 分支提交，核验零跨切片越界、零 `.trellis/**` 修改、零吞错、零敏感数据，执行竞态测试与构建门禁。

## 2. 执行步骤清单

1. [ ] `m1-xray` 按 Fail-repro First 补充并发测试并完成 R1-R4 修复，在自身 `share` 分支跑通 `CGO_ENABLED=1 mise x -- go test -race ./internal/adapter/xray/... ./internal/service/... ./internal/protocol/...` 后提交分支并 `@m1-verifier`。
2. [ ] `m1-net` 按 Fail-repro First 补充伪造头拦截/分片上限与 TLS `h2` 探测单测并完成 R5-R7 修复，跑通 `CGO_ENABLED=1 mise x -- go test -race ./internal/delivery/http/middleware/... ./internal/adapter/reality/...` 后提交分支并 `@m1-verifier`。
3. [ ] `m1-verifier` 交叉审查两个分支 diff，核验规范与测试通过后 `@coordinator`。
4. [ ] Coordinator 执行 `git merge --squash` 归拢两个分支至主工作区（暂不 `git commit`），随后 `kill` 临时 worker，推进下一里程碑。

## 3. 验证命令

```bash
mise x -- go test ./...
CGO_ENABLED=1 mise x -- go test -race ./...
mise x -- go vet ./...
mise x -- go build .
```
