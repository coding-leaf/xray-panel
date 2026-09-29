# Design: 登录与 Portal 控制台收敛

- 采用 Neutral Design Tokens (`bg-background`, `bg-card`, `border-border`, `text-foreground`)；
- 复用 `Button.vue`, `Input.vue`, `Badge.vue`；
- 保持原登录状态机与凭据兑换 API 契约完全一致。
