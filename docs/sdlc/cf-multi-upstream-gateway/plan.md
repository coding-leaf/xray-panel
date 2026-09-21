# Plan: Cloudflare 多 VPS 弹性调度与顺序漫游网关 - 实施计划

- **关联 Spec**: cf-multi-upstream-gateway
- **实施执行人 / Agent**: Builder
- **当前状态**: Draft / Pending Approval
- **Change Tier**: Tier 2 (单模块特性演进)

---

## 1. 变更文件清单 (Files that change)

| 文件路径 | 变动类型 | 作用说明 |
| :--- | :--- | :--- |
| `deploy/gateway.test.js` | New | 建立零外部依赖的纯 Node.js (v22+) 原生测试与上游 Mock 模拟套件 |
| `deploy/cloudflare-worker-sub-proxy.js` | Modify | Worker 网关重构：多源站解析、Body 缓存复用、2.5s 超时顺序漫游与标头清洗 |
| `deploy/cloudflare-pages/_worker.js` | Modify | Pages 网关重构：多源站解析、Body 缓存复用、顺序漫游寻呼与静态资产直通 |
| `docs/sdlc/cf-multi-upstream-gateway/plan.md` | Modify | 实施过程与里程碑记录工件 |

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

> **原则**：每个步骤必须配对明确的局部验证命令，步步红绿流转，禁止跳过单步验证直接写完提交。

### Milestone 1: 建立自动化测试与模拟套件 (Fail-repro First)
- **操作目标**:
  1. 创建自动化测试脚本 `deploy/gateway.test.js`，采用 Node.js 原生 `node:test` 与 `node:assert` 模块，无需引入第三方 npm 依赖；
  2. 模拟 Cloudflare 运行时 Web 标准 API（`Request`, `Response`, `Headers`, `fetch`, `AbortSignal`）；
  3. 编写覆盖完整场景的测试用例：
     - **用例 1 (多源站配置解析)**: 逗号/换行符分隔、前后空白去除、URL 结尾斜杠过滤、非法协议过滤、占位符 (`yourdomain.com`) 拦截并返回 500；
     - **用例 2 (POST 请求体复用)**: 针对 `/api/portal/claim` 等接口，验证使用 `arrayBuffer()` 预读后，在多台源站寻呼时多次复用 Body 无 `Body already consumed` 报错；
     - **用例 3 (2.5s 极速超时熔断)**: 模拟源站 A 挂起 3000ms，验证触发 2500ms AbortSignal 超时熔断后无缝流转至源站 B；
     - **用例 4 (200 OK 命中即止)**: 源站 A 正常返回 200，网关立即响应并终止后续节点遍历，验证指纹剥离（Server, X-Powered-By）与 `Cache-Control: no-store` 注入；
     - **用例 5 (404/400 业务未命中静默顺延)**: 源站 A 返回 404，网关自动顺延寻呼源站 B 并最终成功返回 200；
     - **用例 6 (漫游全穷尽中立兜底)**:
       - 场景 A: 所有节点返回 404 -> `/api/portal/claim` 返回统一中立 JSON `{"error": "凭据无效、已过期或未配置"}`；
       - 场景 B: 所有节点返回 404 -> `/sub` 返回中立纯文本 `404 Not Found`；
       - 场景 C: 所有节点超时/网络异常/5xx -> 返回统一 `502 Bad Gateway`。
- **涉及文件**:
  - `deploy/gateway.test.js` (New)
- **局部验证命令**:
  ```bash
  node --test deploy/gateway.test.js
  ```
- **预期判据**: 测试文件成功运行，由于网关尚未重构导出对应纯函数或支持多源站漫游，相关用例变红（Fail），确立失败复现基准。

---

### Milestone 2: 网关重构与顺序漫游状态机落地 (Implementation)
- **操作目标**:
  1. 在 `deploy/cloudflare-worker-sub-proxy.js` 与 `deploy/cloudflare-pages/_worker.js` 中落地纯函数与状态机：
     - 实现 `parseUpstreamOrigins(rawOrigins, defaultOrigin)`；
     - 实现非 GET/HEAD 请求的单次读取 Body 缓存机制（`await request.arrayBuffer()`）；
     - 实现顺序漫游循回状态机 (Sequential Roaming Loop) 与 2.5s 独立超时控制 (`AbortSignal.timeout(2500)` / `AbortController` 优雅降级)；
     - 依据响应状态码（200 命中即止、400/403/404 记录 clientMiss 顺延、5xx 记录 serverError 顺延、超时/中断记录 networkError 顺延）驱动转移；
     - 实现标头清洗（剥离 Server, X-Powered-By）与安全头注入（`Cache-Control: no-store`、`X-Forwarded-Proto: https`、`X-Forwarded-Host`、真实 IP）；
     - 落地全穷尽安全中立兜底应答（404 统一错误 JSON / 502 熔断提示）；
  2. 保持 Pages 专属的静态资产直通逻辑（`/` 与 `/portal` 经由 `env.ASSETS.fetch` 返回静态单页，不受影响）；
  3. 规范模块导出（`export { parseUpstreamOrigins, normalizeAndSanitizePath, ... }` 及默认 `fetch` 处理器），以便离线单测套件直接引入。
- **涉及文件**:
  - `deploy/cloudflare-worker-sub-proxy.js` (Modify)
  - `deploy/cloudflare-pages/_worker.js` (Modify)
- **局部验证命令**:
  ```bash
  node --test deploy/gateway.test.js
  ```
- **预期判据**: 伴随式单测套件从红色转为全绿（100% Pass），全部多源站解析、POST 体复用、超时熔断与中立兜底断言全部通过。

---

### Milestone 3: 全局质量门禁与端到端客观验证 (Quality Gate & Regression)
- **操作目标**:
  1. 执行 JS 代码语法分析与静态质量检查，杜绝语法错误与死循环隐患；
  2. 运行完整网关自动化测试集；
  3. 运行 Go 后端全量测试与并发竞态检测，确保项目级无任何破坏性回归；
  4. 运行 `task_cli check` 执行 SDLC 规范完整性核验。
- **涉及文件**:
  - `deploy/cloudflare-worker-sub-proxy.js`
  - `deploy/cloudflare-pages/_worker.js`
  - `deploy/gateway.test.js`
- **客观验证命令组合**:
  ```bash
  node --check deploy/cloudflare-worker-sub-proxy.js
  node --check deploy/cloudflare-pages/_worker.js
  node --test deploy/gateway.test.js
  go test -race ./...
  go vet ./...
  python3 tooling/task_cli.py check
  ```
- **预期判据**: 语法静态校验零错误；JS 网关测试全部绿灯；Go 单元测试全绿且 `-race` 竞态检测零告警；SDLC 门禁核验通过。

---

## 3. 动态核验 7 维风险矩阵 (7-Dimensional Risk Matrix)

| 风险维度 (Dimension) | 评估等级 | 实施期风险应对与防御策略 |
| :--- | :--- | :--- |
| **1. Affected Files** | 低 (2+1 files) | 生产代码仅修改 `deploy/cloudflare-worker-sub-proxy.js` 与 `deploy/cloudflare-pages/_worker.js`，新增测试 `deploy/gateway.test.js`，完全不触碰 `internal/` 下 Go 后端核心 |
| **2. Public API & Protocol** | 零变动 | 严格沿用已有对外路由 (`/api/portal/claim`, `/sub`, `/portal`) 与参数协议，对外部客户端表现完全透明 |
| **3. Data Schema** | 零变动 | 无数据库读写或表结构迁移，纯边缘计算无状态流转 |
| **4. Auth & Security** | 高度加固 | 1. `ArrayBuffer` 内存中隔离重放，不落盘；<br/>2. 敏感凭据路由强制注入 `Cache-Control: no-store`；<br/>3. 剥离源站服务标头，全面隐匿各 VPS 真实指纹与网络拓扑 |
| **5. Dependencies** | 零外部依赖 | 测试与生产代码全部采用 Node.js / V8 原生能力（`node:test`、Fetch、AbortSignal），杜绝外部 npm 供应链污染 |
| **6. Rollback Difficulty** | 极低 (秒级可逆) | 如出现兼容性异常，环境变量切换单源站即刻降级，或 Git 回滚单次发布 |
| **7. Blast Radius** | 极低 | 影响仅局限于边缘反代网关调度层，各 VPS 独立运行不受任何干扰 |

---

## 4. 自适应分批建议 (Batch Strategy)

- **建议策略**: **One-Shot Builder 交付** (或紧凑型 M1-M3 连续执行)
- **判定依据**:
  - 本任务属于 Tier 2 单模块特性，改动范围高度内聚于 `deploy/` 目录；
  - 核心逻辑基于纯 JS 原生实现，且已设计了自包含的 `node:test` 离线模拟套件，前后依赖明确，上下文体量适中；
  - 推荐派发专职 `builder` 一次性完成 M1 桩构建 -> M2 网关重构 -> M3 质量核验的完整实施闭环，避免频繁上下文切换带来的通讯开销。

---

## 5. 实施偏差记录 (Deviations Log)

*在实际实施过程中，若发现技术方案或依赖需要调整，在此严格如实记录：*
- 暂无偏差：方案严格遵循 spec.md 技术契约执行。

---

## 6. 阶段准出签批 (Gate 3 Sign-off)

- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-21 18:43
