# Fix Inbound and Outbound Vision Flow Disable Bug

## Goal

修复面板入站与出站在 VLESS+TCP（TLS/REALITY）下，关闭 Vision 流控后重新查看/编辑面板仍然显示 Vision 开启、无法持久化关闭状态的缺陷。

## Requirements

1. **入站 (Inbounds) 流控关闭与回显一致性**：
   - 在入站编辑/创建抽屉 (`InboundFormDrawer.vue`) 中，流控策略选项 "none (无流控)" 的值规范化为 `'none'`。
   - 在数据清洗 `sanitizeInboundPayload` 中，保留用户选择的 `'none'` 状态，禁止将空/none 错误 fallback 覆盖为 `'xtls-rprx-vision'`。
   - 在 `buildSettingsJSON` 中，当流控为 `'none'` 时，明确写入 `settings.flow = 'none'`，且针对分配的客户端不写入 `flow`（Xray 客户端不支持 `'none'`），保证 Xray 配置语法正确。
   - 在编辑抽屉打开时，准确解析 `settings.flow`（若为 `'none'` 或 `''` 则对应无流控 `'none'`），使下拉框正确选中 "none (无流控)"。
   - 在网络/安全模式变动函数（`onNetworkChange`, `onSecurityChange`, `onProtocolChange`）中，避免将用户主动选择的 `'none'` 无条件重置回 `'xtls-rprx-vision'`。

2. **出站 (Outbounds) 流控关闭与回显一致性**：
   - 在出站编辑/创建抽屉 (`OutboundFormDrawer.vue`) 中，流控策略选项 "none (无流控)" 的值统一规范化为 `'none'`。
   - 在 `buildSettingsJSON` 中，顶层记录 `flow: flowVal`（若为 `'none'` 则写入 `'none'`），而对于 `vnext[0].users[0].flow`，若非 vision 流控则设为空串 `""`，符合 Xray-core 出站语法。
   - 在 `populateOutboundForm` 中，优先读取 `s.flow` 或 `u.flow`，若值为 `'none'` 或 `""` 则回显为 `'none'`，仅在新建且全新无配置时才按协议安全规则给出默认值。
   - 在 `onOutboundSecurityChange` 等联动事件中，保留用户的 `'none'` 选择，不强制变更为 `'xtls-rprx-vision'`。

3. **后端节点转换与订阅链接一致性**：
   - 在 `internal/protocol/node_converter.go` 中，当入站 `settingsMap["flow"]` 显式设为 `"none"` 时，彻底禁用流控，不得 fallback 到 `user.Flow`。

## Acceptance Criteria

- [x] 入站配置为 VLESS + TCP + REALITY/TLS 时，将流控切换为 "none (无流控)" 并保存，重新打开抽屉查看，流控选项保持为 "none (无流控)"，不再变回 "xtls-rprx-vision"。
- [x] 出站配置为 VLESS + TCP + REALITY/TLS 时，将流控切换为 "none (无流控)" 并保存，重新打开抽屉查看，流控选项保持为 "none (无流控)"，不再变回 "xtls-rprx-vision"。
- [x] 入站列表与出站列表配置摘要正确反映关闭状态（入站列表不显示 flow badge，出站列表摘要无额外 flow 后缀）。
- [x] 后端单元测试与编译门禁全绿：`mise x -- go test ./...`，`mise x -- go vet ./...`，`mise x -- go build .`。
- [x] 前端类型检查与构建全绿：`cd web && mise x -- npm run typecheck`，`cd web && mise x -- npm run build`。
