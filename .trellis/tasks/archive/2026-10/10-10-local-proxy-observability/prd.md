# 本机代理流量与访问记录可观测性优化

## Goal

服主能够直接查看 VPS 本机代理入口的累计上传、下载及访问目标；保留 `local` 的回环监听和免认证接入方式，移除额外的用户流量归属概念。

## Background

- 用户于 2026-10-10 许可创建任务并进入规划；本轮许可不是最终实施审批。
- VPS 轻量只读检查确认 `local` 为回环监听 SOCKS 入站，免认证，用户/入站统计均开启，存在原生入站计数器。SSH 账号不能读取数据库和访问日志正文，非交互 sudo 不可用，实际落盘记录尚未核实。
- 前序归档任务为 `.trellis/tasks/archive/2026-10/10-09-inbound-traffic-user-binding/`；本次根据用户的观测需求重新收敛产品边界。
- 入站累计与按入站过滤访问日志已有实现，应复用现有采集与日志读取链路。

## Requirements

### R1. 取消人工用户归属

移除前端入站编辑/详情/列表中的流量归属配置与展示，停止把入站总量复制到用户用量、日流量账单和瞬时用户速率。原生用户统计继续独立核算。缺陷证据：`internal/delivery/cron/traffic_sync.go:164`、`:195` 同时累加两种统计；`web/src/views/inbounds/components/InboundFormDrawer.vue:637` 无协议条件地展示绑定项。

### R2. 展示入口用量与访问记录

复用入站累计上传/下载字段；在入站列表与详情提供直接查看该入站访问日志的入口。跳转后自动按实际入站 tag 精确筛选，用户筛选恢复全部用户，确保匿名请求可见。刷新、同页 query 变化和浏览器前进后退均保持 URL 所表达的筛选。采用通用入站行为，不硬编码 `local` 或新增重复页面。证据：`web/src/views/inbounds/components/InboundTable.vue:113`；`web/src/views/inbounds/components/InboundDetailDrawer.vue:24`、`:270`；`web/src/views/LogsView.vue:952`、`:1091`、`:1157`；`internal/delivery/http/handler_log.go:31` 已支持入站参数。

### R3. 统一 Xray 用量口径

Xray 代理累计用量包含本机免认证入口，采用所有现存业务入站（含禁用入站）的累计口径，不能把用户总量与入站总量相加。主机网卡总量和 Xray 入站累计明确区分；删除入站后其累计不再进入该合计，不宣称永久服务器历史总量。证据：`internal/service/monitor_service.go:104` 当前汇总用户；`internal/adapter/repository/inbound_repo.go:28`、`:50` 为硬删除与完整列表；`web/src/views/DashboardView.vue:163`、`:181` 混用网卡/用户累计。

### R4. 保留历史与兼容

保留既有用户、入站累计及每日用户账单，不删除用户、不重算或回填历史数据，不执行破坏性数据库迁移。升级及新版 UI 编辑保留旧绑定值，字段不继续影响核算或阻碍入站编辑；外部 PUT 保持现有完整替换语义，省略字段仍可置空。证据：`internal/domain/inbound.go:22` 已发布 GORM/JSON 字段；`internal/service/config_service.go:347`、`:371`、`:383` 有旧绑定专用用户存在性校验；`internal/adapter/repository/inbound_repo.go:25` 为完整 Save。

### R5. 验证记录链路

先用失败测试复现绑定入站/原生用户双加与免认证入口缺失总量，再修复；核对入站用量落盘、日志过滤和页面跳转。入站累计读取失败应明确返回错误，不能显示为有效零值。日志读取失败应明确提示，不能与无访问记录混淆。访问记录范围为日志已有的时间、目标地址/端口、入站/出站及结果，不承诺 HTTPS 网页正文或完整页面路径。证据：`internal/delivery/cron/traffic_sync_test.go:278`、`internal/service/monitor_service_test.go:101`；`internal/adapter/xray/log_reader.go:63` 有界读日志，`:193` 解析匿名记录。

## Acceptance Criteria

- AC1 / R1：所有协议表单均不再显示额外的流量归属项；只出现入站统计时，用户用量、用户日账单和用户速率不会因此增长。
- AC2 / R1：同轮原生用户与对应入站均报告流量时，用户只按原生用户增量计入一次，入站累计正常增长。
- AC3 / R2：可从入站列表/详情直接打开该入口访问日志，默认包含匿名请求；`local` 不混入 `local-other`；刷新/同页 query 切换/前进后退保持一致筛选，旧异步响应不会覆盖当前入口结果。
- AC4 / R3：无认证本机入口和禁用入站的历史计入现存入站累计；没有业务入站时该合计为零；主机网卡累计与入站累计文案和数值来源一致。
- AC5 / R4：升级不删除用户、累计字节或用户每日历史；新版 UI 对有旧绑定值的入站编辑往返保持该值，旧绑定不再产生新核算，也不因旧用户已删除阻碍编辑；外部 PUT 兼容现有完整替换语义。
- AC6 / R5：对应后端失败用例先红后绿；`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`web` 下 `mise x -- npm run typecheck` 和 `mise x -- npm run build` 均实际通过；页面关键流程通过浏览器验证。
- AC7 / R5：入站列表读取或日志读取失败时用户得到明确错误；mock 演示具备有用量的免认证入口和匿名访问记录，用户、入站、网卡累计刻意不同以验证来源。

## Out of Scope

- 新增 SOCKS/HTTP 用户认证、配额、计费、账号热更新或重启策略。
- 新增入站每日历史表、历史回填、流量纠错或清理现有虚拟用户。
- 抓包、内核改造、新采集守护进程或重型 VPS 排查命令。
- 现网部署、重启服务、修改 VPS 配置；现网进一步排查仍限轻量只读。
