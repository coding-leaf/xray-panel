# 技术设计

## 目标与最小行为差距

本机免认证入口已经具备原生入站计数和访问日志筛选能力，但旧修复复制流量到用户，仪表盘只汇总用户，入口到日志页面也没有筛选上下文。复用现有链路，统一现存入站累计与按入口看访问的行为。

## 核算数据流

```text
Xray 原生 user 增量    -> userDeltas    -> 用户累计 / 用户日账单 / 用户速率
Xray 原生 inbound 增量 -> inboundDeltas -> 入站累计
两者继续在现有 BatchSyncTraffic 原子事务落盘
现存业务入站累计求和  -> /dashboard totalUp/totalDown
Xray 访问日志文件      -> 已有 /logs?type=access&inbound=tag -> 日志视图
```

`TrafficSyncJob` 去掉 BoundUser 映射查询及额外合并；不增加 polling、goroutine 或数据库查询。现有用户重置防冲正、最终刷盘、原生配额逻辑和批量事务保留。任务不新增或修复本机入口配额机制。

## API 与数据库兼容

1. `Inbound.BoundUserEmail` 的 JSON 字段、数据库列与已有值暂保留，明确注释 deprecated / compatibility only。取消专用用户存在性校验，字段不进入核算、Xray 编译或授权决策。
2. 前端只移除可见表单与详情/列表展示，内部旧值可沿用现有表单状态与 payload 透传。升级及新版 UI 编辑保持旧值；外部 PUT 保持既有完整替换语义，省略字段仍可置空。新记录默认空，用户不能再配置此项。不要为保留一个遗留字段增加新 DTO/查询层。
3. 不删用户、不 DROP COLUMN、不清理或回填既有用户/入站累计及日账单；旧客户端携带字段仍兼容保存，但字段不再产生计费效果。
4. `/dashboard` 响应结构保持，`totalUp/totalDown` 语义修正为所有现存业务入站累计（含 disabled、排除内部 api）。这不是永久服务器账单；硬删除入站会从合计移除其历史计数。
5. 入站列表读取失败由 service 返回带上下文 error，HTTP 沿现有错误契约返回；不创建看似有效的零合计。用户人数仍由用户仓储统计。

## 前端行为

- 入站列表新增紧凑累计上传/下载列（复用 `formatBytes`），同步空态 colspan，宽表允许水平滚动；详情也显示同一入站计数。列表操作和详情操作区新增“访问日志”。
- 子组件只 emit logs，父 `InboundsView` 集中调用 router.push 到 `/logs`，query 为 `{ type: 'access', inbound: inb.tag }`。
- `LogsView` 对 type/inbound query 建立 immediate watcher，应用筛选后发起对应请求；删除 mounted 的重复首轮 fetch。流切换/入站下拉/点击日志 tag 更新 query，刷新及前进后退可复现上下文。
- 新入站导航清空用户和搜索筛选，保持匿名记录可见；用户之后可以手动过滤。query 支持 access/error/audit，非法 type 回归 access；仅 access 请求发送 inbound。
- type/inbound 变化清空过时视图数据；请求序号阻止旧响应/旧 loading 状态覆盖最新上下文；离页使旧请求提交失效并沿用 timer 清理。不新增自动刷新频率。
- structured 采用 case-sensitive tag 精确相等；raw 采用后端已有 `[tag ->` / `[tag]` 边界匹配；缺 tag 的记录不能假冒指定入口。将 query 中当前选中 tag 纳入下拉候选。
- 日志失败通过已有 toast 告知最新请求错误；空结果与读取失败区分。不扩展抓包或 HTTPS 解密能力。
- Dashboard 分别显示“主机累计”和“Xray 入站累计”，移除两种来源之间的逻辑或 fallback；后者说明为现存入站累计，不叠加用户累计。

## Mock 与局部验证

演示数据补入站累计和匿名回环 SOCKS 入口，使用模拟地址/域名。用户、入站、网卡累计刻意不同；Dashboard/mock stats 同源入站累计。访问 table/raw 样例按相同入口精确过滤。使用新隔离浏览器上下文运行 `dev:demo` 验证；不强制迁移/清空已有演示缓存。

## 修改边界

后端：`internal/domain/inbound.go` 注释；`internal/delivery/cron/traffic_sync.go` 及测试；`internal/service/config_service.go`、`monitor_service.go` 及测试；必要的 `internal/adapter/xray/log_reader_test.go` 合成匿名日志回归。仓储/API 路由结构复用，不另造采集器。

前端：`web/src/views/inbounds/components/{InboundFormDrawer,InboundTable,InboundDetailDrawer}.vue`；必要的 `web/src/views/inbounds/types.ts` 兼容注释；`InboundsView.vue`、`LogsView.vue`、`DashboardView.vue`；`web/src/mock/{index,storage}.ts`。同步生产产物 `web/dist`。

规范：更新 `.trellis/spec/backend/database-guidelines.md` 旧映射条款，补入站累计/遗留字段与日志导航合同到相应 backend/frontend spec。研究依据见 `research/findings.md`；已批准的新合同优先于旧映射条款。

## 风险、部署与回滚

- 旧账无法从当前 SSH 权限下核实，亦无法仅凭累计辨别过去是否双计；不自动纠账。
- 入站硬删除/重建会使累计合计变化，界面不承诺永久历史。入站每日趋势不在本次范围。
- 日志可见性受实际文件权限、保留/轮转及现有日志格式影响；本地合成日志验证，不宣称已经核实现网日志。
- 本任务交付仓库改动与检查，不部署或重启 VPS；后续上线只需更新面板，不要求改 local 的认证或启动配置。
- Schema 保持兼容。代码回退会恢复旧核算行为，可能恢复双计，不以整核重启/清空计数作为修复；任何现网回滚另行按部署审批执行。
