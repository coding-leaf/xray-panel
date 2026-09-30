# Design: 前端入站出站协议匹配与参数表单规范化重构

## 1. 架构与边界设计

本任务属于前端交付层重构，主要修改范围为 `web/src/views/inbounds/` 和 `web/src/views/outbounds/`。
后端 `internal/adapter/xray` 与 `internal/protocol` 已经具备强类型的 `InboundStreamAccessor` 与 `ProtocolAdapter`，前端应当严格遵循后端的协议与 Xray 核心官方配置格式契约。

### 1.1 Inbound 协议矩阵与契约映射

| 协议 (Protocol) | 传输层 (Network) | 安全层 (Security) | Flow 流控 | clients 格式 / settings 字段 |
| :--- | :--- | :--- | :--- | :--- |
| **vless** | tcp, xhttp, grpc, ws, httpupgrade | reality, tls, none | 仅 tcp+tls/reality 支持 vision | `{ id: uuid, flow?: vision, email }`, `decryption: "none"`, `fallbacks?` |
| **vmess** | tcp, xhttp, grpc, ws, httpupgrade | tls, none (严禁 reality) | 无 | `{ id: uuid, email, level: 0 }` |
| **trojan** | tcp, ws, grpc, xhttp | tls, reality (xray 支持), none | 无 | `{ password: uuid, email, level: 0 }`, `fallbacks?` |
| **shadowsocks** | tcp, udp, tcp+udp | none | 无 | `method`, `password` (单用户) 或 `clients: [{ password, method, email }]` |
| **socks** | 仅标准 (无 streamSettings) | none | 无 | `auth: "noauth"\|"password"`, `accounts: [{ user, pass }]`, `udp: boolean` |
| **http** | 仅标准 (无 streamSettings) | none | 无 | `accounts: [{ user, pass }]` |
| **dokodemo-door** | 仅标准 (无 streamSettings) | none | 无 | `address: string`, `port: number`, `network: "tcp"\|"udp"\|"tcp,udp"` |

### 1.2 Outbound 协议矩阵与契约映射

| 协议 (Protocol) | Settings 结构 | StreamSettings 允许配置 |
| :--- | :--- | :--- |
| **freedom** | `{ domainStrategy }` | 否（清空） |
| **blackhole** | `{ response: { type } }` | 否（清空） |
| **wireguard** | `{ secretKey, address, peers: [...] }` | 否（清空） |
| **dns** | `{}` | 否（清空） |
| **socks** | `{ servers: [{ address, port, users?: [...] }] }` | 否（清空） |
| **http** | `{ servers: [{ address, port, users?: [...] }] }` | 否（清空） |
| **vless** | `{ vnext: [{ address, port, users: [{ id, flow, encryption }] }] }` | 是（支持 reality / tls） |
| **vmess** | `{ vnext: [{ address, port, users: [{ id, security, alterId }] }] }` | 是（仅 tls / none，禁止 reality） |
| **trojan** | `{ servers: [{ address, port, password }] }` | 是（tls / none） |
| **shadowsocks** | `{ servers: [{ address, port, method, password }] }` | 是（仅 tls / none，禁止 reality） |

---

## 2. 状态模型与函数设计

### 2.1 Inbound 字段扩展 (`inbounds/types.ts`)
- `InboundFormData` 增加 `ssMethod: string`, `ssPassword: string`；
- `sanitizeInboundPayload(raw)`：
  - 如果 `['socks', 'http', 'dokodemo-door'].includes(clean.protocol)`，强制 `network = 'tcp', security = 'none'`；
  - 如果 `clean.protocol === 'shadowsocks'`，清洗 `ssMethod`、`ssPassword`；
  - 如果 `clean.protocol !== 'vless'`，清空 `vlessFlow`。
- `buildSettingsJSON(clean, usersList)`：
  - 支持 `trojan` 生成 `{ password: userObj.uuid, email: userObj.email, level: 0 }`；
  - 支持 `shadowsocks` 结合 `ssMethod` 和选中用户生成规范的 clients；
  - 完善 `socks`、`http`、`dokodemo-door` 序列化。
- `buildStreamSettingsJSON(clean)`：
  - 如果是 `socks` / `http` / `dokodemo-door`，直接返回空字符串 `""`。

### 2.2 InboundFormDrawer UI 模板交互
- 协议切换 (`onProtocolChange`)：自动重置或切换兼容的传输层与安全层；
- 新增专属 SectionCard：
  - Socks 认证与 UDP 开关；
  - HTTP 认证凭据；
  - dokodemo-door 目标地址、端口与网络；
  - Shadowsocks 加密方式与密码模式；
  - 授权用户关联与 VLESS 分流线路仅在对应协议下渲染。

### 2.3 Outbound 数据清洗与 UI 联动 (`outbounds/types.ts` & `OutboundFormDrawer.vue`)
- `buildStreamSettingsJSON(form)`：
  - 排除列表增加 `['freedom', 'blackhole', 'wireguard', 'socks', 'http', 'dns']`；
- Protocol 下拉选项增加 `dns (DNS 拦截与分流)`；
- 安全协议下拉选项：当 `protocol === 'vmess'` 或 `protocol === 'shadowsocks'` 时，屏蔽 `reality` 选项。

---

## 3. 回退与兼容性保障

- 存量 VLESS / Trojan / VMess 配置完全无损加载回填；
- 后端 `compiler.go` 原有配置编译器依然健壮，前端纯化数据后直接减轻后端清洗负担，保障热重启与保存 100% 成功。
