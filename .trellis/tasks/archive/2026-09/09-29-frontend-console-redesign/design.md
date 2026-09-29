# 前端风格重构：技术设计方案 (Technical Design)

## 1. 架构定位与设计哲学

- **范式**：开发者基础设施控制台（Infrastructure Control Plane），融合 Vercel 极简骨架、Linear 高信息密度排版与 Railway 现代运维表达。
- **重构原则**：
  - **重核重写**：API/Mock 数据契约与核心计算属性严格复用；视图模板、CSS 与组件容器彻底掀翻；
  - **零自由发挥**：样式必须来自全局 Tailwind 语义令牌，严禁内联 Hex 色值与随意圆角；
  - **Table First + Inspector Drawer**：资源类页面统一遵循列表即数据表、点击即滑出右侧侧边抽屉。

## 2. 视觉设计系统与 Design Tokens 契约

在 `web/tailwind.config.js` 与 `web/src/style.css` 中定义基于 CSS 变量的语义令牌系统：

```css
:root {
  --background: 0 0% 4%;         /* #0A0A0A */
  --foreground: 0 0% 98%;        /* #FAFAFA */
  --card: 0 0% 7%;              /* #121212 */
  --card-foreground: 0 0% 98%;
  --popover: 0 0% 7%;
  --popover-foreground: 0 0% 98%;
  --muted: 0 0% 12%;            /* #1F1F1F */
  --muted-foreground: 0 0% 64%; /* #A3A3A3 */
  --border: 0 0% 15%;           /* #262626 */
  --input: 0 0% 15%;
  --accent: 0 0% 15%;
  --accent-foreground: 0 0% 98%;
  --destructive: 0 62.8% 30.6%;
  --destructive-foreground: 0 0% 98%;
  --ring: 0 0% 30%;
  --radius: 0.375rem;           /* 6px 基础圆角 */
}
```

- **圆角约束**：
  - Controls (Button, Input, Badge): `rounded-md` (`6px`)
  - Panels & Cards: `rounded-lg` (`8px`)
  - Modals & Drawers: `rounded-xl` (`10px` / `12px`)
- **投影与边框约束**：
  - 静态卡片与表格：`border border-border`，禁止全局悬浮阴影；
  - 浮层 (Dropdown / Drawer / Modal): `shadow-2xl shadow-black/80` + `border border-border`。

## 3. UI 基础原子组件规范 (`web/src/components/ui/`)

采用轻量、强类型、零重量第三方黑盒的自研封装（遵循 shadcn 设计思想）：

1. **`Button.vue`**:
   - Variants: `default` (高对比度前景色), `secondary` (中性暗色背景), `outline` (带边框透底), `ghost` (悬停变暗), `destructive` (红色危险操作);
   - Sizes: `sm` (紧凑高度 32px), `default` (36px), `icon` (32x32 方形);
2. **`Input.vue`**:
   - 6px 圆角，高度 32px/36px，等宽/中性字体，focus-visible:ring-1 聚焦边框。
3. **`Badge.vue`**:
   - 扁平微边框，极小字体 (11px)，通过前置微指示灯圆点 (如 `bg-emerald-500`) 表达状态。
4. **`Table.vue` & 子组件 (`TableHeader`, `TableRow`, `TableCell`, `TableHead`)**:
   - 彻底取代旧卡片列表。紧凑表头 (border-b, tracking-wider, text-muted-foreground), 数据行 hover 高亮 (`hover:bg-muted/50`)。
5. **`Drawer.vue` (Inspector Sheet)**:
   - 专门用于承载资源详情与快速编辑。
   - 固定右侧定位 (`right-0 top-0 bottom-0`)，可配置宽度 (`w-[480px]` 或 `w-[560px]`)；
   - 包含 Header、Body (带细滚动条)、Footer (操作按钮栏)；
   - 包含背景蒙层并支持 ESC 退出与外部点击关闭。

## 4. App Shell 架构重塑 (`App.vue`)

- **左侧导航栏 (`Sidebar`)**:
  - 宽度紧凑至 `220px`；
  - 顶部极简 Logo 区域：无花哨彩虹渐变，极简 Mono 字符与单色纯净图标；
  - 扁平化分组导航：
    - `OVERVIEW`: 仪表盘
    - `RESOURCES`: Inbounds (入站), Outbounds (出站), Routes (路由), Users (用户)
    - `OPERATIONS`: Logs (日志), Traffic/Topology (拓扑)
    - `SYSTEM`: DNS, Config, Settings
  - 激活态：`bg-muted text-foreground` + `border border-border/50`，放弃大面积发光条。
- **顶栏 (`TopBar`)**:
  - 左侧：当前路径面包屑导航 (如 `Resources / Inbounds`);
  - 右侧：Xray 核心微型运行状态灯、快捷刷新按钮、用户信息与退出。
- **主内容区 (`Main Content Area`)**:
  - 标准内边距与最大宽度控制；
  - 提供 Legacy 页面包裹层 (`class="legacy-container"`), 确保现有页面无缝过渡。

## 5. Inbounds 标杆示范页重写架构 (`InboundsView.vue`)

- **数据流与接口对齐**：
  - 继续调用原有 `api.get('/inbounds')` 与 `api.getRealityStatus()` / `api.generateRealityKey()`；
  - 状态管理：`inboundsList`, `realityStatusMap`, `selectedInbound` (控制抽屉激活)；
- **Table-First 呈现**：
  - Columns:
    1. 标识 (`Tag` + 备注名称)
    2. 协议 (`Protocol`，如 VLESS, Trojan, SS，文字徽标)
    3. 端口与网络 (`Port` / `Network`，如 `443 / tcp`)
    4. 传输安全 (`Security`，如 `Reality`, `TLS`, `none`)
    5. 用户数与绑定路由 (`Users Count` / `SubRoutes`)
    6. 状态 (`● Running` / `○ Inactive`)
    7. 操作 (查看详情、快速开关、删除)
- **Inspector Drawer (右侧滑出抽屉)**：
  - 点击表格任意行，激活抽屉展示该 Inbound 完整概览；
  - 分区 Tab 或分组折叠：
    - **General**: 端口、协议、监听地址、Tag；
    - **StreamSettings & Reality**: 真实域名 (SNI)、公私钥对、ShortId、一键测试 Reality 连通性；
    - **SubRoutes**: 查看并管理分配到此 Inbound 的子路由；
    - **Actions**: 一键复制客户端节点分享链接/二维码、修改配置、危险操作 (删除)。

## 6. 回滚与兼容策略

- 采用单一文件替换与 Git 严格追踪：
  - 若重构后的 Inbounds 页面或 App Shell 出现不可预见的问题，可通过 Git 独立回滚特定视图；
  - 旧有 API 客户端与 Mock 数据体系完全保持零变更，确保前后端契约 100% 兼容。
