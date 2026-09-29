# PRD — 前端 TypeScript 与 LSP 代码卫生约束强化

## 1. 目标与背景 (Background & Goals)

为防止 AI 或开发过程中生成无意义参数、未用变量、僵尸导入（Dead Imports）与拼写笔误，需要在项目工程配置中建立静态分析硬约束（TypeScript 编译器与 IDE LSP），使 `vue-tsc` 具备拦截“无意义参数”与“未用局部变量”的硬门禁能力，在后续重构与开发中形成严密的自动化代码卫生防线。

## 2. 核心功能与需求规范 (Functional Requirements)

### 2.1 TypeScript 编译器选项加固 (`web/tsconfig.json`)
- 启用 `"noUnusedLocals": true`：禁止声明未使用的局部变量、函数与导入；
- 启用 `"noUnusedParameters": true`：禁止未使用的函数参数（如必须占位则显式用 `_` 前缀）；
- 启用 `"noFallthroughCasesInSwitch": true`：拦截 switch case 穿透隐患；
- 启用 `"forceConsistentCasingInFileNames": true`：防止跨平台文件名大小写问题。

### 2.2 编辑器与 LSP 统一配置 (`.vscode/settings.json`)
- 配置 TypeScript SDK 路径与 Vue 官方插件 (Volar/Vue-Official) 混合模式；
- 配置保存时自动组织导入 (`source.organizeImports`) 与校验一致性。

### 2.3 存量代码卫生平稳清理 (Baseline Cleanup)
- 开启新规则后，运行 `cd web && mise x -- npm run typecheck`；
- 对报告的存量“未使用导入”、“死变量”与“未使用参数”进行安全清理，确保在新约束下 `vue-tsc` 达到 0 警告、0 错误全绿基线。

### 2.4 规范同步 (`.trellis/spec/frontend/quality-guidelines.md`)
- 将 `noUnusedLocals` / `noUnusedParameters` 正式纳入前端开发门禁规范。

## 3. 验收标准 (Acceptance Criteria)

- [ ] `web/tsconfig.json` 包含 `noUnusedLocals`、`noUnusedParameters` 等代码卫生配置；
- [ ] `.vscode/settings.json` 完成工程级 LSP 约束配置；
- [ ] 执行 `cd web && npm run typecheck` 零错误通过；
- [ ] 执行 `cd web && npm run build` 零报错，构建产物正常；
- [ ] 后端测试与构建 `go test ./...` 与 `go build .` 100% 通过。
