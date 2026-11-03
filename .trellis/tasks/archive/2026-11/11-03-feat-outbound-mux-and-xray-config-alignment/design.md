# Technical Design: Outbound Mux & Xray Config Alignment

## 1. Architecture & Layering Boundaries

- **`internal/domain`**:
  - `Outbound` (`internal/domain/outbound.go`): 新增 `SendThrough string `json:"sendThrough,omitempty"``、`TargetStrategy string `json:"targetStrategy,omitempty"``、`Mux string `json:"mux,omitempty"``（保持与 `SettingsJSON` / `StreamSettings` 一致的 JSON 字符串承载风格，同时兼容结构化序列化）。
  - `RoutingRule` (`internal/domain/outbound.go`): 新增 `Source []string `json:"source,omitempty"``、`SourcePort string `json:"sourcePort,omitempty"``、`BalancerTag string `json:"balancerTag,omitempty"``、`DomainMatcher string `json:"domainMatcher,omitempty"``。
- **`internal/adapter/xray/schema.go`**:
  - `XrayOutbound`: 新增 `SendThrough string `json:"sendThrough,omitempty"``、`TargetStrategy string `json:"targetStrategy,omitempty"``、`Mux json.RawMessage `json:"mux,omitempty"``（同时提供 `XrayMuxConfig` 辅助结构体用于校验与互斥判断）。
  - `XraySockopt`: 补齐 `DialerProxy`、`AcceptProxyProtocol`、`DomainStrategy`、`TCPKeepAliveInterval`、`TCPKeepAliveIdle`、`TCPUserTimeout`、`TCPCongestion`、`Interface`、`TcpMptcp`、`TcpNoDelay`。
  - `XrayStreamSettings`: 补齐 `TCPSettings json.RawMessage `json:"tcpSettings,omitempty"``、`RAWSettings json.RawMessage `json:"rawSettings,omitempty"``、`SplitHTTPSettings json.RawMessage `json:"splithttpSettings,omitempty"``。
  - `XrayRealitySettings`: 补齐 `Target`、`Password`、`Mldsa65Seed`、`Mldsa65Verify`、`LimitFallbackUpload`、`LimitFallbackDownload`。
  - `XraySniffingConfig`: 补齐 `MetadataOnly bool `json:"metadataOnly,omitempty"``、`DomainsExcluded []string `json:"domainsExcluded,omitempty"``、`IPsExcluded []string `json:"ipsExcluded,omitempty"``。
  - `XrayRoutingRule`: 补齐 `Source`、`SourcePort`、`BalancerTag`、`DomainMatcher`。
- **`internal/adapter/xray/compiler.go`**:
  - `compileOutbound`: 解析 `ob.Mux`（非空且为合法 JSON 对象时写入 `xrayOb.Mux`）；若检测到 VLESS 出站 `settings.vnext[*].users[*].flow == "xtls-rprx-vision"` 且 `mux.enabled == true`，为防止 Xray 运行时流拼接冲突，仅当非 vision 时启用或保留（但若 `mux` 为禁用态或空则正常处理，或者若只透传合法 JSON 则保证与 `xray.exe run -test` 兼容）。同步写入 `SendThrough` 与 `TargetStrategy`。
  - `compileRouting`: 同步映射 `Source`、`SourcePort`、`BalancerTag`、`DomainMatcher`。
- **`internal/service/config_service.go`**:
  - `ListOutbounds`: 扩展内联解析结构体，读取 `mux`、`sendThrough`、`targetStrategy` 并填充至 `domain.Outbound`。
  - `SaveOutbound`: 支持新增/更新 `Mux`、`SendThrough`、`TargetStrategy`；若更新已有同 Tag 出站且请求未显式传 `Mux`/`SendThrough`/`TargetStrategy`（前端未改动或仅局部更新），注意支持前端显式清空（如传 `""` 或 `{"enabled":false}`）与正常保存。
- **`web/src/views/`**:
  - `outbounds/`: 扩展 `OutboundFormState` 与序列化器，新增 Mux & XUDP 卡片、高级出站选项（`dialerProxy`、`sendThrough`、`targetStrategy`），列表与详情展示 `MUX` 与 `dialerProxy`。
  - `topology/`: `QuickPublishWizardModal.vue` 增加落地出站开启 Mux (默认推荐并发 8 / XUDP 16 / UDP443 reject，当非 Vision 时可勾选) 的选项。
  - `inbounds/`: 扩展 `InboundFormData`，支持 `sockopt.acceptProxyProtocol` 与 Sniffing `metadataOnly` / `domainsExcluded` / `ipsExcluded`。
  - `RoutingView.vue`: 扩展路由规则表单，支持 `source`、`sourcePort`、`domainMatcher`。
