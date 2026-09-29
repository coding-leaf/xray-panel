# 前端风格重构：基础设施控制台与标杆示范 (Milestone 1)

## Goal

彻底重塑前端视觉与交互范式：确立“开发者基础设施控制台 (Infrastructure Control Plane)”美学，建立严格无自由发挥的 Neutral 语义设计令牌与原子组件；打造 Vercel 风格的 App Shell 骨架；以 Inbounds 节点管理为标杆落地 Linear 密度 Table-First + 右侧 Inspector Drawer，并在新架构中严格隔离兼容 Legacy 未重构页面。

## Requirements

1. **设计系统与刚性红线 (Design Tokens & Strict Constraints)**:
   - 全面封杀自由色值：全站严禁散落 Hex 色值，严禁毛玻璃 `glass-card`、`backdrop-blur` 和发光渐变。
   - 建立 Neutral 语义灰度变量体系：背景 (`#0A0A0A`)、卡片/面板 (`#121212`)、边框 (`#262626` / `#1F1F1F`)、文字前景色 (`#FAFAFA` / `#A3A3A3` / `#737373`)。
   - 严格圆角与边框规范：Button/Input/Select 锁定 `6px` (`rounded-md`)，Panel/Container 锁定 `8px` (`rounded-lg`)，Modal/Drawer 锁定 `10px` (`rounded-xl`)。统一 `1px solid` 微边框，全站禁止卡片悬浮大投影。
   - 语义功能色严格收敛：仅允许绿 (正常/运行中)、红 (异常/停止/危险操作)、黄 (警告/临期)、蓝/紫 (当前激活态/主要操作)，严禁协议彩虹色。

2. **原子与基础组件库 (UI Primitives)**:
   - 在 `web/src/components/ui/` 下建立纯粹、强类型且符合设计令牌的原子规范组件：
     - `Button` (default, secondary, destructive, outline, ghost)
     - `Input` / `Select` (紧凑表单控件，锁定 6px 圆角与 focus 环)
     - `Badge` (语义状态徽章，靠文字与状态微指示灯区隔，而非鲜艳背景)
     - `Table` / `TableHead` / `TableRow` / `TableCell` (高密度、平铺边框)
     - `Drawer` (Sheet) (平滑滑出的右侧 Inspector 检查器容器，支持遮罩、关闭动画与焦点管理)

3. **基础设施控制台外壳 (App Shell)**:
   - 重构 `App.vue` 与顶栏/侧边栏：
     - 固定左侧极窄/紧凑导航：Xray 标识 + 资源分类 (Overview / Resources: Inbounds, Outbounds, Routes, Users / Operations: Logs, Traffic / System: Settings)。
     - 极简面包屑与状态顶栏：仅保留当前资源路径、Xray 运行状态核心徽标、全局快捷操作与管理员信息，彻底移除每个页面顶部的“欢迎巨幅卡片”。
     - 移动端适配：抽屉式菜单保留但同步更新至 Neutral 现代极简风格。

4. **标杆示范页重构 (Inbounds View & Inspector Drawer)**:
   - 彻底推翻旧版 `InboundsView.vue` 的模板与 CSS，重写为 Table-First 模式：
     - 顶栏：搜索、协议过滤下拉、添加入站按钮。
     - 资源列表表格：Tag/名称、协议 Badge、监听端口、绑定的用户数、流量统计、运行状态指示灯、快捷操作。
     - 交互核心——右侧 Inspector Drawer：点击任意一行不是跳转全屏表单，而是在右侧滑出检查器抽屉，展示该 Inbound 的基础配置、Reality 探测状态/公私钥、绑定的 SubRoute 分流规则与用户授权，并提供就地编辑与开关能力。
     - 完整保留并无缝对接原有的后端 API 与 Mock 行为（包括 Reality 密钥生成、探测触发等）。

5. **旧页面隔离与平滑过渡 (Legacy Compatibility)**:
   - 其他 11 个旧页面 (Users, Routing, DNS, Config, Settings 等) 挂载在新 App Shell 的主体内容容器内。
   - 统一提供 Legacy 样式隔离垫片，保证在整体暗黑极简背景下不破损、功能全量可用。

## Acceptance Criteria

- [x] **视觉纯粹性**：全站无任何彩色大面积渐变背景与发光光晕；App Shell 与 Inbounds 页面完全基于统一 Design Tokens 实现。
- [x] **组件与交互规范**：
  - [x] 基础组件 (`Button`, `Input`, `Badge`, `Table`, `Drawer`) 在 `web/src/components/ui` 就位并正确类型化；
  - [x] Inbounds 页面以表格形式紧凑展示所有节点，点击节点能稳定弹出右侧 Inspector Drawer；
  - [x] Inbound 的新建、编辑、删除、Reality 状态检查等核心逻辑完整正常工作，零业务逻辑丢失。
- [x] **Legacy 兼容性**：点击导航切换到 Users、Settings、Config 等未重构页面时，界面正常加载，无白屏、无阻塞性报错。
- [x] **门禁验证**：
  - [x] 前端类型与构建通过：`cd web && npm run build`（包括 `vue-tsc` 与 `vite build`）全绿零错误；
  - [x] 后端测试保持全绿：`mise x -- go test ./...`。
