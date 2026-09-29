# Design: 文档刷新与归档

## 1. 版本事实源与判定规则

**结论**：以 README / CHANGELOG 的发布线为事实源，当前为 `v2.6.0-beta.1`。

- `main.go:32` 的 `Version` 是**本地构建兜底值**；CI/release 通过 ldflags 注入覆盖（`.github/workflows/ci.yml:78`、`release.yml:92` 均为 `-X 'main.Version=...'`）。
- 因此把兜底值同步为 `v2.6.0-beta.1`，消除"文档说 beta.1、本地构建说 v2.5.0"的矛盾。
- 文档中不再手写相互独立的版本数字：README 徽章、CHANGELOG 首条、`main.go` 三者必须一致。
- 不改动 CHANGELOG 历史条目，只对齐当前版本口径。

## 2. 归档模型（docs/archive/）

```
docs/
├── ARCHITECTURE_AND_LOGIC.md          # 新版（v2.6.0-beta.1）
├── archive/
│   ├── README.md                      # 封存说明（只读约定）
│   ├── ARCHITECTURE_AND_LOGIC.v2.0.0.md   # 旧架构文档原样封存
│   └── sdlc/                          # 旧 SDLC 机制整体封存（含 _template/）
├── ARCHITECTURE_AND_LOGIC.md.bak?     # 否
└── sdlc/                              # 迁移后不再存在
```

规则：
- **封存 = 原样迁移**，用 `git mv` 保留文件历史；不改动封存物内容。
- 封存物只读，后续演进不再回填；新文档在原路径（`docs/ARCHITECTURE_AND_LOGIC.md`）以新版本整体重写。
- 命名：`<原文件名>.v<旧版本>.md`，版本取自旧文档标题（`v2.0.0`）。

## 3. 各文档处理策略

### 3.1 README.md — 原地增量修正（不封存）
README 是入口文档，主体结构仍有效，采用**定点修正**而非重写：

| 位置 | 动作 |
|---|---|
| L4 版本徽章 | 保持 `v2.6.0-beta.1` |
| L5 Go 徽章 | `Go 1.22+` → `Go 1.26+`（对齐 `go.mod:3`） |
| L208-216 参数表 | 补 `-log-level` / `-log-json` / `-public-url` / `-v`，默认值照抄 `config.go` |
| L158-173 分层树 | 与 `internal/` 实目录对齐，补 `config/`、`pkg/`、`adapter/reality/`、`xray/proto/` |

不改：功能特性、CHANGELOG 正文、快速开始、安全建议。

### 3.2 docs/ARCHITECTURE_AND_LOGIC.md — 新版重写 + 旧版封存
- 旧版（v2.0.0）→ `docs/archive/ARCHITECTURE_AND_LOGIC.v2.0.0.md`（git mv）。
- 新版在原路径重写，保留 v2.0.0 仍有效的骨架（领域模型、RouteID 单端口多出口、路由分层、Ticket 阅后即焚、反探测网关），新增：
  - v2.1+ 子系统：SubRoute 用户隔离与 Layer 3 物理阻断、Reality 域名合规巡检、Telegram Bot 运维、审计日志、GeoData 热更新、Alert 告警、
  - 协议层：hysteria2 / socks / vmess / shadowsocks / trojan / vless 多态注册表（`internal/protocol/registry.go`），
  - 服务编排：8 个服务（Host Monitor/HTTP/Traffic Sync/Reality Sync/Telegram Bot 等，见 `main.go:171-180`）与两阶段释放，
  - 持久化：WAL + 单事务批量落盘（BatchSyncTraffic），
  - 边缘：CF Worker/Pages 多机漫游网关 + 提取门户，
  - 前端：Vue3 控制台体系（Table-First + 异步抽屉）与分包策略。
- 版本标记统一为 `v2.6.0-beta.1`（标题与正文）。

### 3.3 docs/sdlc/ — 整体封存
- `git mv docs/sdlc docs/archive/sdlc`。
- 不在 `ARCHIVE.md` 补新任务（机制已停用）。
- 全仓仅 README 有一条指向 `docs/ARCHITECTURE_AND_LOGIC.md` 的链接（`README.md:156`），无 `docs/sdlc` 引用，故链接无需改；但需重新验证（AC7）。

## 4. 契约与兼容性

- **无公共 API / DB Schema / 持久化格式变更**：仅 `main.go` 常量字符串与 Markdown。
- **ldflags 兼容**：`main.Version` 变量名不变，CI 注入不受影响。
- **构建兼容**：`docs/`、`docs/archive/` 不参与 `go:embed`（只嵌入 `web/dist`，见 `embedded.go:8`），移动无副作用。

## 5. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 架构文档重写丢失旧版内容 | 旧版 git mv 封存，可随时对照；AC5 校验完整性 |
| 参数表抄错默认值 | 逐条对照 `config.go:33-42`，作为 AC2 |
| 误删 `docs/sdlc` 而非移动 | 使用 `git mv`，`git status` 应显示 rename |
| 版本口径再次分叉 | 明确"三处一致"作为 AC1 |

回滚：`git revert`/`git restore` 即可；封存移动可逆。

## 6. 不做

- 不引入文档生成器/校验脚本（KISS）。
- 不改 CI 的 go/node 版本线（另立任务，超出本 PRD 范围）。
