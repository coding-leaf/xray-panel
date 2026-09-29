# Design — Spec 文档整理（00-bootstrap-guidelines）

> 技术方案：仅涉及 `.trellis/spec/` 文档结构与内容来源，不改动运行时代码。

---

## 1. 边界 (Boundaries)

| 允许 | 禁止 |
|------|------|
| 新增/改写 `.trellis/spec/**.md` 文档 | 修改 `internal/**` 或 `web/**` 源码 |
| 新增/改写任务工件 (`.trellis/tasks/00-bootstrap-guidelines/*`) | 改公共 API、DB Schema、持久化格式 |
| 更新 `frontend/index.md` 中与代码矛盾的既有描述 | 修改 `spec/guides/*` 通用思考指南 |

---

## 2. 目标文档结构

沿用 backend 的「index + 分层子文档」形态，前端补齐为：

```
.trellis/spec/
├── backend/
│   ├── index.md                  # 既有，校正
│   ├── directory-structure.md    # 既有，校正
│   ├── database-guidelines.md    # 既有，校正
│   ├── error-handling.md         # 既有，校正
│   ├── quality-guidelines.md     # 既有，校正
│   └── logging-guidelines.md     # 补全（当前纯占位符）
└── frontend/
    ├── index.md                  # 索引 + 技术栈；删除 "Pinia 单例 Store" 描述
    ├── directory-structure.md    # 新增
    ├── api-and-dataflow.md       # 新增
    ├── components-and-routing.md # 新增
    └── quality-guidelines.md     # 新增
```

---

## 3. 内容来源与证据锚点

文档只写代码事实，每条结论挂 `file:line`。核心证据（来自只读调研）：

### Backend logging
- `internal/pkg/logger/logger.go:18-45` — `Init(level, isJSON)`，`slog.SetDefault`；`:31-34` `AddSource: true`；`:36-41` JSON/Text handler 写 `os.Stdout`
- `internal/pkg/logger/logger.go:47-56` — `FromContext(ctx)` 追加 `request_id`
- `main.go:39-40` — 唯一初始化点；`main.go:52-53` — `Error` + `os.Exit(1)` 代替 Fatal
- `internal/delivery/http/middleware/logger.go:32-39` — access log 字段；`:43-57` panic 恢复
- 等级现状：无 `Debug`、无 `Fatal`；Info≈34 / Warn≈5 / Error≈11
- 无脱敏 helper；`client_ip` 与 `email` 明文记录；JWT secret 值从不进 slog（`main.go:97`，仅字符串）
- 配置：`internal/config/config.go:39-40` — `-log-level`（env `LOG_LEVEL`，默认 info）、`-log-json`（默认 false）
- 无轮转 / 无文件 sink / 无色 / 无第三方库

### Frontend
- `web/src/` 子目录仅 `views / components / api / router / mock / utils`，无 `stores`/`composables`/`types`
- Pinia 仅 `web/src/main.ts:2,8` 注册，`defineStore` 全仓 0 匹配 → 写实
- `web/src/api/index.ts:4-7` axios 单例 `baseURL:'/api'`、timeout 30000；`:9-15` Bearer 注入；`:17-28` 解包 + 401 跳转；`:30-55` 泛型封装 + mock 分流
- `web/src/router/index.ts:4-15` 懒加载；`:38-50` beforeEach 守卫（localStorage token）；`meta.layout:'blank'`
- 14/14 `.vue` 均 `<script setup lang="ts">`；全仓无 `defineProps/defineEmits`
- `web/tsconfig.json:7` `strict:false`；`web/package.json:6-13` scripts（含 `typecheck: vue-tsc --noEmit`）
- 无前端测试框架；无 i18n；`web/.env.demo` 仅 `VITE_MOCK_MODE=true`

---

## 4. 关键决策与取舍

1. **写实而非写理想**：Pinia 已注册但零使用、`@/` alias 配置未使用、`strict:false`。
   决策为如实记录并标注「当前不启用」，避免子代理产出与既有代码风格冲突的代码。
   （依据 bootstrap PRD `Step 3: Document reality, not ideals`）
2. **拆分前端子文档**：单文件会膨胀且难以按需注入 jsonl；拆分后 `implement.jsonl`
   可按层精确挂载。
3. **不动 `guides/`**：其内容为 Trellis 通用最佳实践，未发现与项目冲突，保持稳定。
4. **不引入新工具/依赖**：纯文档任务，无构建或运行时影响。

---

## 5. 兼容性与回滚

- 变更全部落于 `.trellis/`，对 `go build` / `npm run build` 无影响。
- 回滚形态：`git checkout -- .trellis/spec` 即可还原（新增文件用 `git clean` 或删除）。
- 无数据迁移、无灰度需求。
