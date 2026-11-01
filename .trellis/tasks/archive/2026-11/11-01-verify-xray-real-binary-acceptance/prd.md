# Xray 核心源码契约对齐与真实二进制联动验收 (PRD)

## Goal

在根目录已忽略目录 `temp/` 中引入官方 Xray-core 真实二进制（Windows 64-bit），打通自动化发现与真实验收机制。针对前序任务中修复与增强的全部编译器配置 Schema（Sniffing `routeOnly`/`destOverride`、DNS 扩展参数、StreamSettings `httpupgrade`/`mkcp`/TLS ALPN/Fallbacks、Outbound 指纹/SpiderX/WireGuard、路由 `user`/`vlessRoute`/`tag` 匹配、User `Flow` 变种）进行严格的真实内核语法校验；同时拓展动态 gRPC 运行时集成测试（多协议 `AddUser`、`RemoveUser`、用户 Email/UUID 变更重同步、`QueryTrafficStats`），彻底消灭配置编译器与真实 Xray-core 内核行为之间的契约脱节。

## Confirmed Facts (Codebase Audit)

- **`temp/` 目录已被 Git 严格隔离**：`.gitignore:42` 显式忽略了 `temp/`，确保下载解压的官方真实二进制（`xray.exe`、`geoip.dat`、`geosite.dat`）与中间测试配置绝不会被误提交至版本控制。
- **二进制发现链路存在本地检索缺口**：`internal/adapter/xray/xray_integration_test.go:25-99` 中的 `FindXrayBinary()` 检索了环境变量 `XRAY_BIN`/`XRAY_BIN_PATH`、`os.TempDir()` 缓存、相对 `bin/` 目录以及回溯到项目根目录的 `bin/xray(.exe)`，尚未包含本地开发常用的 `<repo_root>/temp/xray(.exe)` 或 `./temp/xray.exe`，导致将二进制放入 `temp/` 后仍需手动导出环境变量。
- **动态端口规避机制完备**：`internal/adapter/xray/xray_integration_test.go:102-108` 提供了 `getFreePort()` 工具函数（通过监听 `:0` 动态分配空闲 TCP 端口），可确保测试端口无冲突并发绑定。
- **现存 gRPC 集成测试覆盖单一**：`internal/adapter/xray/xray_integration_test.go:110-238` 中的 `TestXrayCore_GRPC_RealBinary` 仅覆盖单协议（VLESS TCP）的一次性 `AddUser`、`QueryTrafficStats` 与 `RemoveUser`，缺乏多协议支持（Trojan、Shadowsocks 等）、用户变更重同步（修改 Email / UUID 触发的 Remove+Add 幂等重置）以及异常断开容错测试。
- **静态内核 CLI 校验能力已具备但覆盖不足**：`internal/adapter/xray/compiler_test.go:16-51` 实现了 `validateWithXrayCore(t, jsonBytes)`，通过 `xray run -test -c <tmpPath>` 直接调用真实内核校验配置合法性；但全文件中仅有 3 处调用（`compiler_test.go:602, 647, 676`），大量新编译器特性未经过真实二进制的语法与 Schema 验证。
- **前置任务修复/增强的编译器契约范畴**：
  - 入站 Sniffing：`routeOnly: true/false`，`destOverride: ["http", "tls", "quic", "fakedns"]`；
  - DNS 配置：`clientIp`、`disableCache`、`disableFallback`、`hosts` 静态域名/IP 映射字典；
  - StreamSettings：`httpupgrade`（`httpupgradeSettings`）、`mkcp`（`kcpSettings`）、TLS `alpn`、Fallback `alpn`/`path`；
  - Outbound 设置：TLS `fingerprint`、Reality `spiderX`、WireGuard `mtu` 与 `noKernelTun`；
  - 路由规则：`user`（Email 数组匹配）、`vlessRoute` 专属分流通道、`tag` 显式路由规则；
  - 用户 Flow：`xtls-rprx-vision` 与空 Flow。
- **gRPC 用户动态变动机制**：`internal/adapter/xray/grpc_client.go:88-164` 中 Xray 原生仅暴露 `AddUserOperation` 与 `RemoveUserOperation`，用户修改 Email 或 UUID 需经由 Remove(old) ➔ Add(new) 顺序执行，需在真实内核中验证状态切换的一致性与无冲突残留。

## Requirements

### 1. `FindXrayBinary` 自动发现增强 (Automatic Discovery for `temp/`)
- 在 `internal/adapter/xray/xray_integration_test.go` 中扩展 `FindXrayBinary()` 搜索链：
  - 向上递归至仓库根目录阶段（以及相对路径判断阶段），增加对 `temp/xray`（Linux/macOS）与 `temp/xray.exe`（Windows）的自动检索；
  - 确保只需将官方 Xray 二进制放置于项目根目录 `temp/` 下，执行 `mise x -- go test ./internal/adapter/xray/...` 即可零环境变量配置自动命中并激活真实测试。

### 2. 真实 Xray 二进制静态配置契约验收矩阵 (Static Schema Validation Matrix)
使用 `xray run -test -c` 对前置任务修复与新增的全部编译器特性构建全景校验测试，覆盖：
1. **入站 Sniffing 特性**：
   - `routeOnly: true` 与 `routeOnly: false`；
   - `destOverride` 包含 `["http", "tls", "quic", "fakedns"]`。
2. **DNS 完备配置**：
   - 包含 `clientIp`、`disableCache`、`disableFallback`、`hosts`（静态 IP/域名映射）；
   - 验证 Xray-core 对 DNS 解析配置合法加载。
3. **传输层 StreamSettings 扩展**：
   - `httpupgrade` 协议及 `httpupgradeSettings`（`path`、`host`）；
   - `mkcp` 协议及 `kcpSettings`（`mtu`、`tti`、`uplinkCapacity`、`downlinkCapacity`、`congestion`、`readBufferSize`、`writeBufferSize`、`header`）；
   - TLS `alpn`（如 `["h2", "http/1.1"]`）；
   - Fallbacks 的 `alpn` 与 `path` 精确匹配配置。
4. **出站配置增强**：
   - Outbound TLS 的 `fingerprint`（`chrome`、`firefox`、`safari` 等 uTLS 模拟）；
   - Reality `spiderX` 路径参数；
   - WireGuard `mtu` 与 `noKernelTun: true/false`。
5. **路由规则与多字段匹配**：
   - 规则 `user` 字段（按用户 Email 数组分流）；
   - `vlessRoute` 跨机通道专属规则；
   - 显式 `inboundTag`、`outboundTag`、`domain`、`ip` 复合匹配。
6. **用户 Flow 变种**：
   - `xtls-rprx-vision`（TCP/TLS/Reality）；
   - WebSocket/gRPC/xHTTP 等非 Vision 传输下的空 Flow。

### 3. 动态 gRPC 运行时联动与用户变更重同步验收 (Live gRPC Integration & User Mutation)
通过真实 Xray 进程拉起测试服务，验证运行中动态操作：
1. **多协议用户下发**：
   - 启动包含 VLESS、Trojan、Shadowsocks 等多入站的真实 Xray 进程；
   - 通过 `GRPCClient.AddUser` 分别为各协议入站下发用户账户（验证 `BuildAccountMessage` 契约与 Xray-core 内部 Account 解包无报错）。
2. **用户身份变更重同步（User Mutation Re-sync）**：
   - 模拟用户修改 Email（`RemoveUser(oldEmail)` ➔ `AddUser(newEmail, sameUUID)`）；
   - 模拟用户重置 UUID（`RemoveUser(email)` ➔ `AddUser(email, newUUID)`）；
   - 验证连续操作在 Xray-core 内存中无幽灵残留、无 UUID 冲突、无状态锁死。
3. **流量监控与查询**：
   - 执行 `GRPCClient.QueryTrafficStats`，验证统计服务返回数据结构与预期 tag/email 格式对齐，零崩溃与数据正常解析。
4. **清理与注销**：
   - 执行 `RemoveUser` 彻底注销用户；
   - 验证重复移除已注销用户的错误或幂等表现，符合系统预期。

### 4. 运行环境安全、资源清理与隔离防线 (Isolation & Safety Invariants)
- **零凭证泄露**：全部测试配置一律使用 RFC 4122 生成的模拟 UUID、本地自签名或模拟测试密钥，杜绝任何生产环境凭证、真实公网 IP 或敏感域名的出现；
- **动态端口隔离**：测试动态端口全部由 `getFreePort()` 自动分配，规避与本机运行的 panel、开发服务或 Xray 守护进程产生端口冲突；
- **进程生命周期强保证**：通过 `context.WithCancel`、`defer cmdCancel()` 以及明确的 `cmd.Process.Kill()`，确保无论测试成功、失败或 panic，拉起的 Xray 二进制子进程均能 100% 被清理，绝不残留孤儿僵尸进程；
- **文件防污染**：临时生成的配置文件写入 OS 临时目录并在 `defer` 中可靠删除；`temp/` 下的二进制由 `.gitignore:42` 拦截，绝不污染代码树。

## Acceptance Criteria

- [ ] `FindXrayBinary` 支持自动检索 `<repo_root>/temp/xray(.exe)` 及相对 `temp/` 路径，无需手动配置环境变量即可在存在 `temp/xray.exe` 时自动开启真实测试。
- [ ] 真实 Xray 二进制静态配置校验（`xray run -test -c`）通过对 6 大编译器特性（Sniffing `routeOnly`/`destOverride`、DNS 全字段、StreamSettings `httpupgrade`/`mkcp`/TLS `alpn`/Fallbacks、Outbound `fingerprint`/`spiderX`/WireGuard、路由 `user`/`vlessRoute`/`tag`、Flow 变种）的全覆盖测试。
- [ ] 真实 Xray gRPC 动态测试覆盖多协议 `AddUser`、用户 Email/UUID 变更重同步（Remove+Add）、`QueryTrafficStats` 与 `RemoveUser`，测试在真实进程中 100% 成功。
- [ ] 测试生命周期管理严密，进程退出与资源清理完全可靠，无僵尸进程，无硬编码端口冲突。
- [ ] `mise x -- go test -v ./internal/adapter/xray/...` 全部通过（包含真实二进制测试与普通单测）。
- [ ] 全量工程门禁执行通过：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`cd web && mise x -- npm run build`。

## Out of Scope

- 在测试中发起真实的公网端到端代理握手（如真实连接外部 Telegram/Google 或真实 WireGuard 对端网络），仅聚焦于 Xray-core 本地语法/Schema 校验与本地回环 gRPC 运行时契约；
- Web 前端界面的功能实现（由 Child Task 1、2、3 负责）；
- 生产环境自动化下载部署脚本或 systemd 服务单元管理。

## Open Questions

None. 所有事实源、发现路径与测试用例矩阵均已与源码及架构规约严格对齐。
