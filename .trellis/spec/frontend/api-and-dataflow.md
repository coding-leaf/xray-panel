# API & Dataflow

> axios 单例、请求拦截、鉴权跳转、mock 分流与前端数据流

---

## 数据流总览

```
View  ──►  web/src/api/index.ts (axios 单例)  ──┬──►  /api/**  (真实后端, Vite 代理)
                                               └──►  web/src/mock/index.ts (mock 模式)
                              │
                              └──►  toast 单例 (web/src/utils/toast.ts) 提示用户
```

- 页面不直接调用 `axios`，一律经 `api` 单例。
- 后端返回体在拦截器中被解包，调用方拿到的是**业务数据本身**，不是 `AxiosResponse`。

---

## axios 单例（`web/src/api/index.ts`）

- 实例创建（`:4-7`）：`baseURL: '/api'`、`timeout: 30000`。
- **请求拦截器**（`:9-15`）：从 `localStorage` 读取 `token` 并注入
  `Authorization: Bearer <token>`。
- **响应拦截器**（`:17-28`）：
  - 成功时直接返回 `response.data`（`:18`），因此 `api.get('/x')` 的返回值即响应体；
  - 收到 **401** 时清除 token 并硬跳转 `/login`
    （跳过 `/login` 与 `/portal`，`:20-25`）；
  - 其它错误以 `error.response?.data?.error || error.message` 拒绝（`:26`），
    即**优先取后端 `error` 字段**。
- **泛型封装**（`:30-55`）：`api.get<T>` / `api.post<T>` / `api.put<T>` / `api.delete<T>`，
  每个方法在 `isMockMode()` 为真时改走 `handleMockRequest`。
- 默认导出 `api` 并 re-export `./reality`（`:57-58`）。

---

## 类型化接口

- 泛型默认为 `any`（`<T = any>`），多数页面直接以 `ref<any>` 接收
  （例：`web/src/views/DashboardView.vue:278`）。这是当前现状，非强制约定。
- 唯一类型化的领域模块是 `web/src/api/reality.ts`：
  接口 `RealityDomainStatus` / `RealityCheckItem` / `RealitySummaryStatus`（`:3-30`），
  函数 `getRealityStatus()`（`:32-38`）、`checkRealityStatus()`（`:40-45`）。
  其中对可能的 `res.data` 包装做了防御性解包（`:34,42`）。

> **规范**：新增领域接口优先参照 `reality.ts` 补齐类型；
> 不要在页面里对同一后端字段重复做局部强转 —— 收敛到 `api/` 下。

---

## mock 模式（`web/src/mock/`）

- 触发条件（`web/src/mock/index.ts:10-13`）：
  `VITE_MOCK_MODE === 'true'`、`env.MODE === 'demo'`、URL 带 `?mock=true`、或 `github.io` 域名。
- `index.ts` 是请求分发器（`handleMockRequest`），`storage.ts` 以 localStorage 模拟可变状态。
- 独立 env 文件 `web/.env.demo` 仅含 `VITE_MOCK_MODE=true`（`:1`）。
- 项目**没有** `ImportMetaEnv` 类型声明；读取 env 时使用 `(import.meta as any).env?.BASE_URL`
  这类写法（`web/src/router/index.ts:34`）。

---

## 错误处理与提示

- HTTP 层错误由响应拦截器统一 reject，错误文案优先取后端 `error` 字段。
- 用户可见提示统一走 `toast` 单例：`import { toast } from '../utils/toast'`，
  由 `ToastContainer.vue` 渲染 —— 不要在页面内自建弹窗提示。
- 全局 `401` 已由拦截器强制跳登录，页面无需重复处理鉴权失效。

---

## 入站与出站协议规范与参数清洗 (Inbound & Outbound Protocol Contracts)

为杜绝非法配置导致 Xray-core 启动异常或静默崩溃，前端在提交前必须通过纯函数严格执行协议级参数清洗 (`sanitizeInboundPayload` / `sanitizeOutboundPayload`)：

1. **入站协议 (Inbounds)**:
   - **Socks / HTTP / dokodemo-door**: 属于无传输安全层封装的本地/原生应用协议，`streamSettings` 必须清洗为空字符串 `""`，锁定 `network = 'tcp', security = 'none'`；
   - **Trojan**: 客户端认证凭据必须为 `{ password, email, level: 0 }`，严禁输出 `id`；
   - **Shadowsocks / SS2022**: 必须提供合法加密算法 (`method`)，单用户填密码或结合关联用户生成 Sub-Key clients；
   - **VMess / Socks / HTTP**: 严禁选择 `REALITY` 伪装；
   - **Flow 流控 (xtls-rprx-vision)**: 严格限定仅在 `vless` + `tcp` + (`reality` 或 `tls`) 下有效并允许输出，其他所有协议一律清洗为空。

2. **出站协议 (Outbounds)**:
   - **freedom / blackhole / wireguard / socks / http / dns**: 均为无传输安全层出站，`streamSettings` 必须清空为 `""`，禁止附带任何 Reality / TLS / XHTTP 参数；
   - **vmess / shadowsocks**: 安全协议仅允许选择 `tls` 或 `none`，禁止选择 `reality`；
   - **dns**: 作为系统出站协议，输出空的 settings `{}`，用于 DNS 分流路由规则。

