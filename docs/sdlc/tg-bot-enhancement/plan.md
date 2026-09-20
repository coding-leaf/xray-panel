# Plan: Telegram Bot 交互体验与运维管控能力增强 - 实施计划

- **关联 Spec**: tg-bot-enhancement
- **实施执行人 / Agent**: Dev
- **当前状态**: Draft

---

## 1. 变更文件清单 (Files that change)
* `internal/adapter/telegram/bot.go` (Modify: 增加纯函数解析/渲染、CallbackQuery 处理与防连击、新命令与内联按钮)
* `internal/adapter/telegram/bot_test.go` (Modify: 增加纯函数测试、CallbackQuery Mock HTTP 测试、并发防连击与新命令测试)
* `main.go` (Modify: 为 BotHandler 注入已实例化的 UserService)

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)
> **原则**：每个步骤必须配对明确的局部验证命令，步步红绿流转，禁止跳过单步验证直接写完提交。

### Step 1: 测试先行 (Fail-repro First) 与纯函数落地 (M1)
* **操作目标**:
  - 在 `bot_test.go` 中编写针对 `ParseAddUserCommand`、`ParseCallbackData`、`RenderStatusCard`、`RenderUserCard` 的单元测试桩并先行确认红灯；
  - 在 `bot.go` 中实现上述纯函数（参数拆分校验、卡片文本格式化、InlineKeyboardMarkup 按钮构造），确保测试全绿。
* **涉及文件**: `internal/adapter/telegram/bot.go`, `internal/adapter/telegram/bot_test.go`
* **局部验证命令**: `go test -race -v ./internal/adapter/telegram -run "Test(Render|Parse)"`
* **预期判据**: 纯函数单测 100% 通过，边界用例（非法参数、超长输入、不同状态卡片）全部覆盖。

### Step 2: 接口拓展、CallbackQuery 调度与新指令实现 (M2)
* **操作目标**:
  - 在 `bot.go` 中定义 `UserServiceController` 契约，拓展 `BotHandler` 结构体字段与构造函数入参；
  - 在 `main.go` 中更新 `telegram.NewBotHandler` 调用，注入既有的 `userService`；
  - 在 `bot.go` 的轮询主循环中加入 `update.CallbackQuery` 监听与分发；
  - 实现 `handleCallbackQuery`：包含统一鉴权、`actionLock.TryLock()` 防连击保护、`AnswerCallbackQuery` 消除转圈、针对 `status:refresh` / `status:restart` / `user:toggle` / `user:reset` / `user:sub` / `user:refresh` 的业务流转与 `EditMessageText` 原地刷新；
  - 实现 `/user <email>` 与 `/adduser <email> [GB] [天数]` 指令。
* **涉及文件**: `internal/adapter/telegram/bot.go`, `main.go`
* **局部验证命令**: `go build . && go vet ./...`
* **预期判据**: 编译完全通过，静态代码检查零告警。

### Step 3: Mock HTTP 交互测试与端到端链路验证 (M3)
* **操作目标**:
  - 在 `bot_test.go` 中使用 Mock HTTP Server 模拟 Telegram BotAPI 对 `editMessageText`、`answerCallbackQuery`、`sendMessage` 的响应；
  - 编写端到端覆盖测试用例：
    1. 非管理员 CallbackQuery 与指令拦截（返回 403 并不予处理）；
    2. `/status` 发送与 `status:refresh` / `status:restart` 原地刷新回调；
    3. `/user` 查询与 `user:toggle` 启用/禁用切换、`user:reset` 流量重置；
    4. `/adduser` 参数缺省与显式赋值的新建用户流；
    5. 针对耗时操作并发连击的 TryLock 互斥拦截验证。
* **涉及文件**: `internal/adapter/telegram/bot_test.go`
* **局部验证命令**: `go test -race -v ./internal/adapter/telegram/...`
* **预期判据**: 全部测试用例通过，无任何 race 告警。

## 3. 全局质量门禁核验 (Global Quality Gate)
* **代码风格与静态检查**: `go vet ./...`
* **全量相关测试回归**: `go test -race ./...`
* **后端工程构建**: `go build .`
* **核验结果**: 所有静态检查与全量测试 100% 绿灯，构建无报错。

## 4. 实施偏差记录 (Deviations Log)
* [无偏差]

---

## 5. 阶段准出签批 (Gate 3 Sign-off)
- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **验证人 / 日期**: [待人类确认] / 2026-09-20 17:45
