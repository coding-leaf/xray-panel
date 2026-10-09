# 实施与验证计划

## 前置门禁

- [ ] 用户在收到最终规划摘要后明确批准实施；此前不运行 task.py start、不改产品代码。
- [ ] PRD/设计无阻塞项，implement.jsonl/check.jsonl 已包含真实 spec/research。
- [ ] 通过 get_context.py 加载 Phase 2.1，委派 trellis-implement；角色提示以实际 Active task 路径开头，使用原生上下文注入，缺失时由子会话补拉。

## Milestone 1：核算与兼容

- [ ] 在 traffic_sync_test.go 先写遗留绑定不复制及原生用户+入站同轮不双加用例，并实际观察红灯。覆盖入站增量和用户速率/账单触发边界，不削弱断言。
- [ ] 删除同步映射及复制段，保留原有原子批量仓储/回退/最终刷盘；旧生产样例在触及测试中改为保留域名/回环数据。
- [ ] config_service_test.go 先复现陈旧绑定用户导致编辑失败；取消专用校验，保留旧字段读写，domain 注释说明退役语义。
- [ ] 验证升级及新版 UI 编辑旧绑定值往返保持，外部 PUT 省略字段沿用既有完整替换语义；受保护的累计字段不会被资料更新覆盖。
- [ ] monitor_service_test.go 先复现 total 漏本机入口。测试用户与入站累计不相等、本机+普通+disabled、无入站不回退用户、管理 api 不计入以及入站查询错误。
- [ ] Dashboard 合计改为现存业务入站累计；明确错误传播，保持缓存/字段类型和用户人数语义。
- [ ] 运行针对性后端测试，从红灯修到绿灯；确认必要的真实 SQLite 落盘回归存在，缺失则补有意义的集成用例。

## Milestone 2：入口用量、日志上下文与演示

- [ ] 前端保留旧字段隐式兼容透传，删除所有可见流量归属控件/徽标/详情；不增加新身份机制。
- [ ] InboundTable/Detail 添加累计上下行和日志动作，由父视图统一导航。
- [ ] LogsView 建立 type/inbound query 同步与唯一上下文加载入口，支持刷新、同页导航、前进后退，入站导航恢复全部用户及清空搜索。
- [ ] 日志精确 tag 过滤；未知 tag 在下拉显示；处理快速切换的旧响应和离页 timer，读取错误明确提示。
- [ ] 核对已有有界日志读取测试，缺失时补合成匿名 TCP/UDP、accepted/rejected、单 tag 及相似 tag 隔离用例，验证真实日志读取/解析链路，不只依赖 mock。
- [ ] Dashboard 网卡累计与入站累计分开展示，包含真实零值，不混用 fallback。
- [ ] Mock 补有用量的免认证入站、匿名访问目标、相似 tag；修正 Dashboard/mock stats 与日志 raw 过滤，使用新隔离浏览器上下文。
- [ ] 在 demo 浏览器验证：列表/详情累计一致；两个日志入口；匿名表格/raw；local 与相似 tag 隔离；用户手动过滤与重新入口跳转恢复；刷新/同页 query/前进后退；慢响应快速切换；access/error/audit；读取错误；窄屏表格。

## Milestone 3：全量门禁与收尾

- [ ] 委派 trellis-check 对照 AC1–AC7、兼容字段和旧 spec 冲突，自修真实偏差。
- [ ] 实际运行 `mise x -- go test ./...`、`mise x -- go vet ./...`。
- [ ] web 工作目录运行 `mise x -- npm run typecheck`、`mise x -- npm run build`（build 仅 Vite，不能替代 typecheck）；随后根目录运行 `mise x -- go build .`，确保嵌入新产物。
- [ ] 如新增/修改并发共享状态，按本机工具链可用性运行 `CGO_ENABLED=1 mise x -- go test -race ./...` 并如实记录结果。
- [ ] 更新旧单租户映射 spec 为原生用户/入站各自核算合同，补 Dashboard 合计与日志导航约定；按需使用 trellis-break-loop 记录该类概念混用根因。
- [ ] review diff 与 git 状态；按工作流提交、trellis-finish-work 归档/日志。VPS 部署与服务重启不在此次实施许可范围内。

## 风险文件与回滚点

- traffic_sync.go 是数据核算边界，测试必须先红后绿；不能改为吞掉增量或弱化断言。
- 保留 BoundUserEmail 列/JSON 结构及旧值，避免 Save 省略字段覆盖；禁止删除历史行。
- monitor_service.go 仅修正累计来源，不增加 RPC 或高频查询；返回入站读取错误不能入缓存为零快照。
- LogsView.vue 不作整页重构，新增路由监听必须防重复首轮抓取与过期响应。
- 不执行现网修改；仓库回滚不涉及 Schema 回滚，回退旧同步代码可能恢复人工归属与双计，必须明确该影响。
