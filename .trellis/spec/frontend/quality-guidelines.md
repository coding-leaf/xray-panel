# Quality & Build Guidelines

> 前端质量门禁、无测试现状与安全防线

---

## 构建与类型门禁

交付前端改动前必须通过两条命令（均以 `web/` 为工作目录）：

```bash
cd web && mise x -- npm run typecheck   # vue-tsc --noEmit
cd web && mise x -- npm run build       # vite build
```

`web/package.json:6-13` 提供的完整脚本：

| 脚本 | 命令 | 用途 |
|------|------|------|
| `dev` | `vite` | 本地开发（`/api` 代理到 `http://127.0.0.1:9000`） |
| `dev:demo` | `vite --mode demo` | demo/mock 模式开发 |
| `build` | `vite build` | 生产构建，输出 `web/dist` |
| `build:demo` | `vite build --mode demo` | demo 构建 |
| `typecheck` | `vue-tsc --noEmit` | 类型检查 |
| `preview` | `vite preview` | 预览构建产物 |

---

## 类型安全与代码卫生（写实与约束）

- `web/tsconfig.json` 显式保持 `strict: false` —— 存量类型错误不作为阻断门禁；
  API 泛型默认 `any`，页面普遍 `ref<any>`。
- **代码卫生硬门禁**：
  - `"noUnusedLocals": true`：禁止声明未使用的局部变量与僵尸导入；
  - `"noUnusedParameters": true`：禁止未使用的函数参数（占位参数必须显式使用 `_` 或 `_` 前缀如 `_from`）；
  - `"noFallthroughCasesInSwitch": true`：拦截 switch case 穿透隐患；
  - `"forceConsistentCasingInFileNames": true`：防止跨平台文件名大小写问题。
- 环境变量无 `ImportMetaEnv` 声明，读取处使用 `(import.meta as any).env?.*`。

> **规范**：不要求为存量代码补齐严格类型，但**新增领域 API 应参照
> `web/src/api/reality.ts` 提供类型**，避免继续扩散无类型数据。
> 所有代码提交必须保证 `npm run typecheck` 零警告零错误。

---

## 测试现状：无

- **前端没有任何自动化测试**：无 vitest / jest / playwright / cypress 依赖，
  无测试文件、无测试脚本、无测试配置文件。
- 唯一自动化保障是 `typecheck` 与 `build`。
- 修改前端逻辑时，需手工在浏览器验证关键路径，并在任务记录中说明验证方式。

---

## 安全防线

- **凭据不留存**：禁止在 `LocalStorage` / `SessionStorage` 长久保存明文提取码；
  Portal 等敏感页面必须支持超时锁屏物理自毁（见 `frontend/index.md`）。
- **Token 处理**：认证 token 存于 `localStorage.token`，由 axios 拦截器注入请求头，
  401 时由拦截器统一清除并跳登录 —— 页面不要自行重复实现该逻辑。
- **不引入明文敏感信息**：禁止在源码、配置或 demo 数据中写入真实公网 IP、生产域名、
  Token 或私钥；mock 数据使用模拟值。

---

## 依赖与产物

- 构建产物输出到 `web/dist`，由根目录 `embedded.go`（`//go:embed all:web/dist`）
  打包进单一二进制；因此**前端构建失败会阻断后端交付**。
- `web/vite.config.ts` 使用 `base: './'`，并手动切分 vendor chunk（`:26-49`）；
  新增大型依赖时留意产物体积。
- 新增运行时依赖前先确认是否必要（当前核心依赖仅 vue、vue-router、pinia、
  axios、lucide-vue-next、qrcode.vue）。
