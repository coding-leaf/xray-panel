# 修复 Clash 订阅全节点 Timeout 适配失败

## Goal
根治 `v2.7.0` 发布后 Clash（Mihomo / Clash Verge）导入订阅时所有 VLESS+REALITY 节点全部 `Timeout` 的 P0 缺陷，同时清理 Clash 模板中 `AUTO` 测速组混入虚拟信息节点与 `port`/`mixed-port` 端口重复声明两项配置卫生问题，验证通过后重新部署至 `vps-jp`（Master）并发布补丁版本。

## Root Cause (Verified)
详证见 [clash-root-cause.md](research/clash-root-cause.md)：
1. **P0 直接根因（协议握手层）**：
   - 线上主控与 3 台远端节点均运行 **`Xray 26.11.9`**（上游 `xtls/reality` 强制要求 REALITY `ClientHello` 的 `key_share` 携带且前置 `X25519MLKEM768`，缺失则静默回落至 fallback 目标站）。
   - `MetaCubeX/mihomo` 默认 `reality-opts.support-x25519mlkem768: false`，会主动从 uTLS `ClientHello` 剥离 `X25519MLKEM768`（`component/tls/reality.go:55-60`）。
   - `AttachClashTLS`（`internal/protocol/helpers.go:71-85`）未输出 `support-x25519mlkem768: true`，导致 Mihomo 全部 REALITY 节点静默回落并超时；而 `v2rayN` / `sing-box` 原生携带 ML-KEM 因此正常。
2. **为何 `v2.7.0` 暴露**：`v2.7.0`（`e76747f`）新增 `detectFormatFromUA`（`internal/delivery/http/handler_sub.go:38-40, 87-97`），使 Clash Verge 请求默认 `/sub/<token>` 从 Base64 切换为 Clash YAML，立即命中上述缺失字段。
3. **次要配置卫生问题**：
   - `internal/sub/clash.go:77-87`：`AUTO`（`url-test`）组混入了 3 条 `127.0.0.1:10000` 虚拟信息 Trojan 节点（截图中 `AUTO` 甚至选中了 `📢 公告` 节点）；
   - `internal/sub/clash.go:98-101`：`ClashConfig` 同时输出 `port: 7890` 与 `mixed-port: 7890`，导致原生 Mihomo CLI 端口冲突。

## Requirements
- **R1 (P0 核心修复)**：修改 `internal/protocol/helpers.go:71-85` (`AttachClashTLS`)，在 `security == "reality"` 且 `len(realityOpts) > 0` 时写入 `realityOpts["support-x25519mlkem768"] = true`。
- **R2 (`AUTO` 测速组排除虚拟信息节点)**：修改 `internal/sub/clash.go`，`PROXIES`（`select`）继续保留虚拟信息节点供用户查看公告/剩余流量/到期时间，但 `AUTO`（`url-test`）组仅纳入真实代理节点（排除 `Address == "127.0.0.1" && Port == 10000 && UUID == "00000000-0000-0000-0000-000000000000"`）；若过滤后无真实节点则按既有 `DIRECT` 兜底逻辑安全降级。
- **R3 (消除 `port` / `mixed-port` 重复冲突)**：修改 `internal/sub/clash.go:98-101`，仅输出 `MixedPort`（留空 `Port` 与 `SocksPort` 由 `omitempty` 省略），避免原生 Mihomo CLI 端口冲突。
- **R4 (Fail-Repro First 回归测试)**：
  - `internal/protocol/protocol_test.go`：验证 VLESS+REALITY（及非 REALITY TLS 不注入该字段）调用 `ToClash` 后 `reality-opts["support-x25519mlkem768"] == true`；
  - `internal/sub/clash_test.go`：验证生成的 Clash YAML 包含 `support-x25519mlkem768: true`、`AUTO` 组不含虚拟信息节点而 `PROXIES` 保留虚拟信息节点、仅存在 `mixed-port` 无重复 `port`；
  - `internal/delivery/http/handler_sub_test.go`：验证 Clash Verge UA 请求 `/sub/:token` 输出的 YAML 含 `support-x25519mlkem768: true`。

## Out of Scope
- 不修改标准 `vless://` Base64 URI（不注入非标准 query 参数，避免影响 v2rayN/Shadowrocket 等客户端）；
- 不修改远端 Agent 二进制或数据库 Schema（仅需更新 Master `panel` 二进制）。

## Acceptance Criteria
1. **AC-1**：所有经 `AttachClashTLS` 导出的 REALITY 节点（VLESS/Trojan 等）均包含 `reality-opts.support-x25519mlkem768: true`，非 REALITY 节点不包含该字段。
2. **AC-2**：Clash `AUTO`（`url-test`）代理组不再包含 `127.0.0.1:10000` 虚拟信息节点，而 `PROXIES`（`select`）仍保留用于展示剩余流量/到期时间/公告。
3. **AC-3**：Clash YAML 仅输出 `mixed-port: <port>`，不再同时输出重复的 `port: <port>`。
4. **AC-4**：全量后端门禁通过（`mise x -- go test -count=1 ./...`、`CGO_ENABLED=1 mise x -- go test -race -count=1 ./...`、`mise x -- go vet ./...`、`mise x -- go build -o NUL .`）。
5. **AC-5**：重新交叉编译并部署 `panel` 至 `vps-jp` 后，验证 Clash 订阅实际输出含 `support-x25519mlkem768: true` 且用户在 Clash Verge 刷新订阅可正常连通。
