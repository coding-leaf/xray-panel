# Implementation Plan (Sub-2)

## Checklist

1. [x] 更新 `web/src/views/outbounds/types.ts`：扩展 `OutboundItem`, `OutboundFormState`, `createDefaultOutboundForm`, `populateOutboundForm`, `sanitizeOutboundPayload`, `formatSettingsSummary`。
2. [x] 更新 `web/src/views/outbounds/components/OutboundFormDrawer.vue`：新增 Mux & XUDP 配置区（含 Vision 互斥处理）及高级链路区（`dialerProxy`, `sendThrough`, `targetStrategy`）。
3. [x] 更新 `web/src/views/outbounds/components/OutboundTable.vue` 与 `OutboundDetailDrawer.vue`：展示 `MUX` 与 `dialerProxy` / `sendThrough` 徽章及详情。
4. [x] 更新 `web/src/views/topology/components/QuickPublishWizardModal.vue`：支持导入外部节点时勾选启用 Mux & XUDP。

## Validation Commands

```bash
cd web && mise x -- npm run build
```
