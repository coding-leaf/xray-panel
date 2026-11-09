# Add IPv4/IPv6 Prefer Strategy Options (UseIPv4v6 / UseIPv6v4)

## Goal

在面板的出站域名解析策略选择器中补齐 Xray-core 原生支持的“优先 IPv4 / 优先 IPv6（双栈回退）”策略选项（`UseIPv4v6` / `UseIPv6v4`）及 `ForceIP*` 枚举，统一修正面板中关于 `UseIPv4`、`UseIPv6`、`UseIPv4v6`、`UseIPv6v4`、`ForceIP*` 的语义描述文案，并消除 Freedom 协议双选择器冲突、`AsIs` 回显盲区及 DNS 双栈联动提示缺失问题，确保生成的 `config.json` 100% 符合 Xray-core 规范并通过 `xray run -test` 校验。

## Background & Confirmed Facts

1. **Xray-Core (`v26.10.31`, `./temp/xray.exe`) 底层协议契约**：
   - **出站域名解析策略 (`xray.transport.internet.DomainStrategy`)**：作用于 `freedom.settings.domainStrategy`、`outbound.targetStrategy` 以及 `streamSettings.sockopt.domainStrategy`。
     - 完整合法枚举集合：
       - `AsIs`：不预先本地解析，保持原样交由系统或对端处理；
       - `UseIP`：双栈自动解析（仅当匹配到 DNS 规则或需本地解析时生效）；
       - `UseIPv4`：仅使用 IPv4（只解析/连接 A 记录，无 IPv6 回退）；
       - `UseIPv6`：仅使用 IPv6（只解析/连接 AAAA 记录，无 IPv4 回退）；
       - `UseIPv4v6`：**优先 IPv4 (PreferIPv4)**，同时支持双栈，IPv4 不可用时回退至 IPv6；
       - `UseIPv6v4`：**优先 IPv6 (PreferIPv6)**，同时支持双栈，IPv6 不可用时回退至 IPv4；
       - `ForceIP` / `ForceIPv4` / `ForceIPv6` / `ForceIPv4v6` / `ForceIPv6v4`：忽略路由/内置条件，强制经内置 DNS 解析并使用对应栈策略。
     - **二进制实测结论**：若直接写入字面量 `"PreferIPv4"` 或 `"preferIPv4"`，`xray run -test` 会以 `exit code 23: unsupported domain strategy` 拒绝启动；写入 `"UseIPv4v6"` 与 `"UseIPv6v4"` 则返回 `Configuration OK.`。
   - **DNS 全局查询策略 (`xray.app.dns.QueryStrategy`)**：作用于 `dns.queryStrategy`。
     - Xray-core Protobuf 定义的枚举仅包含 `USE_IP` (`"UseIP"`)、`USE_IP4` (`"UseIPv4"`)、`USE_IP6` (`"UseIPv6"`)、`USE_SYS` (`"UseSystem"`)。
     - DNS 查询层只决定是否发起 A / AAAA 记录查询。若出站选择了 `UseIPv4v6` / `UseIPv6v4` / `ForceIPv4v6` / `ForceIPv6v4` 等双栈回退策略，需配合 DNS 查询策略开启 `UseIP` 才能同时查询 A 与 AAAA 记录。
2. **代码库现状与痛点（含多智能体辩论识别的边界缺陷）**：
   - `web/src/views/outbounds/components/OutboundFormDrawer.vue:45-54`（Freedom 直连 `settings.domainStrategy`）：目前仅有 `UseIP`、`UseIPv4`、`UseIPv6`、`AsIs`，**缺失 `UseIPv4v6`、`UseIPv6v4` 及 `ForceIP*` 枚举**。
   - `web/src/views/outbounds/components/OutboundFormDrawer.vue:534-549`（出站高级路由 `outbound.targetStrategy`）：
     - 将 `UseIPv4` 误标为 `UseIPv4 (强制优先 IPv4)`、`UseIPv6` 误标为 `UseIPv6 (强制优先 IPv6)`（实际上这两项是仅单栈，无回退）；
     - `UseIPv4v6` 与 `UseIPv6v4` 仅有裸值，缺少中文语义标注；
     - 缺少 `ForceIPv4v6` 与 `ForceIPv6v4` 两个合法枚举项；
     - 当 `form.protocol === 'freedom'` 时，底部仍然渲染 `targetStrategy` 选择器，与顶部 `freedomDomainStrategy` 形成双重选择器冲突，且从其他协议切到 `freedom` 时未清空 `targetStrategy`。
   - `web/src/views/outbounds/types.ts:173,205-219`（回显与序列化）：
     - 若存量配置 `ob.targetStrategy === 'AsIs'`，因下拉框默认项 `value=""`，会导致回显为空白；
     - 若存量 Freedom 节点通过 `sockopt.domainStrategy` 承载解析策略，`populateOutboundForm` 未从中回退读取，且 `rawSockopt.domainStrategy` 未清理会导致覆盖。
   - `web/src/views/DNSView.vue:45-48`（DNS 全局查询策略 `dns.queryStrategy`）：将 `UseIPv6` 标为 `UseIPv6 (优先/仅查询 IPv6 AAAA 记录)`，“优先”二字容易与双栈回退混淆，实际为“仅查询 IPv6 AAAA 记录”。

## Requirements

- **REQ-1（Freedom 直连策略补齐 `UseIPv4v6` / `UseIPv6v4` / `ForceIP*` 并修正语义）**：
  - 文件锚点：`web/src/views/outbounds/components/OutboundFormDrawer.vue:45-54`
  - 在 `form.freedomDomainStrategy` 下拉框中补齐 `UseIPv4v6`、`UseIPv6v4` 及 `ForceIP*`，并将所有选项文案统一为准确语义：
    - `UseIP` → `UseIP (双栈自动解析)`
    - `UseIPv4` → `UseIPv4 (仅 IPv4 - 推荐)`
    - `UseIPv6` → `UseIPv6 (仅 IPv6)`
    - `UseIPv4v6` → `UseIPv4v6 (PreferIPv4 - 优先IPv4回退IPv6)`
    - `UseIPv6v4` → `UseIPv6v4 (PreferIPv6 - 优先IPv6回退IPv4)`
    - `ForceIP` → `ForceIP (强制内置DNS双栈解析)`
    - `ForceIPv4` → `ForceIPv4 (强制仅 IPv4)`
    - `ForceIPv6` → `ForceIPv6 (强制仅 IPv6)`
    - `ForceIPv4v6` → `ForceIPv4v6 (强制优先IPv4回退IPv6)`
    - `ForceIPv6v4` → `ForceIPv6v4 (强制优先IPv6回退IPv4)`
    - `AsIs` → `AsIs (保持原样)`
  - 当选中双栈回退策略（`UseIPv4v6` / `UseIPv6v4` / `ForceIPv4v6` / `ForceIPv6v4`）时，在提示文案中说明需配合 DNS 模块开启 `UseIP` 双栈查询。
- **REQ-2（出站高级路由 `targetStrategy` 修正语义、补齐枚举并与 Freedom 互斥）**：
  - 文件锚点：`web/src/views/outbounds/components/OutboundFormDrawer.vue:534-549, 645-665`
  - 仅在 `form.protocol !== 'freedom'` 时渲染底部高级路由中的 `targetStrategy` 选择器；在 `onOutboundProtocolChange` 切换至 `freedom`（或 `sanitizeOutboundPayload` 为 `freedom`）时清空 `targetStrategy`，消除双选择器竞争。
  - 修正 `targetStrategy` 选项文案并补齐 `ForceIPv4v6` / `ForceIPv6v4`：
    - `""` → `默认 (AsIs 保持原样)`
    - `UseIP` → `UseIP (双栈自动解析)`
    - `UseIPv4` → `UseIPv4 (仅 IPv4)`
    - `UseIPv6` → `UseIPv6 (仅 IPv6)`
    - `UseIPv4v6` → `UseIPv4v6 (PreferIPv4 - 优先IPv4回退IPv6)`
    - `UseIPv6v4` → `UseIPv6v4 (PreferIPv6 - 优先IPv6回退IPv4)`
    - `ForceIP` → `ForceIP (强制内置DNS双栈解析)`
    - `ForceIPv4` → `ForceIPv4 (强制仅 IPv4)`
    - `ForceIPv6` → `ForceIPv6 (强制仅 IPv6)`
    - `ForceIPv4v6` → `ForceIPv4v6 (强制优先IPv4回退IPv6)`
    - `ForceIPv6v4` → `ForceIPv6v4 (强制优先IPv6回退IPv4)`
- **REQ-3（`types.ts` 回显归一化与防幽灵覆盖）**：
  - 文件锚点：`web/src/views/outbounds/types.ts:173, 205-220, 515-530`
  - 在 `populateOutboundForm` 中：
    - 将 `ob.targetStrategy === 'AsIs'`（以及 `ob.protocol === 'freedom'` 时的 `targetStrategy`）归一化为 `''`，确保下拉框准确回显“默认 (AsIs 保持原样)”；
    - 对 `freedomDomainStrategy` 支持 `s.domainStrategy || str.sockopt?.domainStrategy || ob.targetStrategy || 'UseIPv4'` 回退读取；若 `protocol === 'freedom'`，从 `rawSockopt` 中剔除 `domainStrategy`，防止旧 `sockopt.domainStrategy` 幽灵覆盖表单修改。
  - 在 `sanitizeOutboundPayload` 中：当 `form.protocol === 'freedom'` 时强制 `targetStrategy: ''`。
- **REQ-4（DNS 查询策略 `queryStrategy` 文案语义修正）**：
  - 文件锚点：`web/src/views/DNSView.vue:45-48`
  - 将 `DNSView.vue` 中 `UseIPv6` 的文案从 `UseIPv6 (优先/仅查询 IPv6 AAAA 记录)` 修正为 `UseIPv6 (仅查询 IPv6 AAAA 记录)`；将 `UseIP` 文案明确为 `UseIP (双栈同时查询 A/AAAA - 双栈优先回退必选)`。
- **REQ-5（后端编译器测试覆盖）**：
  - 文件锚点：`internal/adapter/xray/compiler_test.go`
  - 新增/扩展单元测试用例，验证 Freedom `domainStrategy: "UseIPv4v6" / "UseIPv6v4"` 及 Outbound `targetStrategy: "UseIPv4v6" / "UseIPv6v4"` 编译产物准确无损，并通过 `validateWithXrayCore` 实测校验。

## Out of Scope

- 不修改数据库 Schema 或后端 REST API 字段结构。
- 不向 `dns.queryStrategy` 注入非法的 `UseIPv4v6` / `PreferIPv4` 枚举值（因其不属于 Xray `xray.app.dns.QueryStrategy` 协议规范）。
- 不在本任务中重构现有后端编译器对 Freedom `settings.domainStrategy` 的存储位置（保留现有向后兼容的 `settings.domainStrategy` 契约，避免破坏已有单元测试与配置断言）。

## Acceptance Criteria

- [ ] AC-1：`OutboundFormDrawer.vue` 的 Freedom 直连 `domainStrategy` 下拉框包含 `UseIPv4v6`、`UseIPv6v4` 及 `ForceIP*` 选项，且“仅单栈”与“Prefer 双栈优先回退”语义区分清晰。
- [ ] AC-2：`OutboundFormDrawer.vue` 的高级路由 `targetStrategy` 下拉框在 `protocol === 'freedom'` 时隐藏，修正 `UseIPv4/6` 文案，明确标注 `UseIPv4v6 (PreferIPv4 - 优先IPv4回退IPv6)` 与 `UseIPv6v4 (PreferIPv6 - 优先IPv6回退IPv4)`，并包含 `ForceIPv4v6` / `ForceIPv6v4`。
- [ ] AC-3：`DNSView.vue` 的 `queryStrategy` 消除“优先/仅查询”歧义文案，准确表述 `UseIPv6 (仅查询 IPv6 AAAA 记录)` 及 `UseIP` 的双栈回退配合说明。
- [ ] AC-4：`populateOutboundForm` 与 `sanitizeOutboundPayload` 能正确回显与序列化 `UseIPv4v6` / `UseIPv6v4`，正确归一化 `targetStrategy: "AsIs"` 为 `""`，并消除 Freedom 节点下 `targetStrategy` / `sockopt.domainStrategy` 的竞争覆盖。
- [ ] AC-5：`internal/adapter/xray/compiler_test.go` 覆盖 `UseIPv4v6` / `UseIPv6v4` 编译与 `xray.exe run -test` 校验，且全量质量门禁（`go test ./...`、`go vet ./...`、`go build .`、`cd web && npm run build`）100% 通过。
