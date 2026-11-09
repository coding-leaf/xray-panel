# Technical Design — Add IPv4/IPv6 Prefer Strategy Options

## 1. Architecture & Boundaries

本次变更遵循项目极简规范（KISS）与分层边界：

- **Frontend (`web/src/views/`)**：
  - `web/src/views/outbounds/components/OutboundFormDrawer.vue`：
    - 更新 Freedom `domainStrategy` 与通用 Outbound `targetStrategy` 的 `<select>` 选项列表及中文语义说明；
    - 当 `form.protocol === 'freedom'` 时隐藏底部高级路由卡片中的 `targetStrategy` 字段，并在 `onOutboundProtocolChange` 切到 `freedom` 时清空 `form.targetStrategy`；
    - 为双栈优先回退选项提供 DNS `UseIP` 联动提示。
  - `web/src/views/outbounds/types.ts`：
    - 在 `populateOutboundForm` 中归一化 `ob.targetStrategy === 'AsIs'` 为 `''`；
    - 当 `ob.protocol === 'freedom'` 时，`freedomDomainStrategy` 按 `s.domainStrategy || str.sockopt?.domainStrategy || ob.targetStrategy || 'UseIPv4'` 顺序回退读取，并将 `form.targetStrategy` 置空、剔除 `rawSockopt.domainStrategy`，防止多处策略字段互相覆盖；
    - 在 `sanitizeOutboundPayload` 中确保 `form.protocol === 'freedom'` 时 `targetStrategy` 输出 `''`。
  - `web/src/views/DNSView.vue`：修正 `dns.queryStrategy` 中 `UseIP` 与 `UseIPv6` 的描述文案。
- **Backend (`internal/`)**：
  - `internal/domain/outbound.go` 与 `internal/adapter/xray/compiler.go` 已经对 `TargetStrategy` 及 `SettingsJSON` 采用字符串/RawMessage 无损透传，无需增加多余抽象。
  - `internal/adapter/xray/compiler_test.go`：补充 `UseIPv4v6` / `UseIPv6v4` 在 Freedom 与非 Freedom 出站下的编译与二进制校验测试。

## 2. Data Flow & Contracts

1. **Freedom 出站 `domainStrategy` 数据流**：
   - 用户在 `OutboundFormDrawer.vue` 选择 `UseIPv4v6` 或 `UseIPv6v4`；
   - `buildSettingsJSON(form)` (`web/src/views/outbounds/types.ts:316-320`) 序列化为 `{"domainStrategy":"UseIPv4v6"}`，且 `targetStrategy` 保持为空；
   - `POST /api/v1/outbounds` → `ConfigService.SaveOutbound` → `XrayCompiler.CompileToJSON` 原样写入 `outbounds[i].settings.domainStrategy`；
   - 重新打开编辑抽屉时，`populateOutboundForm(ob)` 从 `s.domainStrategy` 读取 `"UseIPv4v6"` 并绑定回 `<select>`。

2. **非 Freedom 出站 `targetStrategy` 数据流**：
   - 用户在 `OutboundFormDrawer.vue` 选择 `UseIPv4v6` / `UseIPv6v4` / `ForceIPv4v6` / `ForceIPv6v4`；
   - `sanitizeOutboundPayload(form)` 将 `targetStrategy` 透传至后端；
   - `XrayCompiler.compileOutbound` (`compiler.go:481`) 写入 `XrayOutbound.TargetStrategy`。

## 3. Compatibility & Migration Notes

- **向后兼容**：新建 Freedom 节点默认值仍保持 `UseIPv4`，新建其他出站节点 `targetStrategy` 默认值仍保持 `""`（即 `AsIs`），不改变存量节点行为。
- **存量节点回显安全**：
  - 存量若配置了 `targetStrategy: "AsIs"`，打开表单自动映射至 `""`（默认 AsIs 项），不会出现空白下拉框；
  - 存量 Freedom 节点若配置了 `ForceIP*` 或通过 `sockopt.domainStrategy` 配置了策略，打开表单均可无损回显。
- **关于 Xray-core `freedom.domainStrategy` 弃用警告的权衡**：
  - Xray-core v26 中 `freedom.settings.domainStrategy` 仍正常生效并自动内部迁移至 `sockopt.domainStrategy`。由于现有代码库与测试套件（如 `mock/storage.ts`、`formatSettingsSummary`）均以 `settings.domainStrategy` 作为 Freedom 的标准存储契约，本次保持该存储位置不变，同时在 `populateOutboundForm` 中兼容读取 `sockopt.domainStrategy`。
