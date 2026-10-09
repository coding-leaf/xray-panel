# 修复 UsersView 后台轮询防空转缺陷

## Goal

修复 `UsersView.vue` 在标签页切入后台 (`document.hidden === true`) 时未清除速度轮询定时器的缺陷，严格遵循前端规范中关于 `setInterval` 与 `visibilitychange` 成对清理的要求，防止无意义网络轮询持续向后端发送 `/api/users/speeds` 请求消耗 VPS 资源。

## Requirements

1. **生命周期对齐与成对清理**：
   - 在 `web/src/views/UsersView.vue` 中，当页面切入后台 (`document.hidden === true`) 时，必须主动 `clearInterval(speedTimer)` 并将 `speedTimer` 置为 `null`，停止无意义的网络请求；
   - 当用户重新切回当前标签页 (`document.hidden === false`) 时，立即主动调用一次 `fetchSpeeds()` 刷新数据，若定时器不存在则重新激活 `speedTimer = setInterval(fetchSpeeds, 3000)`；
   - 保持组件在 `onUnmounted` 时清理定时器和移除事件监听的完整逻辑。
2. **代码风格与规范一致性**：
   - 逻辑结构与 `App.vue` 及 `DashboardView.vue` 保持完全一致的防空转范式；
   - 满足 `.trellis/spec/frontend/components-and-routing.md` 中的规范约束。

## Acceptance Criteria

- [x] `web/src/views/UsersView.vue` 的 `handleVisibilityChange` 实现了标签页隐藏时销毁定时器、激活时恢复定时器与首触刷新的完整逻辑。
- [x] 前端生产构建门禁通过：`cd web && mise x -- npm run build` 零警告、零类型错误通过。
- [x] 后端测试与静态检查不受影响：`mise x -- go test ./...` 与 `mise x -- go vet ./...` 全绿。
