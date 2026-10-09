# 已确认事实与实施边界

## 用户决定与历史

用户明确只需要本机代理累计用量及访问目标；本机入口继续免认证。2026-10-10 获得任务创建/规划许可，尚未获得最终规划的实施审批。旧任务虽经用户审批，旧说明对双计防护、免认证入口配额与访问日志身份的承诺没有得到现有实现支持。相关本地历史主会话：`ses_edebaba32ffexHKQDTjCXnhnIH`，旧工件在 `.trellis/tasks/archive/2026-10/10-09-inbound-traffic-user-binding/`。

## VPS 轻量只读证据

- 已授权 `ssh vps-jp`，仅运行服务状态、版本/脱敏配置读取及一次非重置统计查询；没有生成代理测试流量、扫描磁盘、抓包或修改服务。
- Xray 版本为 26.9.30；本机 SOCKS 入口 `local` 监听回环地址、免认证、开启 UDP。
- 用户/入站/出站统计策略已开启；`local` 原生入站上下行计数器存在。
- 非重置查询当时显示零增量；面板周期读取并重置计数，所以不能推断没有业务流量。
- 面板服务由 root 运行，SSH 账号不能读取访问日志正文；非交互 sudo 要求密码，未继续提权。三个固定常见路径未找到可读数据库，未进行递归搜索。
- 尚不能声明现网数据库累计或具体访问日志已经核实；实施验收首先用本地真实仓储和合成日志闭环，不据现网空读结果推断业务故障。

## 核算与兼容

- `internal/delivery/cron/traffic_sync.go:140` 每周期构建旧绑定映射，`:164` 接收原生用户增量，`:195` 复制完整入站增量。删除映射与复制段即可停止错误归属，保留批量原子落盘。
- `internal/delivery/cron/traffic_sync_test.go:278` 旧绑定测试只验证复制成功，未防双计。改为遗留字段不产生业务效果的回归，使用模拟 email 与回环地址。
- `internal/domain/inbound.go:22` 字段同时承担 GORM/JSON 兼容。暂保留并标注 deprecated；`internal/service/config_service.go:347` 的专用校验及 `:371`、`:383` 调用退役。
- `internal/adapter/repository/inbound_repo.go:25` 完整 Save 会用省略字段的零值覆盖旧绑定。为保留遗留值，可沿用当前前端内部字段隐式透传，只移除可见配置与展示，并明确它没有业务效果。
- 不 DROP COLUMN、不修改历史计数、不删除用户。旧绑定值保留针对升级及新版 UI 编辑；外部 PUT 的既有完整替换语义保持，省略字段仍可置空。无需增加 DTO/interface/迁移框架。
- `.trellis/spec/backend/database-guidelines.md:45` 的单租户映射条款是旧方案。最终批准本设计后由新合同替代，在实施/验证后同步规范；不能按旧条款重新恢复复制。

## 统计口径与消费者

- 实际服务方法是 `internal/service/monitor_service.go:69` 的 `GetDashboardData`；`:104` 当前求和用户计数。
- 改用所有现存业务入站计数，含 disabled，排除内部管理 `api`（`internal/adapter/xray/compiler.go:99` 跳过该业务记录，`:118` 注入管理入站）。`internal/adapter/repository/inbound_repo.go:50` 列表不限制 Enabled，`:28` 是硬删除，因此合计不是永久服务器历史。
- `internal/delivery/http/handler_dashboard.go:20` 直接输出数据；`totalUp/totalDown` 字段名/类型保持，只修正为入站口径。
- `web/src/views/DashboardView.vue:163`、`:181` 把网卡累计和 Dashboard 总量放在同一 fallback 表达式，必须分开显示。
- Telegram 没有使用该 Dashboard 合计：`main.go:129` 直接传 HostMonitor；`internal/adapter/telegram/bot.go:797` 状态卡为网卡速率，`:392` `/traffic` 为逐用户流量。无需改 Telegram。
- 用户日流量弹窗 `web/src/views/users/components/UserTrafficModal.vue:261` 是局部历史合计，保持原生用户语义。

## 入站界面与日志导航

- `web/src/views/inbounds/components/InboundTable.vue:113` 操作区已有 click.stop；当前 7 列无累计量。追加上传/下载紧凑列，空态 colspan 同步；添加访问日志动作。
- `web/src/views/inbounds/components/InboundDetailDrawer.vue:24` 概览可展示累计上下行，`:270` 操作区可加日志入口。
- 两子组件 emit logs；`web/src/views/InboundsView.vue` 集中导航 `/logs?type=access&inbound=<actual tag>`，router 编码 tag，避免硬编码 local。
- `web/src/views/LogsView.vue:952` fetch 按本地 ref 组请求；没有读取 URL。`web/src/App.vue:283` 以 path 作为 key，同路由 query 不重新挂载，因此必须 watch query。
- 流切换、入站下拉和表格 tag 点击需同步 URL，由 query watcher 作为该上下文切换的统一加载入口，避免 mounted+watch+change 三重抓取。
- 进入入站上下文时清用户/搜索，匿名 email 为空仍可显示。保留用户手动选择后的过滤能力。
- `LogsView.vue:1095` 当前包含匹配且缺 tag 仍通过，`:1157` raw 是任意子串匹配。改为实际 tag 精确匹配，与后端 `[tag ->`/`[tag]` 同口径，避免 local-other 或 email 文本误匹配。
- 当前选中 tag 纳入选项，即使已删除/未知也不产生空白下拉与实际后台筛选不一致。
- 异步抓取以简单请求序号限定最新上下文响应；离页清理已有 timer，不增加新轮询或采集器。
- `internal/delivery/http/handler_log.go:31` 已支持 inbound；`internal/adapter/xray/log_reader.go:63` 有界读日志，`:193` 支持匿名解析。保留现有后端链路，仅在测试暴露具体问题时针对修复。

## Mock 与验证

- `web/src/mock/index.ts:55` 用用户合计，`:73` 网卡等于同值，`:108` 演示 stats 合计也需对齐当前入站口径。
- `web/src/mock/storage.ts:21` 默认入站没有上下行计数和本机入口；`:539` 历史浏览器缓存整数组覆盖默认 fixture。补模拟累计与免认证入口，在全新隔离浏览器上下文验证，不强制清除已有缓存、不以用户值兜底。
- `web/src/mock/index.ts:607` 访问样例均有 email；补匿名本机样例、相似 tag 对照；`:712` state.logs 追加须遵守同一 inbound 筛选，确保 table/raw 一致。
- 使用 `dev:demo`，避免仅 `?mock=true` 在跨页导航丢失后连接真实后端。前端当前没有测试框架，不为此次显示/导航引入框架。
- 后端 meaningful red 用例：遗留绑定不复制；user+inbound 同批只算一次；本机/禁用入站参与合计；无入站不得退回用户合计；旧绑定用户已删除也能编辑；入站读取失败返回明确错误。
