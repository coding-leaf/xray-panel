# Intent: Telegram Bot 交互体验与运维管控能力增强

- **任务编号**: tg-bot-enhancement
- **提出人**: Dev
- **创建时间**: 2026-09-20 17:29
- **初始 Change Tier**: Tier 2
- **当前状态**: Draft

---

## 1. 问题与现状背景 (Problem)
当前 Telegram Bot (`internal/adapter/telegram`) 的交互和功能较为局限：
1. **刷屏与缺乏内联交互**：仅支持传统文字指令和底部 ReplyKeyboard，每次发送 `/status` 或操作都会产生新消息，不支持 Telegram InlineKeyboardMarkup 内联按钮与 CallbackQuery 原地编辑消息 (EditMessageText)；
2. **缺乏单用户深度运维**：虽然有 `/traffic` 查看全员概览，但无法查看指定用户的详细信息（配额、已用、到期时间、状态），且无法在 TG 中快捷执行封禁/解禁或流量重置；
3. **无法快捷开号**：为新设备或朋友开号必须登录 Web 后台，无法在 Telegram 中通过指令快速创建用户并获取专属订阅链接。

## 2. 变更性质分类 (Change Archetype - 单选)
- [ ] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [x] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. **内联交互与原地刷新**：
   - `/status` 消息附带内联按钮（`[🔄 刷新]`、`[⚡ 重启核心]`），点击通过 CallbackQuery 原地更新内容与时间戳，彻底消除多轮刷屏；
   - 处理完 CallbackQuery 时调用 `AnswerCallbackQuery`，避免客户端加载转圈。
2. **用户详情与快捷管控**：
   - 支持 `/user <email>` 查看指定用户的配额、已用流量、到期时间、启用状态；
   - 卡片下方附带内联操作按钮：`[⛔ 禁用]` / `[✅ 启用]`、`[🔄 重置流量]`、`[🔗 获取订阅]`，点击后原地执行并更新卡片状态。
3. **快捷开号命令**：
   - 支持 `/adduser <email> [GB] [天数]`，快速生成新用户、初始化流量限额与到期时间，并在 TG 中直接返回带有专属订阅链接的结果卡片。

## 4. 波及工程分面 (Affected Architectural Layers)
- [ ] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [x] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**:
  - 权限安全：所有内联回调与新指令必须经过严格的 `adminChatID` 鉴权校验；
  - 并发与响应：CallbackQuery 必须始终响应 `AnswerCallbackQuery`，长耗时操作需防重复点击并发竞态；
  - 架构整洁：遵守现有 `internal/adapter/telegram` 分层，复用已有 Domain 接口或必要的 Service 编排，严禁侵入核心业务领域模型。
* **明确非目标 (Non-Goals / Out-of-Scope)**:
  - 不修改现有 Web 前端与 HTTP REST API 契约；
  - 不修改底层 SQLite 数据库 Schema；
  - 本阶段不引入外部第三方大依赖（如二维码图片生成库等待后续按需独立规划），保持 KISS。
* **完成判定条件 (Definition of Done)**:
  - `/status` 命令带有内联刷新与重启按钮，点击原地编辑刷新；
  - `/user <email>` 命令可显示用户详情，并支持内联按钮禁用/启用与重置流量；
  - `/adduser <email> [GB] [天数]` 可快速新建用户并返回订阅卡片；
  - 自动化测试用例覆盖新增指令与 CallbackQuery 分支，`go test -race ./...` 与 `go vet ./...` 100% 通过。

## 6. 未决疑问与待探讨点 (Open Questions)
- `/adduser` 默认参数处理：若用户只输入 `/adduser test@example.com`，默认配额建议为 0 (不限) 还是例如 100GB？默认有效期建议是不限期还是 30 天？建议设为不限（或合理默认值），支持显式指定。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-20 17:29
