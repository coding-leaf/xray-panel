# Technical Design (Sub-1: Backend Schema & Mux Compiler)

## Data Flow & Contracts

1. **`ListOutbounds` (`internal/service/config_service.go:421-450`)**:
   ```go
   var root struct {
       Outbounds []struct {
           Tag            string          `json:"tag"`
           Protocol       string          `json:"protocol"`
           SendThrough    string          `json:"sendThrough"`
           TargetStrategy string          `json:"targetStrategy"`
           Settings       json.RawMessage `json:"settings"`
           StreamSettings json.RawMessage `json:"streamSettings"`
           Mux            json.RawMessage `json:"mux"`
       } `json:"outbounds"`
   }
   ```
   当 `len(r.Mux) > 0 && string(r.Mux) != "null"` 时，设置 `ob.Mux = string(r.Mux)`。

2. **`compileOutbound` (`internal/adapter/xray/compiler.go:392-429`)**:
   - `xrayOb.SendThrough = strings.TrimSpace(ob.SendThrough)`
   - `xrayOb.TargetStrategy = strings.TrimSpace(ob.TargetStrategy)`
   - 当 `muxStr := strings.TrimSpace(ob.Mux)` 非空且不等于 `"null"` 和 `"{}"` 时：
     - 使用 `json.Valid([]byte(muxStr))` 校验合法性（若非法则返回明确编译错误，杜绝静默吞错）。
     - 赋值给 `xrayOb.Mux = json.RawMessage(muxStr)`。

3. **`compileRouting` (`internal/adapter/xray/compiler.go:431-510`)**:
   - 支持 `Source` (`cleanStringSlice(r.Source)`)、`SourcePort` (`strings.TrimSpace(r.SourcePort)`)、`BalancerTag` (`strings.TrimSpace(r.BalancerTag)`)、`DomainMatcher` (`strings.TrimSpace(r.DomainMatcher)`)。
   - `hasMatchCondition` 增加 `len(rule.Source) > 0 || rule.SourcePort != ""`。
   - 目标出站检查：`strings.TrimSpace(r.OutboundTag) == "" && strings.TrimSpace(r.BalancerTag) == ""` 时才跳过。
