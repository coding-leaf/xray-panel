# Sub-1: 后端 Xray Schema 全量无损对齐与 Outbound Mux 编解码

## Goal

补齐 `xray-panel` 后端在 `domain`、`internal/adapter/xray/schema.go`、`compiler.go`、`config_service.go` 中缺失的 Outbound `mux` / `sendThrough` / `targetStrategy`、`sockopt` (`dialerProxy`, `acceptProxyProtocol`, KeepAlive 等)、`streamSettings` (`tcpSettings`/`rawSettings`/`splithttpSettings`)、`realitySettings` 扩展字段、`sniffing` 排除字段与 `RoutingRule` 源地址匹配字段，确保重编译管道 100% 无损保留并通过 `./temp/xray.exe` 真机校验。

## Requirements

- **R1.1 (`internal/domain/outbound.go`)**:
  - `Outbound` 结构体新增：
    - `SendThrough string `json:"sendThrough,omitempty"``
    - `TargetStrategy string `json:"targetStrategy,omitempty"``
    - `Mux string `json:"mux,omitempty"``
  - `RoutingRule` 结构体新增：
    - `Source []string `json:"source,omitempty"``
    - `SourcePort string `json:"sourcePort,omitempty"``
    - `BalancerTag string `json:"balancerTag,omitempty"``
    - `DomainMatcher string `json:"domainMatcher,omitempty"``
- **R1.2 (`internal/adapter/xray/schema.go`)**:
  - `XrayOutbound` 新增 `SendThrough string `json:"sendThrough,omitempty"``、`TargetStrategy string `json:"targetStrategy,omitempty"``、`Mux json.RawMessage `json:"mux,omitempty"``，并定义 `XrayMuxConfig` 结构体（`Enabled bool`, `Concurrency int16`, `XudpConcurrency int16`, `XudpProxyUDP443 string`）。
  - `XraySockopt` 新增 `DialerProxy string `json:"dialerProxy,omitempty"``、`AcceptProxyProtocol bool `json:"acceptProxyProtocol,omitempty"``、`DomainStrategy string `json:"domainStrategy,omitempty"``、`TCPKeepAliveInterval int32 `json:"tcpKeepAliveInterval,omitempty"``、`TCPKeepAliveIdle int32 `json:"tcpKeepAliveIdle,omitempty"``、`TCPUserTimeout int32 `json:"tcpUserTimeout,omitempty"``、`TCPCongestion string `json:"tcpCongestion,omitempty"``、`Interface string `json:"interface,omitempty"``、`TcpMptcp bool `json:"tcpMptcp,omitempty"``、`TcpNoDelay bool `json:"tcpNoDelay,omitempty"``。
  - `XrayStreamSettings` 新增 `TCPSettings json.RawMessage `json:"tcpSettings,omitempty"``、`RAWSettings json.RawMessage `json:"rawSettings,omitempty"``、`SplitHTTPSettings json.RawMessage `json:"splithttpSettings,omitempty"``。
  - `XrayRealitySettings` 新增 `Target string `json:"target,omitempty"``、`Password string `json:"password,omitempty"``、`Mldsa65Seed string `json:"mldsa65Seed,omitempty"``、`Mldsa65Verify string `json:"mldsa65Verify,omitempty"``、`LimitFallbackUpload json.RawMessage `json:"limitFallbackUpload,omitempty"``、`LimitFallbackDownload json.RawMessage `json:"limitFallbackDownload,omitempty"``。
  - `XraySniffingConfig` 新增 `MetadataOnly bool `json:"metadataOnly,omitempty"``、`DomainsExcluded []string `json:"domainsExcluded,omitempty"``、`IPsExcluded []string `json:"ipsExcluded,omitempty"``。
  - `XrayRoutingRule` 新增 `Source []string `json:"source,omitempty"``、`SourcePort string `json:"sourcePort,omitempty"``、`BalancerTag string `json:"balancerTag,omitempty"``、`DomainMatcher string `json:"domainMatcher,omitempty"``。
- **R1.3 (`internal/adapter/xray/compiler.go`)**:
  - `compileOutbound`: 将 `ob.SendThrough`、`ob.TargetStrategy` 写入 `XrayOutbound`；若 `strings.TrimSpace(ob.Mux) != ""` 且为合法 JSON（非 `"null"` / `"{}"`），将其作为 `json.RawMessage(ob.Mux)` 赋给 `XrayOutbound.Mux`。
  - `compileRouting`: 将 `r.Source`、`r.SourcePort`、`r.BalancerTag`、`r.DomainMatcher` 映射到 `XrayRoutingRule`，并扩展 `hasMatchCondition` 判断（若设置了 `Source` 或 `SourcePort` 也视为合法匹配条件，且若设置了 `BalancerTag` 允许无 `OutboundTag`）。
- **R1.4 (`internal/service/config_service.go`)**:
  - `ListOutbounds`: 扩展内联结构体读取 `SendThrough`、`TargetStrategy`、`Mux json.RawMessage`，若 `len(r.Mux) > 0 && string(r.Mux) != "null"` 则存入 `domain.Outbound.Mux`。
  - `ListRoutingRules` (如有从 JSON 解析路由规则的逻辑): 确保 `Source`、`SourcePort`、`BalancerTag`、`DomainMatcher` 完整读取。
- **R1.5 (Unit & Binary Verification Tests)**:
  - 在 `internal/adapter/xray/compiler_test.go` 与 `internal/service/` 测试中，验证带有 `mux`、`sendThrough`、`targetStrategy`、`sockopt.dialerProxy`、`sniffing.domainsExcluded`、`routing.source` 的配置经过读取与重编译后 100% 完好无损，并通过 `validateWithXrayCore` (`./temp/xray.exe`) 验证。

## Acceptance Criteria

- [ ] `mise x -- go test -v ./internal/adapter/xray/... ./internal/service/...` 全绿。
- [ ] `mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .` 全绿。
