# Implementation Plan: 修复 Clash 订阅全节点 Timeout 适配失败

## Ordered Checklist
- [ ] **Step 1 (Fail-Repro First Tests)**:
  - `internal/protocol/protocol_test.go`：新增 REALITY `ToClash` 校验 `reality-opts["support-x25519mlkem768"] == true` 测试并确认修复前红灯。
  - `internal/sub/clash_test.go`：新增测试覆盖 `support-x25519mlkem768: true`、`AUTO` 排除虚拟信息节点（`PROXIES` 保留）及无重复 `port` 字段，确认修复前红灯。
  - `internal/delivery/http/handler_sub_test.go`：新增 Clash UA 输出含 `support-x25519mlkem768: true` 断言。
- [ ] **Step 2 (Core Fix — `internal/protocol/helpers.go`)**:
  - 在 `AttachClashTLS` 的 `reality` 分支注入 `realityOpts["support-x25519mlkem768"] = true`。
- [ ] **Step 3 (Template Hygiene — `internal/sub/clash.go`)**:
  - 新增 `isVirtualInfoNode`，将 `AUTO` (`url-test`) 组限定为真实代理节点（无真实节点时安全降级 `DIRECT`），`PROXIES` (`select`) 保留虚拟信息节点展示。
  - 移除 `ClashConfig` 中重复的 `Port: mixedPort` 赋值，仅保留 `MixedPort: mixedPort`（检查现有 `clash_test.go` 若有旧断言一并对齐）。
- [ ] **Step 4 (Quality Gates)**:
  - `mise x -- go test -count=1 ./...`
  - `CGO_ENABLED=1 mise x -- go test -race -count=1 ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build -o NUL .`
  - `mise x -- go build -o NUL ./cmd/agent`

## Risky Files & Rollback Points
- `internal/sub/clash.go`：需确保仅包含虚拟信息节点（无任何可用真实代理节点）时 `AUTO` 不会输出空 `proxies: []` 导致 Mihomo 配置校验失败（应回落为 `[]string{"DIRECT"}` 或在 `realProxyNames` 为空时省略 `AUTO`）。
