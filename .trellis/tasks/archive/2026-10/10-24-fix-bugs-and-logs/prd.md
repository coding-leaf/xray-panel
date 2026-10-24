# 修复移动端与节点转化异常及扩展日志能力

## Goal
修复系统在多平台运行时的核心缺陷（用户与旧入站解除绑定时的 RPC 报错回滚失败、虚拟节点 SubRoute 映射不清、移动端适配体验缺陷），并扩展全栈日志能力（按出口过滤日志与 Xray/面板日志等级配置与过滤）。

## Task Breakdown (子任务拆解)

1. **Child 1: 修复用户保存异常与虚拟节点转化 (`10-24-fix-user-inbound-and-virtual-nodes`)** - [已交付/归档]
   - 修复 `user_service.go` 中从旧 Inbound 移除用户时，若 Xray 报 `handler not found` 导致的级联回滚与保存失败错误。
   - 规整优化 `node_converter.go` 中单端口多出口（SubRoute / 虚拟节点）的转换、展示、订阅链接生成逻辑，消除语义歧义。
2. **Child 2: 新增出站日志过滤与日志等级全栈能力 (`10-24-feat-log-outbound-and-level`)** - [已交付/归档]
   - 后端扩展日志过滤契约（支持 `outbound` 出口 Tag 与 `level` 日志级别过滤）。
   - 系统设置中支持 Xray 核心运行日志级别（`debug`/`info`/`warning`/`error`/`none`）配置并动态落盘编译生效。
   - 前端 LogsView 访问日志增加出口筛选下拉菜单与级别快速筛选。
3. **Child 3: 移动端响应式布局与交互体验全量适配 (`10-24-fix-mobile-responsive-adaptation`)** - [已交付/归档]
   - 修复移动端各视图的响应式适配（抽屉遮挡、表格横向溢出、弹窗溢出、底部导航栏安全距离等）。

## Parent Acceptance Criteria (跨子任务综合验收标准)

- [x] 后端测试全部通过 (`mise x -- go test ./...`) 且静态分析无 warning (`mise x -- go vet ./...`)。
- [x] 后端完整构建通过 (`mise x -- go build .`)。
- [x] 前端生产构建与类型检查通过 (`cd web && mise x -- npm run build`)。
- [x] 3 个子任务全部测试验收通过并交付。
