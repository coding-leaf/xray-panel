# 修复前后端配置失配与编译器数据丢失 (PRD)

## Goal
修复面板在入站、出站、DNS、路由与系统设置等核心配置流中的前后端脱节、数据静默丢失与未暴露控件问题，消除配置往返丢属性、前端有字段无控件、后端有能力无界面的架构裂痕，确保前后端契约严格对齐与配置编译无损往返。

## Background & Confirmed Facts
基于对代码库的精确审计，确认以下六类事实源缺陷：

1. **入站流量嗅探 routeOnly 与 destOverride 脱节**：
   - `web/src/views/inbounds/types.ts:78,143,164,494` 与 `web/src/views/inbounds/components/InboundFormDrawer.vue:1034` 已有 `sniffingRouteOnly` 序列化与回显代码，但 `InboundFormDrawer.vue:323-336`（“④ 回落伪装与流量探测”）模板遗漏了 `form.sniffingRouteOnly`（仅限路由）的 UI 开关控件；
   - `web/src/views/inbounds/types.ts:493` 在 `buildSniffingJSON` 中将 `destOverride` 强行硬编码为 `['http', 'tls', 'quic']`，无法配置 `fakedns` 等探测目标，且未在表单与回显逻辑中暴露。

2. **DNS 编译器静默丢数据 (Data Loss)**：
   - 领域模型 `internal/domain/dns.go:5-10` 定义了 `ClientIP`, `DisableCache`, `DisableFallback`, `Hosts`；
   - Xray 适配器模式定义 `internal/adapter/xray/schema.go:173-177`（`XrayDNSConfig`）仅包含 `Servers`, `QueryStrategy`, `Tag`；
   - 编译器 `internal/adapter/xray/compiler.go:112-124` 编译时直接丢弃了 `ClientIP`, `DisableCache`, `DisableFallback`, `Hosts`，且强制将 `Tag` 覆盖为 `"dns-in"`；
   - 导致配置服务 `internal/service/config_service.go:628-644`（`GetDNSConfig` 与 `SaveDNSConfig`）在保存后再次读取时数据发生不可逆丢失。

3. **入站配置项未暴露与传输层支持残缺**：
   - `remark`（节点备注）：`internal/domain/inbound.go:17` 持久化字段及 `internal/service/sub_service.go:204,218` 节点订阅命名强依赖 `inbound.Remark`，但 `web/src/views/inbounds/types.ts`（`InboundFormData` / `InboundPayload`）和 `InboundFormDrawer.vue` 均缺少 `remark` 字段与输入框；
   - `routeId`（默认路由编号）：`InboundFormDrawer.vue:897` 与 `types.ts:507` 虽能接收顶层 `routeId`，但抽屉模板未提供顶层 `form.routeId` 输入控件（仅有子路由 `sr.routeId` 控件）；
   - `httpupgrade` 与 `mkcp` 传输协议支持脱节：`InboundFormDrawer.vue:105-106` 与 `OutboundFormDrawer.vue:229` 允许用户选择 `httpupgrade` 和 `mkcp`，但 `internal/adapter/xray/schema.go` 的 `XrayStreamSettings` 缺少对应结构体定义；前端 `types.ts` 和抽屉没有对应传输参数表单；`internal/protocol/node_converter.go:170-220` 生成订阅链接时完全未解析 `httpupgrade` / `mkcp` 参数；
   - TLS `alpn` 与 Fallback `alpn`/`path` 缺失：`web/src/views/inbounds/types.ts:373-380, 421-428` 与 `InboundFormDrawer.vue:290-348` 缺失 TLS `alpn` 配置及 Fallback 高级匹配条件（`alpn`, `path`）。

4. **出站高级配置项未暴露**：
   - `tlsFingerprint`：`web/src/views/outbounds/types.ts:38,343` 已定义 `tlsFingerprint` 并序列化，但 `OutboundFormDrawer.vue:358-368` 缺失指纹选择控件；
   - Reality `spiderX` 与 WireGuard `mtu` / `noKernelTun`：`web/src/views/outbounds/types.ts:202-215` 将 WireGuard 的 `noKernelTun` 硬编码为 `true`，`mtu` 硬编码为 `1280`，Reality 出站缺少 `spiderX` 配置字段与输入。

5. **路由规则缺失 `user` 与 `tag` 字段**：
   - 领域层 `internal/domain/outbound.go:11,21` 与核心编译器 `internal/adapter/xray/compiler.go:232` 已完整支持 `User`（用户 Email 分流）和 `Tag`（规则标识）；
   - 前端 `web/src/views/RoutingView.vue:862-954` 的 `ruleForm`、编辑抽屉、保存逻辑以及规则列表卡片完全遗漏了 `user` 和 `tag`。

6. **系统设置幽灵配置项 `xray_geodata_dir`**：
   - `web/src/views/SettingsView.vue:70-79` 与持久化仓储 `settingRepo` 已支持保存 `xray_geodata_dir`，但 `internal/service/geodata_service.go:57-65`（`getGeoDataDir`）直接忽略系统设置，始终使用硬编码回退逻辑，且 `main.go:153` 未向 `GeoDataService` 注入 `settingRepo`。

## Requirements

### R1. 入站流量嗅探高级参数完备化 (Sniffing RouteOnly & DestOverride)
- **R1.1 RouteOnly 界面控件**：在 `InboundFormDrawer.vue` 的“④ 回落伪装与流量探测”区域中，当“启用域名嗅探 (Sniffing)”勾选时，展示“仅限路由 (RouteOnly)”开关控件，双向绑定 `form.sniffingRouteOnly`。
- **R1.2 DestOverride 可选探测协议**：
  - 在 `web/src/views/inbounds/types.ts` 的 `InboundFormData` 中增加 `sniffingDestOverride: string[]`（缺省值为 `['http', 'tls', 'quic']`）；
  - 在 `InboundFormDrawer.vue` 嗅探开启时提供 `http`, `tls`, `quic`, `fakedns` 的多选复选框；
  - `types.ts:buildSniffingJSON` 使用用户勾选的 `clean.sniffingDestOverride` 序列化；
  - `InboundFormDrawer.vue:initFormData` 正确解析后端回传的 `sniff.destOverride`。

### R2. DNS 编译器数据往返保真 (DNS Compiler Data Preservation)
- **R2.1 契约扩充**：在 `internal/adapter/xray/schema.go` 的 `XrayDNSConfig` 中补充 `ClientIP`, `DisableCache`, `DisableFallback`, `Hosts`，与 `internal/domain/dns.go` 保持字段名与 JSON 标签完全一致。
- **R2.2 编译器保真映射**：在 `internal/adapter/xray/compiler.go` 编译 DNS 时，完整将 `dns.ClientIP`, `dns.DisableCache`, `dns.DisableFallback`, `dns.Hosts` 映射到 `XrayDNSConfig`；若 `dns.Tag` 非空则使用用户指定 Tag，为空时兜底使用 `"dns-in"`。
- **R2.3 往返无损断言**：在 `internal/adapter/xray/compiler_test.go` 中增加覆盖所有高级 DNS 字段的单测，验证 `ConfigService` 读写无损。

### R3. 入站配置项补齐 (Inbound Config Parity & Transports)
- **R3.1 节点备注 (Remark)**：
  - `web/src/views/inbounds/types.ts` 中的 `InboundFormData` 与 `InboundPayload` 增加 `remark: string`；
  - `getDefaultInboundFormData`, `sanitizeInboundPayload`, `buildInboundPayload` 支持 `remark` 校验与透传；
  - `InboundFormDrawer.vue` 在基本信息卡片中提供“节点备注 (Remark)”输入框，并在编辑初始化时回显 `inb.remark`。
- **R3.2 顶层默认路由编号 (RouteID)**：
  - 在 `InboundFormDrawer.vue` 基本信息或高级路由区域提供顶层 `form.routeId`（默认路由编号 / 协议级 16 位路由标识，0 为不替换）输入控件。
- **R3.3 HTTPUpgrade 与 mKCP 传输协议全链路支持**：
  - 在 `internal/adapter/xray/schema.go` 的 `XrayStreamSettings` 中增加 `HTTPUpgradeSettings`（`path`, `host`, `headers`）和 `KCPSettings`（`mtu`, `tti`, `uplinkCapacity`, `downlinkCapacity`, `congestion`, `readBufferSize`, `writeBufferSize`, `header`, `seed`）；
  - 在 `web/src/views/inbounds/types.ts` 和 `InboundFormDrawer.vue` 增加 `httpupgrade`（Path, Host）与 `mkcp`（Header Type, Seed, MTU, TTI, Uplink/Downlink Capacity, Congestion）的表单定义、回显与 `buildStreamSettingsJSON`；
  - 在 `internal/protocol/node_converter.go` 中解析 `httpupgrade`（Path, Host）与 `kcp`/`mkcp`（HeaderType, Seed）参数，输出到订阅节点的通用参数中。
- **R3.4 TLS ALPN 与 Fallback 高级条件**：
  - `web/src/views/inbounds/types.ts` 与 `InboundFormDrawer.vue` 增加 `tlsAlpn`，在 `buildStreamSettingsJSON` 生成 `stream.tlsSettings.alpn`；
  - Fallback 项补充 `fallbackAlpn` 与 `fallbackPath`，在 `buildSettingsJSON` 中输出到 `settings.fallbacks[0].alpn` 与 `path`。

### R4. 出站配置项补齐 (Outbound Config Parity)
- **R4.1 TLS 指纹控件**：
  - 在 `web/src/views/outbounds/components/OutboundFormDrawer.vue` 的 TLS 专属参数区域，添加 TLS 指纹伪装下拉选择控件（`chrome`, `firefox`, `safari`, `ios`, `edge`, `random`, `randomized`），绑定 `form.tlsFingerprint`。
- **R4.2 Reality SpiderX 支持**：
  - 在 `web/src/views/outbounds/types.ts` 的 `OutboundFormState` 中增加 `realitySpiderX: string`；
  - 在 `OutboundFormDrawer.vue` 的 Reality 参数卡片中增加 SpiderX 输入框；并在 `buildStreamSettingsJSON` 与 `parseOutboundToFormState` 中正确读写。
- **R4.3 WireGuard MTU 与 NoKernelTun 可配**：
  - 在 `web/src/views/outbounds/types.ts` 的 `OutboundFormState` 中增加 `wgMtu: number`（默认 1280）与 `wgNoKernelTun: boolean`（默认 true）；
  - 在 `OutboundFormDrawer.vue` 中开放 MTU 输入框与 NoKernelTun 开关，并在 `buildSettingsJSON` 与 `parseOutboundToFormState` 中双向绑定。
- **R4.4 HTTPUpgrade 出站传输参数**：
  - 在 `web/src/views/outbounds/types.ts` 与 `OutboundFormDrawer.vue` 中补齐 `httpupgradePath` 与 `httpupgradeHost` 表单与序列化。

### R5. 路由分流规则完备化 (Routing Rule User & Tag)
- **R5.1 规则抽屉表单补全**：
  - 在 `web/src/views/RoutingView.vue` 的 `ruleForm` 增加 `tag: string`（规则备注/标识）与 `userStr: string`（按用户 Email 分流，逗号或换行分隔）；
  - 在抽屉模板中添加“规则标识 / 备注 (Tag)”与“用户匹配列表 (User，可选)”输入控件；
  - 在 `openAddRuleDrawer`, `editRule`, `saveRuleInDrawer` 中正确初始化、解析数组与保存。
- **R5.2 规则展示列表与卡片增强**：
  - 在桌面端规则表格中展示 `rule.tag`（若有）与 `rule.user`（条件匹配徽章）；
  - 在移动端规则卡片中同步展示 `rule.tag` 与 `rule.user`。

### R6. GeoData 规则库存储目录配置实装 (GeoData Dir Injection)
- **R6.1 依赖注入仓储**：在 `internal/service/geodata_service.go` 中为 `GeoDataService` 注入 `settingRepo domain.SettingRepository`（通过构造函数或链式配置注入），在 `main.go:153` 完成依赖装配。
- **R6.2 动态读取系统设置**：`geodata_service.go:getGeoDataDir(ctx)` 优先从 `settingRepo` 读取 `xray_geodata_dir`，非空时采用该配置值；仅在未配置或读取失败时回退至原有的官方默认与二进制所在目录。

## Acceptance Criteria

- [ ] **AC1 (对应 R1)**：入站抽屉开启嗅探后可配置 RouteOnly 与选择 DestOverride（`http`, `tls`, `quic`, `fakedns`），保存后 `sniffingJson` 正确记录所选项，重新打开抽屉回显一致。
- [ ] **AC2 (对应 R2)**：执行 `compiler_test.go`，验证包含 `ClientIP`, `DisableCache`, `DisableFallback`, `Hosts`, `Tag` 的 DNS 配置经由 `XrayCompiler.Compile` 能够完整保留在编译产物中，无静默字段丢失。
- [ ] **AC3 (对应 R3)**：
  - 入站抽屉支持配置 `remark` 与顶层 `routeId`，保存并落库，且订阅生成时显示自定义 `remark`；
  - 入站和出站支持配置 `httpupgrade`（Path, Host）与 `mkcp`（HeaderType, Seed 等），`schema.go` 序列化无误，`node_converter.go` 能成功输出订阅参数；
  - 入站支持配置 TLS `alpn` 及 Fallback `alpn`/`path`。
- [ ] **AC4 (对应 R4)**：出站抽屉支持选择 TLS 指纹、配置 Reality `spiderX`、调整 WireGuard `mtu` 与 `noKernelTun`，以及配置 HTTPUpgrade 传输参数，保存与回显一致。
- [ ] **AC5 (对应 R5)**：路由规则抽屉支持输入 `tag` 与 `user`，保存后规则包含 `tag` 与 `user`，路由列表和移动端卡片能清晰显示用户分流条件与规则标识，经后端编译可进入 `XrayRoutingRule`。
- [ ] **AC6 (对应 R6)**：当通过系统设置设定 `xray_geodata_dir` 为自定义目录时，`GeoDataService.GetStatus` 返回的目标目录为该自定义路径，单测验证通过。
- [ ] **AC-Gates (质量门禁)**：
  - `mise x -- go test ./...` 全绿；
  - `mise x -- go vet ./...` 无告警；
  - `cd web && mise x -- npm run build` 前端生产编译与类型检查零错误。

## Out of Scope
- 面板版本号动态注入与顶栏展示（由 Child Task 2: `11-01-feat-dynamic-panel-version` 负责）；
- 用户多选批量操作与订阅高度自定义策略组（由 Child Task 3: `11-01-enhance-user-and-sub-customizability` 负责）；
- 在真实环境运行 `temp/` 下 Xray 二进制进行端到端集成验收（由 Child Task 4: `11-01-verify-xray-real-binary-acceptance` 负责）；
- 不做数据库破坏性 Schema 变更，不更改 SQLite 表主键策略。
