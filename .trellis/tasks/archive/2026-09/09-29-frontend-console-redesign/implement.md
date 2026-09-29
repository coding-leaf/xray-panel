# 前端风格重构实施计划 (Implementation Plan)

## Milestone 1: 基础设施控制台底座与 Inbounds 标杆示范

### Step 1: Design Tokens 与 Tailwind 主题配置
- [x] 备份并改造 `web/tailwind.config.js`，引入 Neutral 系列 CSS 语义变量 (`background`, `foreground`, `card`, `border`, `muted`, `accent` 等)；
- [x] 改造 `web/src/style.css`，注入全局变量、消除遗留的发光与毛玻璃类，统一滚动条与字体规范。

### Step 2: 建立轻量原子组件体系 (`web/src/components/ui/`)
- [x] 实现 `Button.vue` (带 variant / size / loading / disabled 属性及 TypeScript 定义)；
- [x] 实现 `Input.vue` (带类型安全、紧凑高度与 focus-ring)；
- [x] 实现 `Badge.vue` (带 status 指示灯圆点、variant 语义)；
- [x] 实现 `Table.vue`, `TableHeader.vue`, `TableRow.vue`, `TableCell.vue`, `TableHead.vue`；
- [x] 实现 `Drawer.vue` (基于 Teleport 的右侧 Inspector 检查器抽屉，带平滑过渡与遮罩)。

### Step 3: 重写 App Shell (`web/src/App.vue`)
- [x] 移除旧版大侧边栏与发光装饰；
- [x] 实现紧凑的 Vercel-style 侧边栏（分类导航：Overview, Resources, Operations, System）；
- [x] 实现标准极简面包屑与状态顶栏（包含 Xray 状态徽标与快捷按钮）；
- [x] 验证未重构的 Legacy 页面在新壳下的挂载与显示兼容性。

### Step 4: 重构 Inbounds 标杆示范页 (`web/src/views/InboundsView.vue`)
- [x] 备份原业务逻辑与状态映射；
- [x] 基于 Table-First 重写主表格与顶部操作栏（搜索、协议过滤、新建入站）；
- [x] 基于 `Drawer.vue` 实现右侧 Inspector 抽屉：
  - 节点详情概览；
  - Reality 配置展示与一键测速/检查；
  - 快捷操作（编辑、复制、启停、删除）；
- [x] 对接入站新建/编辑表单，支持结构化折叠表单（告别 30 字段表单墙）。

### Step 5: 质量门禁与端到端验证
- [x] 运行前端类型检查与构建：`cd web && npm run build`（验证 TS 类型全绿，打包零报错）；
- [x] 运行后端单元测试门禁：`mise x -- go test ./...`；
- [x] 浏览器全功能验证与交互测试。
