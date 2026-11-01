# 修复前后端配置失配与编译器数据丢失 - 实施计划 (Implement Plan)

## 1. 验证策略与失败用例先行 (Fail-repro First)

在着手修改产品代码前，遵循 Trellis 规范编写对应失败测试（红灯先行），复现缺陷后编写最小实现直至全绿：

### 1.1 DNS 编译器数据丢失复现 (Red-to-Green 1)
- **目标测试**：`internal/adapter/xray/compiler_test.go`
- **红灯用例**：`TestCompileDNS_PreservesAllFields`
  - 构造包含 `ClientIP: "1.2.3.4"`, `DisableCache: true`, `DisableFallback: true`, `Hosts: {"example.com": "1.1.1.1"}`, `Tag: "dns-custom"` 的 `domain.DNSConfig`；
  - 运行 `compiler.Compile(...)`；
  - 断言 `cfg.DNS.ClientIP == "1.2.3.4"`、`cfg.DNS.DisableCache == true`、`cfg.DNS.DisableFallback == true`、`cfg.DNS.Hosts["example.com"] == "1.1.1.1"`、`cfg.DNS.Tag == "dns-custom"`；
  - 当前代码下断言失败（字段直接丢失且 Tag 被硬编码为 "dns-in"）。

### 1.2 订阅协议转换器传输层参数提取复现 (Red-to-Green 2)
- **目标测试**：`internal/protocol/node_converter_test.go`
- **红灯用例**：`TestInboundToNodeConfig_HttpUpgradeAndKCP`
  - 构造 `Network: "httpupgrade"`, `httpupgradeSettings: {"path": "/up", "host": "up.test.com"}` 的入站；
  - 构造 `Network: "mkcp"`, `kcpSettings: {"header": {"type": "wechat-video"}, "seed": "kcp-seed-123"}` 的入站；
  - 运行 `InboundToNodeConfig` 并断言生成的 `NodeConfig` 中包含正确的 `path`, `host`, `headerType`, `seed` 参数；
  - 当前代码下断言失败（未处理 `httpupgrade` 与 `mkcp` 分支）。

### 1.3 GeoData 服务自定义目录读取复现 (Red-to-Green 3)
- **目标测试**：`internal/service/geodata_service_test.go`
- **红灯用例**：`TestGeoDataService_CustomGeoDataDirFromSettings`
  - Mock `SettingRepository` 返回 `xray_geodata_dir = "/custom/geodata/dir"`；
  - 创建临时测试目录，调用 `svc.GetStatus(ctx)`；
  - 断言返回的 `status.TargetDirectory` 为 `"/custom/geodata/dir"`；
  - 当前代码下断言失败（未注入仓储，始终返回二进制或系统官方默认目录）。

---

## 2. Ordered Implementation Checklist

### Phase 1: 后端核心修复 (Fail-repro First & Backend Fixes)
- [ ] **1.1 DNS 编译器测试先行与保真修复 (R2)**
  - 在 `internal/adapter/xray/compiler_test.go` 编写 `TestCompileDNS_PreservesAllFields`（验证红灯）；
  - 在 `internal/adapter/xray/schema.go:173-177` 扩充 `XrayDNSConfig` 字段（`ClientIP`, `DisableCache`, `DisableFallback`, `Hosts`, `Tag`）；
  - 在 `internal/adapter/xray/compiler.go:112-127` 完整映射 `dns` 领域模型字段（验证绿灯）；
  - 运行 `mise x -- go test ./internal/adapter/xray/...`。
- [ ] **1.2 GeoData 目录系统设置注入与修复 (R6)**
  - 在 `internal/service/geodata_service_test.go` 编写设置目录优先级单测（验证红灯）；
  - 在 `internal/service/geodata_service.go` 中为 `GeoDataService` 增加 `settingRepo domain.SettingRepository` 字段，并在 `getGeoDataDir(ctx)` 优先读取 `xray_geodata_dir`；
  - 在 `main.go:153` 装配 `settingRepo` 依赖；
  - 运行 `mise x -- go test ./internal/service/...`（验证绿灯）。
- [ ] **1.3 传输层 Schema 与订阅协议转换器增强 (R3 后端部分)**
  - 在 `internal/protocol/node_converter_test.go` 编写 `TestInboundToNodeConfig_HttpUpgradeAndKCP`（验证红灯）；
  - 在 `internal/adapter/xray/schema.go:75-86` 为 `XrayStreamSettings` 增加 `HTTPUpgradeSettings` 与 `KCPSettings` 结构定义；
  - 在 `internal/protocol/node_converter.go` 扩展 `inboundRawStream` 并在 `switch network` 中处理 `httpupgrade` 与 `kcp`/`mkcp`（验证绿灯）；
  - 运行 `mise x -- go test ./internal/protocol/...`。

### Phase 2: 入站前端 UI 与序列化对齐 (R1, R3 前端部分)
- [ ] **2.1 入站类型与序列化函数增强**
  - 在 `web/src/views/inbounds/types.ts` 中的 `InboundFormData` 与 `InboundPayload` 增加 `remark: string`；
  - 在 `InboundFormData` 增加 `sniffingDestOverride: string[]`（缺省 `['http', 'tls', 'quic']`）；
  - 在 `InboundFormData` 增加 `tlsAlpn: string`、`fallbackAlpn: string`、`fallbackPath: string`；
  - 增加 `httpupgrade`（`httpupgradePath`, `httpupgradeHost`）与 `mkcp`（`kcpHeaderType`, `kcpSeed`, `kcpMtu`, `kcpTti`, `kcpUplinkCapacity`, `kcpDownlinkCapacity`, `kcpCongestion`）字段；
  - 更新 `getDefaultInboundFormData`、`sanitizeInboundPayload`、`buildSettingsJSON`、`buildStreamSettingsJSON`、`buildSniffingJSON`。
- [ ] **2.2 入站抽屉组件模板与回显补齐 (`InboundFormDrawer.vue`)**
  - 在基本信息卡片增加 `form.remark` 输入框与顶层 `form.routeId` 输入框；
  - 在嗅探设置卡片补齐 `form.sniffingRouteOnly` 切换开关与 `sniffingDestOverride` 复选框组；
  - 在 TLS 卡片增加 `form.tlsAlpn` 输入框；在 Fallback 区域增加 `form.fallbackAlpn` 与 `form.fallbackPath`；
  - 当 `form.network === 'httpupgrade'` 时展示 HTTPUpgrade 专用参数区域；当 `form.network === 'mkcp'` 时展示 mKCP 专用参数区域；
  - 在 `initFormData` 中完整解析回显上述所有新增属性。

### Phase 3: 出站前端 UI 与参数对齐 (R4 前端部分)
- [ ] **3.1 出站类型与序列化函数增强 (`web/src/views/outbounds/types.ts`)**
  - 在 `OutboundFormState` 增加 `realitySpiderX: string`、`wgMtu: number`、`wgNoKernelTun: boolean`、`httpupgradePath: string`、`httpupgradeHost: string`；
  - 更新 `buildStreamSettingsJSON`（输出 `realitySettings.spiderX`，支持 `httpupgradeSettings`）；
  - 更新 `buildSettingsJSON`（输出 WireGuard 的可变 `mtu` 与 `noKernelTun`）；
  - 更新 `parseOutboundToFormState`，完整反向解析回显。
- [ ] **3.2 出站抽屉组件模板与回显补齐 (`OutboundFormDrawer.vue`)**
  - 在 TLS 参数卡片增加 TLS 指纹伪装下拉选择控件（`form.tlsFingerprint`）；
  - 在 Reality 卡片增加 SpiderX 输入控件（`form.realitySpiderX`）；
  - 在 WireGuard 卡片开放 MTU 输入框与 NoKernelTun 开关；
  - 当 `form.streamNetwork === 'httpupgrade'` 时展示 HTTPUpgrade 参数卡片。

### Phase 4: 路由分流规则全栈连通 (R5 前端部分)
- [ ] **4.1 路由抽屉状态与保存补齐 (`web/src/views/RoutingView.vue`)**
  - 在 `ruleForm` 状态中增加 `tag: string` 与 `userStr: string`；
  - 在 `openAddRuleDrawer` 与 `editRule` 中完成初始化与切片回显；
  - 在 `saveRuleInDrawer` 中组装 `newRule.tag` 与 `newRule.user`。
- [ ] **4.2 路由列表与移动端展示卡片补齐 (`web/src/views/RoutingView.vue`)**
  - 在桌面端规则表格中渲染 `rule.tag` 徽章与 `rule.user` 条件标签；
  - 在移动端规则卡片中同步展示 `rule.tag` 与 `rule.user` 条件标签。

### Phase 5: 全套质量门禁与端到端静态检查
- [ ] **5.1 运行全套后端测试**：`mise x -- go test ./...`
- [ ] **5.2 运行后端静态分析**：`mise x -- go vet ./...`
- [ ] **5.3 运行前端生产编译**：`cd web && mise x -- npm run build`

---

## 3. 高危文件与爆炸半径分析 (Risky Files & Blast Radius)

| 文件路径 | 关键职责 | 潜在回归风险 | 防御与隔离手段 |
| :--- | :--- | :--- | :--- |
| `internal/adapter/xray/compiler.go` | Xray 核心 JSON 编译中枢 | 修改 DNS 映射若影响 Tag 或 Servers 默认值，可能导致 Xray 启动解析失败 | 严格保留空值兜底逻辑（`Servers` 缺省 Google/Cloudflare，`Tag` 缺省 `dns-in`）；通过完整编译测试套件覆盖。 |
| `internal/adapter/xray/schema.go` | Xray 官方 JSON 结构体映射 | 标签或类型不匹配导致 Xray 核心拒绝载入配置 | 严格对照 Xray-core 官方文档 `dns`、`kcpSettings`、`httpupgradeSettings` 结构体规范编写 `json` tag。 |
| `internal/protocol/node_converter.go` | 订阅节点参数提取器 | 错误修改已有分支可能破坏 VLESS/Trojan 订阅生成 | 仅在 `switch network` 中安全增添分支，不修改已有 `ws`/`grpc`/`xhttp` 逻辑；伴随单测全量回归。 |
| `internal/service/geodata_service.go` | 规则库文件管理与下载 | 仓储未初始化或网络超时阻塞 | 增加 `s.settingRepo != nil` 空指针防御，超时或查询失败立即降级至默认目录，绝不抛出 panic。 |
| `web/src/views/inbounds/types.ts` | 入站表单数据裁剪与入库中枢 | `sanitizeInboundPayload` 若遗漏字段会导致编辑后丢配置 | 新增字段同步注册到默认值、清洗白名单与序列化器三处，实施严格对应。 |
| `web/src/views/RoutingView.vue` | 路由规则可视化配置与排序 | 规则对象字段结构异常影响拖拽与位次调整 | `tag` 与 `user` 均为纯展示与过滤字段，不参与规则排序下标计算。 |

---

## 4. 验证命令与终端门禁

在提交本任务前，必须在终端依次执行并通过：

```bash
# 1. 验证后端测试全绿
mise x -- go test ./...

# 2. 验证后端静态类型检查
mise x -- go vet ./...

# 3. 验证前端 Vite 编译与 TypeScript 检查
cd web && mise x -- npm run build
```
