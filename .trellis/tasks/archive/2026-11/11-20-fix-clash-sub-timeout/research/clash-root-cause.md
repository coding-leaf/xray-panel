# Research: v2.7.0 Clash (Mihomo / Clash Verge) 订阅全 VLESS 节点 Timeout 根因调查

- **Query**: 排查 `v2.7.0` 发布后 Clash（Mihomo / Clash Verge）导入订阅全量 VLESS 节点显示 `Vless | UDP | XUDP` 且测速全部 `Timeout`，而 `v2rayN` / `sing-box` 正常连通的根本原因。
- **Scope**:
  - `git diff v2.6.10..HEAD`：`internal/sub/`、`internal/service/sub_service.go`、`internal/protocol/`、`internal/delivery/http/handler_sub.go`、`internal/adapter/xray/compiler.go`
  - `vps-jp` 线上只读对照：SQLite `settings` / `nodes` / `inbounds`、`xray` 版本与 `/usr/local/etc/xray/config.json`、本地 `http://127.0.0.1:9000/sub/<REDACTED_TOKEN>?format={clash,sing-box,raw}` 导出对比（全部敏感信息已脱敏）
  - 上游源码交叉验证：`MetaCubeX/mihomo`（`adapter/outbound/vless.go`、`adapter/outbound/reality.go`、`component/tls/reality.go`、`common/convert/v.go`）与 `XTLS/REALITY`（`tls.go`）及 `XTLS/Xray-core`（`go.mod`）
- **Date**: 2026-11-20

---

## 1. 根因结论 (Executive Summary)

### P0 根本原因：Xray-core `26.11.9` REALITY 强制校验 `X25519MLKEM768` 与 Mihomo 默认剥离该扩展不兼容

1. **直接触发点（协议握手层）**：
   - 线上主控（`vps-jp`）与 3 台远端直连节点（`US-1`、`US-2`、`TR-1`）**全部运行 `Xray 26.11.9`**。自 Xray-core `v26.9.8+` 引入上游 `github.com/xtls/reality` 提交 `8cdf7bf` 起，REALITY 服务端在 `tls.go` `Server()` 中**强制要求客户端 `ClientHello` 的 `key_share` 必须包含且仅包含一次 `X25519MLKEM768`（位于可选 `X25519` 之前）**；缺失该扩展时，服务端直接判定非 REALITY 流量并将连接透明转发至 fallback `dest` 站点。
   - 在 `MetaCubeX/mihomo` 内核中，`reality-opts.support-x25519mlkem768` 默认为 `false`（`adapter/outbound/reality.go:17`）。当该选项未显式设为 `true` 时，Mihomo 在 `component/tls/reality.go:55-60` 会调用 `BuildRemovedX25519MLKEM768HandshakeState(uConn)` **主动从 `ClientHello` 剥离 `X25519MLKEM768`**。即使节点配置了 `client-fingerprint: chrome`，发出的 `ClientHello` 仍被 Xray `26.11.9` 拒绝并回落到目标站，客户端验证 REALITY 证书签名失败并触发 `realityClientFallback` / 超时，在 Clash Verge 中全部节点统一表现为 **`Timeout`**。
   - 面板导出层 `AttachClashTLS`（`internal/protocol/helpers.go:71-85`）生成 `reality-opts` 时**未写入 `support-x25519mlkem768: true`**，导致所有 Clash/Mihomo 订阅内的 VLESS+REALITY 节点均触发上述静默回落与超时；而 `v2rayN` 与 `sing-box` 原生发送 `X25519MLKEM768`，因此完全正常。

2. **为何在 `v2.7.0` 升级后集中暴露**：
   - `v2.6.10..HEAD` 期间 `internal/protocol/` **无任何改动**，`internal/sub/clash.go` 仅增加了 `xhttp`/`splithttp` 跳过、`ToClash` 单节点错误跳过、`AUTO` 组分离与空代理 `DIRECT` 兜底，未修改 VLESS 节点字段。
   - 但在 `v2.7.0`（commit `e76747f`）中，`internal/delivery/http/handler_sub.go:38-40, 87-97` 新增了 `detectFormatFromUA`：当 Clash / Mihomo / Stash 请求未带 `?format=` 的默认聚合订阅 URL（`/sub/<token>`）时，服务端从原本返回 Base64 节点链接切换为直接返回 Clash YAML，从而让所有使用聚合订阅的 Clash Verge 客户端直接命中 `AttachClashTLS` 缺少 `support-x25519mlkem768: true` 的缺陷。

3. **关于 `Vless | UDP | XUDP` 展示的定性**：
   - `VlessFormatter.ToClash`（`internal/protocol/vless.go:163`）设置 `udp: true`，且未输出 `packet-encoding` 字段。在 Mihomo `NewVless`（`adapter/outbound/vless.go:476-484`）中，`packet-encoding` 缺省时默认启用 `option.XUDP = true`。因此 Clash Verge 节点卡片显示 `Vless | UDP | XUDP` 属于**正常协议能力标签**，并非故障原因。

---

## 2. `v2.6.10..HEAD` 代码变更审计

| 文件 | 变更摘要（`v2.6.10..HEAD`） | 是否影响 VLESS+REALITY 节点字段 |
|---|---|---|
| `internal/protocol/*` | 无任何 diff | 否（沿用 `v2.6.10` 既有逻辑） |
| `internal/delivery/http/handler_sub.go` | L34-L40, L87-L97：新增 `detectFormatFromUA`，对含 `clash` / `mihomo` / `stash` 的 UA 默认返回 `"clash"`；含 `sing-box` / `singbox` 返回 `"sing-box"` | **触发面变化**：默认 `/sub/<token>` 对 Clash Verge 从 Base64 切换为 Clash YAML |
| `internal/service/sub_service.go` | L34-L171：新增 `WithNodeRepo`、`resolveActiveNodeMap`、`prepareInboundForNode`，将 `enabled && desired_state == active` 的远端直连节点纳入订阅并加 `[Region]` 前缀 | 否（仅聚合远端节点并填充 `ExternalHost`/`ExternalPort`） |
| `internal/sub/clash.go` | L35-L44, L60-L87, L130-L132：跳过 `xhttp`/`splithttp`；`ToClash` 错误改为 warn+skip；`PROXIES` 首项置 `"AUTO"`，`AUTO` 仅含节点列表；无节点时附加 `clashEmptyNotice` + `DIRECT` 兜底 | 否（未改写任何 VLESS 字段） |

---

## 3. 线上 `vps-jp` 只读对照验证（已脱敏）

### 3.1 服务端运行态与配置事实
- **Xray 版本**：主控 `vps-jp` 与远端 `US-1`（洛杉矶）、`US-2`（休斯顿）、`TR-1`（土耳其）均为 **`Xray 26.11.9`**。
- **面板关键设置**：`sub_default_fingerprint: 'chrome'`、`public_port: '443'`、`sub_show_info_nodes: 'true'`、`sub_clash_mode: 'rule'`、`sub_clash_mixed_port: '7890'`。
- **Xray `config.json` 校验项**：`ipv4入口`、`ipv6入口` 与 3 个远端入站均未配置 `minClientVer` / `maxClientVer` / `maxTimeDiff` 限制；`ipv4入口` 的 `shortIds` 为 `["", "<16-hex>"]`，其余 4 个入站均为 `["<16-hex>"]`（非纯数字）。

### 3.2 同一节点三格式逐字段比对

#### 样本 A：主控节点 `日本直连`（来自 `ipv4入口`，`node_id=0`）
| 字段 / 维度 | Clash YAML (`?format=clash`) | Sing-Box JSON (`?format=sing-box`) | v2rayN URI (`?format=raw`) | 一致性判定 |
|---|---|---|---|---|
| `type` / 协议 | `vless` | `vless` | `vless://` | 一致 |
| `server` | `<HOST_1>` (IPv4) | `<HOST_1>` | `<HOST_1>` | 一致 |
| `port` | `443` | `443` | `443` | 一致 |
| `uuid` | `<UUID_3>` (含 routeID) | `<UUID_3>` | `<UUID_3>` | 一致 |
| `flow` | `xtls-rprx-vision` | `xtls-rprx-vision` | `flow=xtls-rprx-vision` | 一致 |
| `network` / `type` | `tcp` | (默认 tcp) | `type=tcp` | 一致 |
| `tls` / `security` | `tls: true` | `tls.enabled: true` | `security=reality` | 一致 |
| `sni` / `servername` | `<SNI_1>` | `<SNI_1>` | `sni=<SNI_1>` | 一致 |
| `fp` / `client-fingerprint` | `chrome` | `utls: {enabled: true, fingerprint: "chrome"}` | `fp=chrome` | 一致 |
| `public-key` | `<PBK_1>` (43位 RawURL Base64) | `<PBK_1>` | `pbk=<PBK_1>` | 一致 |
| `short-id` | *(省略，因 `shortIds[0]==""`)* | *(省略)* | *(省略)* | 一致（服务端允许 `""`） |
| **`support-x25519mlkem768`** | **缺失（Mihomo 默认 `false` 并主动剥离 ML-KEM）** | Sing-Box 原生发送 | Xray-core 原生发送 | **关键差异（根因）** |

#### 样本 B：远端直连节点 `[洛杉矶] 直连`（来自 `us-direct`，`node_id=1`）
| 字段 / 维度 | Clash YAML (`?format=clash`) | Sing-Box JSON (`?format=sing-box`) | v2rayN URI (`?format=raw`) | 一致性判定 |
|---|---|---|---|---|
| `server` / `port` | `<NODE_HOST_1>` (IPv4) : `443` | `<NODE_HOST_1>` : `443` | `<NODE_HOST_1>` : `443` | 一致 |
| `uuid` | `<UUID_1>` | `<UUID_1>` | `<UUID_1>` | 一致 |
| `flow` / `network` | `xtls-rprx-vision` / `tcp` | `xtls-rprx-vision` / (tcp) | `flow=xtls-rprx-vision` / `type=tcp` | 一致 |
| `servername` / `client-fingerprint` | `<SNI_3>` / `chrome` | `<SNI_3>` / `chrome` | `sni=<SNI_3>` / `fp=chrome` | 一致 |
| `reality-opts` | `public-key: <PBK_3>`, `short-id: <SID_2>` (16-hex) | `public_key: <PBK_3>`, `short_id: <SID_2>` | `pbk=<PBK_3>`, `sid=<SID_2>` | 一致 |
| **`support-x25519mlkem768`** | **缺失（Mihomo 默认 `false`）** | Sing-Box 原生发送 | Xray-core 原生发送 | **关键差异（根因）** |

---

## 4. 精确代码锚点与次要隐患清单

### 4.1 主根因锚点
- `internal/protocol/helpers.go:71-85` (`AttachClashTLS`)：构造 `realityOpts` 时仅注入了 `public-key`、`short-id`、`spider-x`，**缺失 `realityOpts["support-x25519mlkem768"] = true`**。
- `internal/delivery/http/handler_sub.go:38-40, 87-97` (`GetSubscription` / `detectFormatFromUA`)：UA 命中 Clash/Mihomo 时下发上述缺失 ML-KEM 开关的 Clash YAML。
- `internal/protocol/vless.go:149-178` (`VlessFormatter.ToClash`)：透传调用 `AttachClashTLS`，同时 `network: tcp`、`flow: xtls-rprx-vision`、`udp: true` 经核对均符合 Mihomo `VlessOption` 语法（`adapter/outbound/vless.go:60-93, 260-264, 464-488`）。

### 4.2 顺带发现的次要配置卫生问题（非本次全线 Timeout 主因，建议一并或后续治理）
1. **监听端口重复声明**（`internal/sub/clash.go:98-101`）：`ClashConfig` 同时设置了 `Port: mixedPort`（7890）和 `MixedPort: mixedPort`（7890）。Clash Verge 会接管覆盖端口因此不报错，但原生 Mihomo CLI 直接加载该 YAML 会因 `7890` 端口冲突无法启动 HTTP 入站。
2. **`AUTO` 测速组混入虚拟信息节点**（`internal/sub/clash.go:77-87` 与 `internal/service/sub_service.go:280-283`）：`buildVirtualInfoNodes` 插入的 3 条 `127.0.0.1:10000` 虚拟 Trojan 信息节点被同时加入 `AUTO`（`url-test`）组，每次测速会产生 3 个必然失败的探测；应仅保留在 `PROXIES`（`select`）或过滤出 `AUTO`。
3. **`shortIds` 首元素为空串时的 `sid` 省略**（`internal/protocol/node_converter.go:237-243`）：当前取 `r.ShortIds[0]`，若管理员配置 `["", "<hex>"]` 会省略 `short-id`；虽在当前服务端允许 `""` 时可用，但优先选取首个非空 `shortId` 抗退化能力更强（或保持与服务端策略一致）。

---

## 5. 最小安全修复方案

1. **核心修复（最小改动、零破坏性）**：
   - 修改 `internal/protocol/helpers.go` 的 `AttachClashTLS`（L71-L85）：在 `security == "reality"` 且 `len(realityOpts) > 0`（或存在 `pbk`）时，注入：
     ```go
     realityOpts["support-x25519mlkem768"] = true
     ```
   - 影响评估：
     - **新版 Mihomo（Clash Verge Rev 等）**：开启 `X25519MLKEM768` 混合后量子密钥交换，立即满足 Xray `26.9.8+` / `26.11.9` 的 REALITY 握手校验；
     - **旧版 Mihomo**：YAML 反序列化会忽略未识别字段，无回归风险；
     - **其他客户端（`v2rayN`、`sing-box`）**：完全不受 Clash 导出路径影响。
2. **可选次要改进（建议纳入同一 PR 或独立小补丁）**：
   - `internal/sub/clash.go:98-101`：仅输出 `MixedPort`（省略重复的 `Port`/`SocksPort`，或保持 `omitempty` 不填 `Port`/`SocksPort`），避免原生 Mihomo 内核端口冲突。
   - `internal/sub/clash.go:77-87`：`AUTO`（`url-test`）排除虚拟信息节点（`Address == "127.0.0.1" && Port == 10000 && UUID == "00000000-0000-0000-0000-000000000000"`），只让真实代理参与自动测速。
   - 补充单测（Fail-Repro First）：
     - `internal/protocol/protocol_test.go`：验证 VLESS+REALITY 调用 `ToClash` 后 `reality-opts["support-x25519mlkem768"] == true`；
     - `internal/sub/clash_test.go` & `internal/delivery/http/handler_sub_test.go`：验证 Clash UA 请求返回 YAML 中包含 `support-x25519mlkem768: true`，以及若采纳次要改进后的 `AUTO` / `mixed-port` 行为。

---

## 6. Files Found

| Path | Role |
|---|---|
| `internal/protocol/helpers.go` | Clash / Sing-Box TLS 与 REALITY 参数注入（`AttachClashTLS` 缺失 `support-x25519mlkem768`） |
| `internal/protocol/vless.go` | VLESS 节点 URI / Clash / Sing-Box 格式化器 |
| `internal/protocol/node_converter.go` | `domain.Inbound` → `protocol.NodeConfig` 转换与 `shortId`/`pbk`/`fp` 解析 |
| `internal/sub/clash.go` | Clash YAML 配置组装（`ClashConfig`、`proxy-groups`、`rules`） |
| `internal/service/sub_service.go` | 订阅节点汇聚、远端节点 `prepareInboundForNode` 与虚拟信息节点注入 |
| `internal/delivery/http/handler_sub.go` | 订阅 HTTP 入口与 `detectFormatFromUA` 客户端格式识别 |
| `internal/sub/clash_test.go` | Clash 导出单元测试 |
| `internal/delivery/http/handler_sub_test.go` | 订阅 HTTP UA 探测集成测试 |
| `internal/protocol/protocol_test.go` | 协议格式化器单元测试 |

---

## 7. Relevant `.trellis/spec/` Files (Candidate List)

- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

---

## 8. Caveats / Open Questions

1. **Mihomo `client-fingerprint` 依赖限制**：在 Mihomo 上游 `metacubex/utls v1.8.7` 中，仅 `chrome` 指纹模板原生携带 `X25519MLKEM768`；`firefox`/`safari`/`ios` 等旧指纹模板不包含 ML-KEM（见 `MetaCubeX/mihomo#3193`）。当前线上面板默认指纹为 `chrome`，加上 `support-x25519mlkem768: true` 后可立即恢复；若管理员后续手动将入站或全局默认指纹改为非 `chrome` 指纹且客户端 Mihomo 内核未同步升级 `utls`，仍可能触发 REALITY 回落。
2. **Base64 `vless://` 回退导入场景**：若极少数用户仍用 Clash Verge 手动导入纯 Base64 `vless://` 链接，Mihomo `common/convert/v.go:53-57` 也仅识别 URL query 中的 `support-x25519mlkem768=true`。目前因 `v2.7.0` 已启用 `detectFormatFromUA` 默认直出 Clash YAML，优先修复 `AttachClashTLS` 即可覆盖所有订阅直导场景；不建议随意向标准 `vless://` URI 注入非标准参数以免干扰其他客户端（如确有需要可再评估）。
3. **尚未在本地启动真实 Mihomo 进程做端到端拨号测试**：本地环境未安装 `mihomo` 二进制，结论基于线上 `vps-jp` 实测数据、Xray `26.11.9` 实机版本与 `MetaCubeX/mihomo` / `XTLS/REALITY` 上游源码逐行核验，修复发布后建议直接在 Clash Verge 中刷新订阅测速验证。
