# Sub-3: 前端入站反代嗅探与路由源地址规则对齐

## Goal

补齐前端入站管理（`web/src/views/inbounds/`）中的 `sockopt.acceptProxyProtocol` 与高级 Sniffing（`metadataOnly`、`domainsExcluded`、`ipsExcluded`）配置，以及路由管理（`web/src/views/RoutingView.vue`）中的客户端源 IP (`source`)、源端口 (`sourcePort`) 与域名匹配算法 (`domainMatcher`) 配置。

## Requirements

- **R3.1 (`web/src/views/inbounds/types.ts` & `InboundFormDrawer.vue` & `InboundDetailDrawer.vue`)**:
  - `InboundFormData` 新增：
    - `acceptProxyProtocol: boolean`（映射至 `streamSettings.sockopt.acceptProxyProtocol`，同时保留原 `sockopt` 其他字段）
    - `sniffingMetadataOnly: boolean`（映射至 `sniffing.metadataOnly`）
    - `sniffingDomainsExcluded: string`（逗号/换行分隔，映射至 `sniffing.domainsExcluded`）
    - `sniffingIpsExcluded: string`（逗号/换行分隔，映射至 `sniffing.ipsExcluded`）
  - 在 `InboundFormDrawer.vue` 的传输层与流量嗅探区域提供对应开关和输入框，并在 `InboundDetailDrawer.vue` 展示。
- **R3.2 (`web/src/views/RoutingView.vue`)**:
  - 路由规则类型与表单新增：
    - `source`（客户端源 IP / CIDR 列表，逗号分隔，如 `192.168.1.0/24`）
    - `sourcePort`（客户端源端口范围，如 `1024-65535`）
    - `domainMatcher`（域名匹配引擎下拉：默认空 / `hybrid` / `linear`）
  - 在路由规则列表卡片中展示 `source` / `sourcePort` 匹配条件标签。

## Acceptance Criteria

- [ ] `cd web && mise x -- npm run build` 零报错通过。
- [ ] 全仓后端 + 前端门禁全绿。
