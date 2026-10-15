# 实施计划 (Implementation Plan)

## 任务目标

废除 `compiler.go` 隐式 Layer 3 追加，确立路由规则单一事实源；新建线路默认追加至尾部；在 `RoutingView.vue` 中加回高对比度、适宜尺寸的上下移动箭头按钮。

---

## 阶段一：后端编译器单一事实源改造与单测验证 (Backend)

- [x] **1.1 移除 `compiler.go` 隐式 Layer 3 自动追加**
  - 文件：`internal/adapter/xray/compiler.go`
  - 彻底移除对 `inbounds[].GetSubRoutes()` 隐式追加兜底规则的循环；
  - 确立规则模型：Layer 1 仅保留 `api -> api`，统一规则池全部由 `routing.Rules` 忠实映射；
  - 增加幂等去重保护，自动过滤连续完全相同的重复规则（清理历史回环翻倍残留）。

- [x] **1.2 编写与更新单测 `compiler_test.go`**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 适配所有单测，验证单一事实源规则编排逻辑；
  - 验证多次连续编译具有完全的幂等性，杜绝规则自动翻倍。

- [x] **1.3 执行后端质量门禁**
  - `mise x -- go test ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build .`

---

## 阶段二：前端 UI 箭头加回与高对比度重构 (Frontend)

- [x] **2.1 `RoutingView.vue` 按钮视觉与交互重构**
  - 文件：`web/src/views/RoutingView.vue`
  - 在操作区重新加入 `ArrowUp` 与 `ArrowDown` 按钮（保留 `moveRule` 函数与边界禁用状态）；
  - 按钮样式强化：`h-8 w-8`、图标 `w-4 h-4`、显式边框与明亮反差色（`bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white`），确保与背景形成强烈的视觉对比；
  - 桌面端操作列与移动端卡片操作区同步加回。

- [x] **2.2 执行前端质量门禁**
  - `cd web && mise x -- npm run typecheck`
  - `cd web && mise x -- npm run build`

---

## 阶段三：综合验证与防线验收 (Review & Finish)

- [x] **3.1 闭环验证**
  - 验证读取配置 ➔ 前端保存 ➔ 再次读取配置流程中，规则数量保持绝对一致，零复制翻倍；
  - 验证小屏与宽屏下高对比度按钮的辨识度与易用性。
- [x] **3.2 全量门禁**
  - `mise x -- go test ./...`
  - `cd web && mise x -- npm run build`
