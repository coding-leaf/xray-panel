# 实施计划 (Implementation Plan)

## 任务目标

在根目录 `temp/` 中就位官方 Xray-core 二进制，更新 `FindXrayBinary()` 发现链，编写并跑通全套编译器配置 Schema 真实内核静态校验（`xray run -test -c`）以及多协议动态 gRPC 运行时联动与用户变更重同步（User Mutation Re-sync）验收测试。

---

## 阶段一：真实二进制环境就绪与 Git 隔离检查 (Preparation & Staging)

- [x] **1.1 确认 `temp/` 目录的 Git 隔离状态**
  - 检查 `.gitignore:42` 确保 `temp/` 规则生效；
  - 验证命令：`git check-ignore temp/`。
- [x] **1.2 建立 `temp/` 目录并放置官方 Xray 二进制**
  - 在项目根目录下创建 `temp/` 目录；
  - 下载官方 `Xray-windows-64.zip` 或解压 `xray.exe`、`geoip.dat`、`geosite.dat` 至 `temp/` 目录下；
  - 验证命令：`./temp/xray.exe version` 能够正常输出版本信息；
  - 验证命令：`git status --porcelain` 确认没有新增任何未跟踪文件。

---

## 阶段二：`FindXrayBinary` 发现机制扩展 (Binary Discovery Extension)

- [x] **2.1 在 `internal/adapter/xray/xray_integration_test.go` 中扩展 `FindXrayBinary`**
  - 在向上递归搜索根目录（包含 `go.mod`）的逻辑中，追加对 `temp/xray` 与 `temp/xray.exe` 的检测；
  - 在相对候选路径列表中追加 `./temp/xray(.exe)`、`../temp/xray(.exe)`；
  - 确保跨平台兼容性（Windows 检测可读，Linux/macOS 检测可执行位）。
- [x] **2.2 验证二进制自动发现**
  - 不设置任何 `XRAY_BIN` 环境变量，运行：
    `mise x -- go test -v -run TestXrayCore_GRPC_RealBinary ./internal/adapter/xray/...`
  - 确认测试不再触发 `t.Skip("xray binary not found...")`，而是直接调用真实 `temp/xray.exe` 运行通过。

---

## 阶段三：编译器全特性静态 Schema 真实内核校验 (Static Schema Matrix)

- [x] **3.1 编写 Inbound Sniffing 契约校验用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖 `routeOnly: true/false` 与 `destOverride: ["http", "tls", "quic", "fakedns"]`；
  - 调用 `validateWithXrayCore(t, jsonBytes)` 确保 Xray-core 退出码为 0。
- [x] **3.2 编写 DNS 完备配置校验用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖 `clientIp`、`disableCache`、`disableFallback` 及 `hosts` 映射；
  - 调用 `validateWithXrayCore` 验证 DNS 语法无误。
- [x] **3.3 编写 StreamSettings 扩展用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖 `httpupgrade` (`httpupgradeSettings`)、`mkcp` (`kcpSettings`)、TLS `alpn` 与 Fallback 路由配置；
  - 调用 `validateWithXrayCore` 验证内核成功解析。
- [x] **3.4 编写 Outbound 增强配置校验用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖 Outbound TLS `fingerprint`、Reality `spiderX` 与 WireGuard `mtu`/`noKernelTun`；
  - 调用 `validateWithXrayCore` 验证内核语法无误。
- [x] **3.5 编写路由规则与 User 匹配用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖路由规则中的 `user`（Email 数组）、`vlessRoute` 专属规则与 `tag` 匹配；
  - 调用 `validateWithXrayCore` 验证规则正常加载。
- [x] **3.6 编写 User Flow 变种校验用例**
  - 文件：`internal/adapter/xray/compiler_test.go`
  - 覆盖 `xtls-rprx-vision` 与空 Flow 在对应传输配置下的编译产物并验证。

---

## 阶段四：多协议运行时联动与用户变更重同步验收 (Live gRPC & User Mutation)

- [x] **4.1 扩展多协议实时入站启动**
  - 文件：`internal/adapter/xray/xray_integration_test.go`
  - 在测试中配置同时包含 VLESS、Trojan、Shadowsocks 的复合配置，并在动态空闲端口（`getFreePort`）上启动 Xray 进程；
  - 验证 gRPC 连接建立与健康探活。
- [x] **4.2 验证多协议 `AddUser` 动态注入**
  - 通过 `GRPCClient.AddUser` 分别为 VLESS、Trojan、Shadowsocks 注入合法用户；
  - 确保 `BuildAccountMessage` 生成的 Protobuf TypedMessage 在运行中内核被正确加载，无报错。
- [x] **4.3 编写并验证用户变更重同步 (User Mutation Re-sync)**
  - 模拟场景 A（修改 Email）：`RemoveUser(oldEmail)` ➔ `AddUser(newEmail, sameUUID)`；
  - 模拟场景 B（重置 UUID）：`RemoveUser(newEmail)` ➔ `AddUser(newEmail, newUUID)`；
  - 验证连续操作前后 Xray-core 均能正常接受且无状态死锁或幽灵冲突。
- [x] **4.4 验证流量统计与资源销毁**
  - 执行 `GRPCClient.QueryTrafficStats` 确认返回指标包含已注册的入站与用户通道；
  - 执行 `RemoveUser` 注销测试账户；
  - 验证测试退出时子进程强制 kill、临时配置文件可靠删除、端口正常释放。

---

## 阶段五：全量门禁执行与规范验收 (Quality Gates & Review)

- [x] **5.1 运行 Xray 适配层完整测试**
  - `mise x -- go test -v ./internal/adapter/xray/...`
- [x] **5.2 运行后端全量质量门禁**
  - `mise x -- go test ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build .`
- [x] **5.3 运行前端构建门禁**
  - `cd web && mise x -- npm run build`
- [x] **5.4 验证 Git 工作区纯净度**
  - `git status` 确保 `temp/` 及任何临时测试文件均未被暂存或未被跟踪。

---

## 关键验证命令清单 (Verification Commands)

```bash
# 1. 确认 temp 目录被 git 忽略
git check-ignore temp/
# 输出应包含 temp/

# 2. 检查放置的真实二进制
./temp/xray.exe version

# 3. 运行 Xray 适配层单测及真实集成测试 (自动发现 temp/xray.exe)
mise x -- go test -v ./internal/adapter/xray/...

# 4. 后端全套质量门禁
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .

# 5. 前端构建检查
cd web && mise x -- npm run build
```

---

## 风险文件与回滚点 (Risky Files & Rollback Points)

| 文件路径 | 风险点说明 | 防御与回滚策略 |
| :--- | :--- | :--- |
| `internal/adapter/xray/xray_integration_test.go` | 进程管理不善可能导致本地测试遗留孤儿 Xray 进程或端口占用 | 采用 `context.WithCancel` + `defer cmd.Process.Kill()` 强保活控制；采用 `getFreePort()` 动态分配 `:0` 端口。 |
| `internal/adapter/xray/compiler_test.go` | 真实内核对旧配置或格式敏感，容易因字段微小变化触发解析失败 | 遵循 Xray-core 官方 Protobuf 结构；通过 `validateWithXrayCore` 输出完整 JSON 与 stderr 错误行便于秒级排错。 |
| 仓库根目录 `temp/` | 下载真实二进制如果误提交至 Git 会大幅膨胀仓库体积 | `.gitignore:42` 严格把关，测试前后执行 `git status` 确认无变更泄漏。 |

---

## 实施前自检清单 (Pre-implementation Checks)

- [x] `prd.md` 需求、约束与验收标准已收敛闭环，无残留疑问；
- [x] `design.md` 双层验证拓扑、发现链机制、测试矩阵与安全隔离边界已明确；
- [x] `implement.jsonl` 与 `check.jsonl` 上下文清单已填充合规规范；
- [x] Trellis 任务状态检验通过（`python ./.trellis/scripts/task.py validate verify-xray-real-binary-acceptance`）。
