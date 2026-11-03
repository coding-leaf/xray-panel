# Implementation Plan (Sub-1)

## Checklist

1. [x] 修改 `internal/domain/outbound.go`：为 `Outbound` 增加 `SendThrough`, `TargetStrategy`, `Mux`；为 `RoutingRule` 增加 `Source`, `SourcePort`, `BalancerTag`, `DomainMatcher`。
2. [x] 修改 `internal/adapter/xray/schema.go`：扩展 `XrayOutbound`, `XrayMuxConfig`, `XraySockopt`, `XrayStreamSettings`, `XrayRealitySettings`, `XraySniffingConfig`, `XrayRoutingRule`。
3. [x] 修改 `internal/adapter/xray/compiler.go`：在 `compileOutbound` 与 `compileRouting` 中完成新字段编译与校验。
4. [x] 修改 `internal/service/config_service.go`：在 `ListOutbounds` 及路由读取逻辑中完整保留 `Mux`, `SendThrough`, `TargetStrategy` 及路由扩展字段。
5. [x] 在 `internal/adapter/xray/compiler_test.go` 编写往返无损测试及 `./temp/xray.exe` 真机集成测试。

## Validation Commands

```bash
mise x -- go test -v ./internal/adapter/xray/...
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .
```
