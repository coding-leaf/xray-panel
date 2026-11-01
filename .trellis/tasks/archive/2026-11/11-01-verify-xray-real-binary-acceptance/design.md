# 技术设计方案 (Technical Design)

## 1. 架构定位与双层验证边界 (Architecture & Verification Boundaries)

为保证面板配置编译器产物与 Xray-core 真实行为 100% 对齐，避免“单测自圆其说、实际上机报错”的契约漂移，本方案构建“双层真实内核验收体系”：

```text
+-----------------------------------------------------------------------------------+
|                        Trellis Real Xray Acceptance Suite                         |
|                                                                                   |
|  +-----------------------------------------------------------------------------+  |
|  | 0. Binary Discovery Layer: FindXrayBinary()                                 |  |
|  |    Env Vars -> temp/xray(.exe) [New] -> ../temp/ -> bin/ -> System PATH      |  |
|  +-----------------------------------------------------------------------------+  |
|                                         |                                         |
|                  +----------------------+----------------------+                  |
|                  |                                             |                  |
|                  v                                             v                  |
|   +-----------------------------+               +-----------------------------+   |
|   |   Layer A: Static Schema    |               |    Layer B: Live Runtime    |   |
|   |  xray run -test -c tmp.json |               |     xray run -c tmp.json    |   |
|   +-----------------------------+               +-----------------------------+   |
|   | - Inbound Sniffing          |               | - Dynamic Port Bind (:0)    |   |
|   |   (routeOnly/destOverride)  |               | - Dial gRPC 127.0.0.1:port  |   |
|   | - DNS Config (clientIp/etc) |               | - Multi-Protocol AddUser    |   |
|   | - StreamSettings            |               |   (VLESS, Trojan, SS)       |   |
|   |   (httpupgrade/mkcp/alpn)   |               | - User Mutation Re-sync     |   |
|   | - Outbound Settings         |               |   (Email/UUID change flow)  |   |
|   |   (fingerprint/spiderX/WG)  |               | - QueryTrafficStats         |   |
|   | - Routing Rules             |               | - RemoveUser & Tear-down    |   |
|   |   (user/vlessRoute/tag)     |               | - Guaranteed Process Kill   |   |
|   | - Flow Variations           |               +-----------------------------+   |
|   +-----------------------------+                                                 |
+-----------------------------------------------------------------------------------+
```

- **Layer A: 静态配置契约校验 (`xray run -test -c <tmp.json>`)**
  - **定位**：纳秒级静态 Schema 语法和结构合法性检测。
  - **职责**：直接调用 Xray 内核的配置解析引擎（Protobuf 反序列化、JSON 结构体映射、废弃字段排查、依赖校验）。如配置不符合 Xray 官方 Spec，内核将以非 0 退出码报错并输出具体错误行，测试直接判定红灯。
- **Layer B: 动态 gRPC 运行时联动 (`xray run -c <tmp.json>` + gRPC)**
  - **定位**：秒级进程间动态控制面验证。
  - **职责**：在本地环回端口上实际拉起内核，建立内部 gRPC 连接，下发实际多协议账户信息（验证 `account_builder.go` 中的 `BuildAccountMessage`），执行用户关键变动重同步（User Mutation Re-sync），并拉取 `QueryTrafficStats` 确认指标系统正常。

---

## 2. `FindXrayBinary` 发现链路扩展设计 (Discovery Extension)

### 2.1 搜索优先级设计
在 `internal/adapter/xray/xray_integration_test.go` 中，更新 `FindXrayBinary()` 搜索链，保证自动探测到仓库根目录 `temp/` 下的二进制：

1. **显式环境变量**（最高优先级）：`XRAY_BIN`, `XRAY_BIN_PATH`；
2. **全局缓存**：`os.TempDir()/opencode/xray_pre/xray(.exe)`；
3. **本地 `temp/` 目录检索（新增关键链）**：
   - 相对当前路径检索：
     - `./temp/xray`, `../temp/xray`, `../../temp/xray`, `../../../temp/xray`
     - Windows 平台对应 `.exe`；
   - 向上递归至根目录检索（以包含 `go.mod` 为根标识）：
     - `filepath.Join(dir, "temp", "xray")`
     - Windows 平台追加 `filepath.Join(dir, "temp", "xray.exe")`；
4. **本地 `bin/` 目录检索**：保留原有的 relative 与 upward 回溯链；
5. **系统 `PATH`**：`exec.LookPath("xray")` / `exec.LookPath("xray.exe")`。

### 2.2 跨平台可执行性校验
- **Windows**：校验文件存在且非目录（`!fi.IsDir()`），支持扩展名补齐；
- **Linux / macOS**：校验文件权限位包含可执行位（`fi.Mode()&0111 != 0`）。

---

## 3. 静态配置 Schema 验收矩阵 (Static Schema Test Matrix)

在 `compiler_test.go` 中，通过调用 `validateWithXrayCore(t, jsonBytes)` 对以下 6 组配置进行真实二进制校验：

| 校验模块 | 测试场景与关键字段 | 校验重点与内核契约 |
| :--- | :--- | :--- |
| **1. Inbound Sniffing** | `routeOnly: true` / `routeOnly: false`<br>`destOverride: ["http", "tls", "quic", "fakedns"]` | 确保 `fakedns` 与 `quic` 嗅探目标合法，`routeOnly` 布尔值在 core 中被正确解析且不触发 schema 报错。 |
| **2. DNS 完备配置** | `clientIp: "1.2.3.4"`<br>`disableCache: true`<br>`disableFallback: false`<br>`hosts: {"example.com": "127.0.0.1", "domain:google.com": "8.8.8.8"}` | 验证 DNS 模块各项控制开关合法解析，自定义 hosts 静态映射在 core 中正常挂载。 |
| **3. StreamSettings** | `network: "httpupgrade"` (`httpupgradeSettings: {"path": "/up", "host": "test.com"}`)<br>`network: "mkcp"` (`kcpSettings: {mtu: 1350, congestion: true, ...}`)<br>`tlsSettings: {alpn: ["h2", "http/1.1"]}`<br>`fallbacks: [{alpn: "h2", path: "/fallback", dest: 8080}]` | 验证新引入的 `httpupgrade` 规范（替代被淘汰的早期实现）、mKCP 密集参数序列化正确，以及 TLS ALPN / Fallbacks 精确路由无语法错误。 |
| **4. Outbound 增强** | `tlsSettings: {fingerprint: "chrome"}`<br>`realitySettings: {spiderX: "/random_path"}`<br>`wireguardSettings: {mtu: 1420, noKernelTun: true}` | 验证 uTLS 浏览器指纹合法性，验证 Reality SpiderX 爬虫路径语法，以及 WireGuard 用户态运行标志。 |
| **5. Routing 规则** | `rules: [{user: ["user1@test.com", "user2@test.com"], outboundTag: "direct"}]`<br>`rules: [{vlessRoute: 1, outboundTag: "jp-direct"}]`<br>`rules: [{tag: "custom-rule-tag", ...}]` | 验证多用户 Email 匹配规则在 core 路由引擎中的合法性，验证 `vlessRoute` 专属路由与 `tag` 标签规则语法。 |
| **6. User Flow 变种** | `flow: "xtls-rprx-vision"`（TCP Reality）<br>`flow: ""`（WebSocket / xHTTP / gRPC 等非 Vision 传输） | 验证 Vision 流控仅在兼容协议上启用，验证空 Flow 在其他传输协议下合法编译。 |

---

## 4. 动态 gRPC 运行时联动与用户变更重同步设计 (Live gRPC & User Mutation)

### 4.1 动态端口与环境隔离
为杜绝测试端口与系统运行环境冲突：
- `grpcPort := getFreePort(t)`
- `vlessPort := getFreePort(t)`
- `trojanPort := getFreePort(t)`
- `ssPort := getFreePort(t)`
所有端口在绑定前互不相等，完全由内核分配临时端口（`:0`）。

### 4.2 用户变更重同步（User Mutation Re-sync）状态机
在 Xray-core gRPC 规范中，修改用户信息无直接 UpdateUser API。系统采用经典的 `RemoveUser` ➔ `AddUser` 幂等重同步范式。

```text
[初始状态]
   Inbound 正常运行 (无目标用户)
      │
      │ 1. AddUser(email: "alice@test.com", uuid: "UUID-1", flow: "xtls-rprx-vision")
      ▼
[用户已激活 (UUID-1)]
      │
      │ 2. 场景 A: 模拟用户修改 Email
      │    a. RemoveUser(email: "alice@test.com")
      │    b. AddUser(email: "alice-new@test.com", uuid: "UUID-1")
      ▼
[用户 Email 更新完成 (alice-new@test.com, UUID-1)]
      │
      │ 3. 场景 B: 模拟用户重置 UUID
      │    a. RemoveUser(email: "alice-new@test.com")
      │    b. AddUser(email: "alice-new@test.com", uuid: "UUID-2")
      ▼
[用户 UUID 轮换完成 (alice-new@test.com, UUID-2)]
      │
      │ 4. 验证 Stats 与注销
      │    a. QueryTrafficStats (确认 stats 指标接口正常)
      │    b. RemoveUser(email: "alice-new@test.com")
      ▼
[清理完成 (无残留)]
```

### 4.3 多协议 Inbound 账户构建联动测试
集成测试中配置复合入站：
1. **VLESS Inbound**：测试 `VLESSAdapter` 构建的 `AccountMessage`（验证 ID、Flow 注入）；
2. **Trojan Inbound**：测试 `TrojanAdapter` 构建的 `AccountMessage`（验证 Password 注入）；
3. **Shadowsocks Inbound**：测试 `ShadowsocksAdapter` 构建的 `AccountMessage`（验证 Password 与 Cipher 注入）。

---

## 5. 资源清理、隔离与安全保障 (Isolation & Security Invariants)

- **Git 隔离防线**：
  - `.gitignore:42` 拦截 `temp/`，无论测试下载解压官方二进制还是放置 GeoIP/GeoSite 数据库，均完全被 Git 忽略，执行 `git status` 保证工作区保持 Clean。
- **子进程防孤儿保证**：
  - 子进程生命周期绑定 `context.WithCancel`；
  - `defer` 中执行强清理三部曲：
    ```go
    cmdCancel()
    if cmd.Process != nil {
        _ = cmd.Process.Kill()
    }
    _ = cmd.Wait()
    ```
- **测试配置销毁**：
  - 临时 JSON 写入 `os.CreateTemp("", "xray-integration-*.json")`，并在 `defer os.Remove(...)` 中即时清理。
- **凭证零泄漏**：
  - 统一使用 RFC 4122 生成的固定测试 UUID（如 `c8402f1a-6df2-4217-a169-23114d640201`）；
  - 密钥使用本地自生成的占位 base64，严禁在测试中注入任何生产公网密钥或生产节点 IP。
