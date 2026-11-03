# Sub-2: 前端出站管理与拓扑中心 Mux 及链式代理配置

## Goal

在前端出站管理（`web/src/views/outbounds/`）与拓扑中心快捷向导（`web/src/views/topology/components/QuickPublishWizardModal.vue`）中，完整支持 Outbound `Mux & XUDP`、链式前置代理 `sockopt.dialerProxy`、出口源 IP `sendThrough`、域名解析策略 `targetStrategy` 的可视化编辑、Vision 互斥保护与徽章展示。

## Requirements

- **R2.1 (`web/src/views/outbounds/types.ts`)**:
  - `OutboundItem` 接口新增 `sendThrough?: string`、`targetStrategy?: string`、`mux?: string`。
  - `OutboundFormState` 新增：
    - `muxEnabled: boolean`
    - `muxConcurrency: number` (默认 `8`, 支持 `-1` ~ `128`)
    - `muxXudpConcurrency: number` (默认 `16`, 支持 `-1` ~ `1024`)
    - `muxXudpProxyUDP443: 'reject' | 'allow' | 'skip'` (默认 `'reject'`)
    - `sendThrough: string`
    - `targetStrategy: string` (默认 `''` / `'AsIs'`)
    - `dialerProxy: string` (链式代理前置出站 Tag，读写于 `streamSettings.sockopt.dialerProxy`)
  - 在 `populateOutboundForm` 中反序列化 `item.mux`、`item.sendThrough`、`item.targetStrategy` 以及 `streamSettings.sockopt.dialerProxy`（同时在重新构建 `streamSettings` 时保留原 `sockopt` 中的其他高级字段，防止前端保存时冲掉 `tcpKeepAliveInterval` 等字段！）。
  - 在 `sanitizeOutboundPayload` 中生成 `mux` JSON 字符串（当启用或原先有配置时）及 `sendThrough`、`targetStrategy`。
- **R2.2 (`web/src/views/outbounds/components/OutboundFormDrawer.vue`)**:
  - 针对支持多路复用的代理协议（`vless`, `vmess`, `trojan`, `shadowsocks`, `socks`, `http`），新增 **“连接多路复用 (Mux & XUDP)”** 配置区块：
    - 开启开关 `muxEnabled`
    - 当 `protocol === 'vless' && vlessFlow === 'xtls-rprx-vision'` 时，展示醒目互斥提示（`xtls-rprx-vision` 流控与 Mux 不兼容，开启 Mux 将自动清空 Flow 或禁用 Mux 开关）。
    - `concurrency`（TCP 最大并发子连接，默认 8，提示 `-1` 表示仅关闭 TCP 复用）、`xudpConcurrency`（UDP XUDP 最大并发，默认 16，提示 `-1` 表示关闭 UDP 复用）、`xudpProxyUDP443`（QUIC UDP/443 处理策略：`reject` 拦截回退 H2 / `allow` 复用转发 / `skip` 绕过 Mux）。
  - 新增 **“高级路由与链式代理 (Sockopt & Relay)”** 配置项：
    - `dialerProxy`（前置代理出站 Tag 输入/选择，用于链式落地）
    - `sendThrough`（出口源 IP / `origin` / `srcip`）
    - `targetStrategy`（目标域名解析策略下拉：默认 `AsIs`、`UseIP`、`UseIPv4`、`UseIPv6`、`ForceIP` 等）
- **R2.3 (`OutboundTable.vue` & `OutboundDetailDrawer.vue`)**:
  - 列表与详情抽屉展示 `MUX` 状态徽章（如 `MUX (8 / XUDP 16)`）以及 `dialerProxy` 链式前置标识、`sendThrough` 绑定信息。
- **R2.4 (`web/src/views/topology/components/QuickPublishWizardModal.vue`)**:
  - 在导入落地节点链接时，若节点非 `xtls-rprx-vision`，提供可选的 **“启用跨洋连接复用 (Mux + XUDP)”** 开关（开启后自动注入 `mux: {"enabled":true,"concurrency":8,"xudpConcurrency":16,"xudpProxyUDP443":"reject"}`）。

## Acceptance Criteria

- [ ] `cd web && mise x -- npm run build` 零报错通过。
