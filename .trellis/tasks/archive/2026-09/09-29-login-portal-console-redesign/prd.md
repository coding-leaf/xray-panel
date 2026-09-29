# PRD: 登录与 Portal 网关控制台风格收敛

## 1. 目标
将 `LoginView.vue` 与 `PortalClaimView.vue` 从黑客风毛玻璃发光升级为 Vercel 极简基础设施控制台风格，同时清除全站剩余毛玻璃细节。

## 2. 改造范围
- `LoginView.vue`：移除背景 glow、渐变边框和 backdrop-blur，改用中性高对比卡片与原子 Input/Button；
- `PortalClaimView.vue`：6 位兑换码提取页收敛为极简控制台网关形态，清除所有 glow 与毛玻璃；
- 全局细节：清除 `ToastContainer.vue` 与 `TopologyView.vue` 残留的渐变与 blur。

## 3. 验收标准
- 0 遗留 `backdrop-blur`、0 渐变光晕；
- `npm run build` 与 `go test ./...` 门禁全绿；
- 保持所有登录鉴权、TOTP 与凭据兑换功能 100% 完整。
