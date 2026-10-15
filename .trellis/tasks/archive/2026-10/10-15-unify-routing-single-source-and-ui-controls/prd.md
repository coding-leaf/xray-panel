# 消灭隐式路由两层皮与单一事实源重构

## Goal

彻底消除 `compiler.go` 底层背着用户隐式追加 Layer 3 规则的技术债，确立“除 `api` 外 100% 用户自主编排、单一事实源（Single Source of Truth）”架构；新入站分流线路默认显式追加至规则池尾部；在 `RoutingView.vue` 中加回尺寸适宜、高对比度、清晰防误触的上下移动箭头按钮，与 `⋮⋮` 拖拽把手及序号直跳形成完整的立体排位交互体验。

## Requirements

1. **废除编译器隐式 Layer 3 追加（Abolish Implicit Layer 3）**
   - 移除 `internal/adapter/xray/compiler.go` 中遍历 `inbounds` 自动隐式追加 SubRoute 兜底规则的循环；
   - 规则层级极简化：
     - Layer 1：仅保留系统核心内部通信必需的 `inboundTag: ["api"] -> outboundTag: "api"`；
     - Layer 2：用户在面板配置与可见的全部路由规则（按用户编排顺序 100% 忠实编译）；
   - 彻底消灭底层背着用户硬塞规则的“阴阳两层皮”现象，从根本上切断“读出 ➔ 保存 ➔ 翻倍”的死循环复制。

2. **新建线路默认追加至尾部（New Routes Appended to Tail）**
   - 入站分流线路（SubRoutes）的路由规则全部显式归入用户的统一规则池，由用户在「路由管理」中统一管控；
   - 新增入站或新建线路时，默认将其显式追加到规则池的最末端（尾部），用户拥有完整的可见性与拖拽重排权。
   - 编译器/服务层对现有规则池中的完全相同规则进行幂等去重保护，防止历史脏数据残留。

3. **前端交互与高对比度按钮优化（High-Contrast Arrow Buttons & Layout）**
   - 在 `web/src/views/RoutingView.vue` 操作栏中加回“上移”与“下移”箭头按钮（`ArrowUp` 与 `ArrowDown`）；
   - **尺寸与触控标准**：按钮尺寸不宜过小（采用 `h-7.5 w-7.5` 或 `h-8 w-8`，图标 `w-4 h-4`），具有良好的点按靶心；
   - **视觉对比度拉开**：严禁昏暗无辨识度，采用高对比度显色（如明亮中性色/轻微底色边框 `bg-neutral-800/80 border border-neutral-700 text-neutral-200 hover:text-white hover:bg-neutral-700`），与深色卡片背景拉开清晰反差；
   - 形成“小距离点箭头、中距离拖拽把手、长距离序号输入直跳”的三位一体交互组合；移动端卡片与桌面端表格均保持良好的响应式适配。

## Acceptance Criteria

- [x] `compiler.go` 彻底移除隐式追加 Layer 3 规则逻辑，除 `api` 外不再背着用户塞任何幽灵规则。
- [x] 路由规则列表成为数据面与控制面的唯一事实源，多次保存配置绝不会产生任何规则重复翻倍。
- [x] 后端单元测试覆盖单一事实源编译与去重逻辑，`mise x -- go test ./...` 100% 通过。
- [x] 前端 `RoutingView.vue` 加回上移与下移箭头，按钮尺寸适宜、与背景对比度鲜明、禁用状态明确。
- [x] 桌面端与移动端卡片均支持箭头、把手拖拽与序号直跳，前端构建 `cd web && mise x -- npm run build` 零报错。
