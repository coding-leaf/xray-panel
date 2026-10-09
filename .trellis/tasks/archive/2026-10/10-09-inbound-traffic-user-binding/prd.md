# Inbound Traffic User Binding

## Goal

支持为特定入站网关（Inbound，如专供服务器本机直连的 SOCKS5/HTTP 代理或独立端口）配置可选的“流量归属用户（Bound User）”。当该入站产生出入站流量时，流量同步引擎（TrafficSyncJob）不仅统计该入站的吞吐，还自动将该流量增量无缝核算归属至指定用户名下，使该用户的总流量、日流量日志及额度熔断策略全面生效。

## Context & Evidence

- 现网在 `vps-jp` 上运行的 Xray 存在专供本机直连的 SOCKS 入站（tag: `local`, 端口 10808，`auth: noauth`）。
- Xray 的 StatsService 原生上报 `inbound>>>local>>>traffic>>>uplink/downlink`，但因无认证不会产出 `user>>>local@yezineko.top>>>traffic>>>...`。
- 面板现有 `TrafficSyncJob`（`internal/delivery/cron/traffic_sync.go`）已定时获取 `TrafficStatTypeInbound`，且通过 `BatchSyncTraffic` 批量累加入站流量。`InboundRepository`（`j.inboundRepo`）已注入到 `TrafficSyncJob` 中。
- 用户诉求：服务器直连 SOCKS 或免密代理产生的流量，需要能够统计并计费归属到指定用户（如 `local@yezineko.top`）。

## Requirements

1. **入站模型扩展 (Domain & Persistence)**:
   - `Inbound` 模型增加字段 `BoundUserEmail string`（映射字段 `bound_user_email`，数据库 varchar(128)，默认空）。
   - 创建/更新 Inbound 时支持保存、清除 `BoundUserEmail`。若填写的 Email 不存在，需返回清晰校验错误。
2. **流量同步引擎核算累加 (Traffic Accounting)**:
   - `TrafficSyncJob` 在每轮执行 `syncOnce` 时，若采集到 `TrafficStatTypeInbound` 的流量增量，且该 Inbound 绑定了有效用户（`BoundUserEmail != ""`）：
     - 将该 Inbound 的 `uplink` 和 `downlink` 增量**额外合并累加**到对应绑定用户的 `userDeltas[BoundUserEmail]` 中。
     - 同时保持该入站自身的 `inboundDeltas[tag]` 正常统计（入站节点总流量统计不受破坏）。
     - 用户流量累加后，原有触发的今日流量日志（`TrafficLog`）、用户总用量（`UpBytes`、`DownBytes`）、瞬时速率（`SpeedStats`）以及超额断网熔断判定全部正常生效。
   - 避免重复计费防线：若某个 Inbound 开启了多用户认证且 Xray 同时上报了该入站的 `user>>>...` 与 `inbound>>>...`，设计明确的归属规则或默认适用于单租户免密/本地入站。
3. **前端管理与配置交互 (Web UI)**:
   - 入站抽屉表单（`InboundFormDrawer.vue`）增加“流量归属用户”配置项（下拉可搜索选择系统用户，支持留空）。
   - 入站表格与详情抽屉中展示当前入站绑定的用户 Email（若有）。
   - 前端 Inbound types 与 Mock 数据同步扩展。

## Acceptance Criteria

- [x] `domain.Inbound` 拥有 `BoundUserEmail` 字段，GORM 自动迁移顺利落库。
- [x] 创建/更新 Inbound 时可绑定或解绑用户 Email，校验逻辑生效。
- [x] `TrafficSyncJob` 单测覆盖：当 `inbound>>>local` 产生流量增量且 `local` 绑定了用户 `local@yezineko.top` 时，该用户的 `up_bytes`、`down_bytes` 自动增长且生成今日流量日志。
- [x] 前端入站列表/抽屉中可正确选择并保存绑定用户，并清晰展示绑定关系。
- [x] 后端测试与静态检查通过：`go test ./...` 与 `go vet ./...` 全绿。
- [x] 前端生产构建通过：`npm run build` 无类型与构建错误。
