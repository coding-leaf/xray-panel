# xray-panel Agent 模式架构、部署与文档/脚本缺口调研报告

> 任务目录：`.trellis/tasks/11-21-agent-deploy-gcp/`
> 范围约束：仅审计 Agent 编译、Master-Agent 接入、部署脚本与文档缺口；**不包含 Xray 配置教学**（仅记录 Agent 对 Xray 运行时的最小依赖与容错边界）。

---

## 1. Agent 编译与启动入口

### 1.1 二进制拆分与架构边界
- **独立轻量二进制**：Agent 与 Master (`panel`) **不共用**可执行文件，入口分离：
  - Master (`panel`)：`main.go:1-276` + `embedded.go`（内嵌 `web/dist` 静态前端及全量控制面服务）。
  - Agent (`xray-agent`)：`cmd/agent/main.go:1-153`（仅依赖 `panel/internal/adapter/xray`、`panel/internal/agent`、`panel/internal/pkg/logger`，无 `gin`/`gorm`/`sqlite`/`telegram` 依赖，参见 `cmd/agent/main.go:13-15` 与 `CHANGELOG.md:42`）。
- **版本变量**：`cmd/agent/main.go:19` 定义 `var Version = "v2.7.3"`，构建时可通过 `-ldflags "-X 'main.Version=...'"` 覆盖；与 `panel`（`internal/config/config.go:44-45`）不同，`cmd/agent` 未提供 `-v` / `-version` 参数，版本号通过启动日志与 `SyncRequest.AgentVersion` 上报（`cmd/agent/main.go:119,139`）。

### 1.2 交叉编译命令（Linux `amd64` / `arm64`）
依据 `.github/workflows/release.yml:101-135` 的官方 Release 构建契约：

- **Linux amd64**（Bash / CI）：
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X 'main.Version=v2.7.3'" -o xray-agent-linux-amd64 ./cmd/agent
  ```
- **Linux arm64**（Bash / CI）：
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X 'main.Version=v2.7.3'" -o xray-agent-linux-arm64 ./cmd/agent
  ```
- **Windows PowerShell 本地交叉编译**（对齐 `mise.toml` 与 `deploy.ps1:45-49`）：
  ```powershell
  $env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
  mise x -- go build -trimpath -ldflags="-s -w" -o xray-agent ./cmd/agent
  ```

### 1.3 CLI Flags 与环境变量全清单
定义于 `cmd/agent/main.go:38-51`（优先级：CLI flag > 环境变量 > 默认值）：

| CLI Flag | 环境变量 | 默认值 | 必填 | 源码锚点与说明 |
| :--- | :--- | :--- | :---: | :--- |
| `-master-url` | `MASTER_URL` | `""` | **是** | `cmd/agent/main.go:39`；`internal/agent/client.go:32-35` 校验非空，如 `https://panel.example.com` |
| `-token` | `AGENT_TOKEN` | `""` | **是** | `cmd/agent/main.go:40`；`internal/agent/client.go:36-39` 校验非空，64 位 hex 节点 Bearer Token |
| `-xray-bin` | `XRAY_BIN` | `"xray"` | 否 | `cmd/agent/main.go:41`；`internal/agent/config_manager.go:63-65`（依赖 `PATH` 或绝对路径） |
| `-xray-config` | `XRAY_CONFIG` | `"/usr/local/etc/xray/config.json"` | 否 | `cmd/agent/main.go:42`；`internal/agent/config_manager.go:60-62` |
| `-xray-grpc` | `XRAY_GRPC` | `"127.0.0.1:8080"` | 否 | `cmd/agent/main.go:43`；必须与远端编译配置的 `remoteAPIPort = 8080` 一致（`compiler.go:349,407`） |
| `-xray-service` | `XRAY_SERVICE` | `"xray"` | 否 | `cmd/agent/main.go:44`；用于 `systemctl reload-or-restart <service>`（`config_manager.go:298`） |
| `-xray-assets-dir` | `XRAY_ASSETS_DIR` | `"/usr/local/share/xray"` | 否 | `cmd/agent/main.go:45`；`geoip.dat` / `geosite.dat` 目标目录（`geodata_updater.go:60-63`） |
| `-spool-dir` | `SPOOL_DIR` | `"/var/lib/xray-agent"` | 否 | `cmd/agent/main.go:46`；存放 `pending_batch.json`、`quota_state.json`、`sync_state.json` |
| `-interval` | `SYNC_INTERVAL` | `60s` | 否 | `cmd/agent/main.go:28-35,47`；须为合法正 duration；运行时受 Master `SyncIntervalSec` 动态覆盖（`runner.go:283-287`） |
| `-log-level` | `LOG_LEVEL` | `"info"` | 否 | `cmd/agent/main.go:48`（`debug` / `info` / `warn` / `error`） |
| `-log-json` | *(无)* | `false` | 否 | `cmd/agent/main.go:49`；是否输出 JSON 格式结构化日志 |

> 注：没有 `-node-id` 参数；Master 通过 `Authorization: Bearer <token>` 的 SHA-256 哈希反查 `nodes.token_hash` 唯一确定 `nodeID`（`internal/delivery/http/middleware_agent.go:34-48`、`internal/service/node_service.go:653-671`）。

---

## 2. Master (`jp`) ↔ Agent 接入全流程

### 2.1 Master 侧节点创建、Token 签发与内置一键脚本
1. **Web UI 创建节点**：管理员在 Master 面板「远端直连节点 (`NodesView.vue`)」点击「新增节点」，在 `NodeFormDrawer.vue:14-64` 填写必填项 `name`（节点名称）与 `host`（客户端直连用公网 IP/域名），可选 `region`、`desiredState`（默认 `active`）、`enabled`（默认 `true`）及配额阈值。
2. **后端 Token 生成与持久化**：`POST /api/admin/nodes`（`router.go:190` → `handler_node.go:72-92`）调用 `NodeService.CreateNode`（`node_service.go:334-422`），生成 32 字节（64 位 hex）随机明文 Token，仅将 `TokenHash`（SHA-256 hex）与 `TokenPrefix`（前 8 位）写入 SQLite，并返回一次性 `CreateNodeResponse.Token`（`node_service.go:229-238,383-421`）。
3. **前端已内置「一键部署脚本」弹窗**：
   - 创建节点或「轮换 Token」（`POST /api/admin/nodes/:id/rotate-token`，`handler_node.go:145-163`）后，自动打开 `NodeTokenModal.vue`（`NodesView.vue:692-702,727-738`）。
   - 弹窗默认以 `window.location.origin` 作为 `masterUrl`，请求 `GET /api/admin/nodes/:id/install-script?masterUrl=...`（`NodeTokenModal.vue:207,221-224`、`handler_node.go:184-227`）。
   - 后端 `NodeService.RenderInstallScript`（`node_service.go:689-783`）生成 systemd 安装脚本（包含 `uname -m` 架构识别、从 GitHub Release 下载 `xray-agent-linux-{amd64,arm64}`、写入 `/etc/xray-agent/agent.env` 与 `/etc/systemd/system/xray-agent.service`）。出于安全原则，后端脚本模板只含占位符 `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"`（`node_service.go:705`），而前端 `NodeTokenModal.vue:192-200` 在创建/轮换弹窗时会在浏览器本地把一次性明文 Token 内嵌替换进去；若之后从 `NodeOpsDrawer.vue:489-492` 的「查看安装脚本」再次打开，需手动填入之前保存的 Token 或重新轮换。

### 2.2 通信方向、协议与端口需求
- **通信方向**：**纯 Agent 主动出站轮询 Master**（`internal/agent/client.go:51-136`、`runner.go:266,658-673`）。Master **不会**主动连接 Agent，GCP 节点**无需**向 Master 暴露任何 Agent 控制端口或 gRPC 端口。
- **传输协议与路由**：
  - 协议：**HTTP/HTTPS + JSON + Bearer Token**（`Authorization: Bearer <token>`，`client.go:67`、`middleware_agent.go:23-34`），非 gRPC、非 WebSocket、非 mTLS。
  - 端点（`router.go:86-93`、`handler_agent.go:22-101`）：
    - `POST /api/agent/sync`：心跳、主机指标、流量批次上报、期望状态/完整配置/用户清单下发、异步命令下发；
    - `POST /api/agent/commands/:id/result`：异步命令（`log_tail` / `geodata_update`）完成后的即时结果回传。
  - Master 反代兼容：`deploy/nginx-sample.conf:111-122` 中 `location /` 已代理所有 `/api/*` 至 `127.0.0.1:9000`；请求体全局上限 `10MB`（`router.go:58`），Agent 单次 HTTP 超时 `15s`（`client.go:17`）。由于 Agent 使用 Go 默认 `http.Client`（`client.go:40-42`），若 `MASTER_URL` 使用 `https://` 则 Master 必须具备可信 TLS 证书（自签证书会校验证书失败）。
- **GCP 防火墙端口需求清单**：
  - **GCP 出站**：允许访问 `MASTER_URL` 对应端口（通常为 `443/TCP` 或面板直连 `9000/TCP`）；若需使用远程 `geodata_update`，需允许出站 HTTPS 访问规则库源（`internal/agent/geodata_updater.go:64-71`）。
  - **GCP 入站**：Agent 控制面 **0 个额外入站端口**（只需 SSH 与你自己的 Xray 业务代理端口）；本机 gRPC `127.0.0.1:8080` 仅限回环通信（`grpc_client.go:60-62`、`compiler.go:110-124`），严禁暴露公网。

### 2.3 Agent 对 Xray 参数的最小依赖与容错行为
（遵照用户要求不展开 Xray 配置教程，仅记录启动/运行期最小契约与容错：）
1. **启动阶段完全容错**：`cmd/agent/main.go:79-148` 仅初始化本地 gRPC 客户端（惰性建连，`grpc_client.go:36-76`）。若启动时 `/usr/local/etc/xray/config.json` 尚不存在，`HashCurrentConfig()` 返回 `"missing"`（`config_manager.go:180-182`）；若 `xray` 不可执行或 gRPC 未就绪，`DetectXrayVersion` 返回 `""`（`config_manager.go:272-276`）、`CollectTrafficStats` 记录 Warn 后跳过（`runner.go:215-218`），**Agent 仍能正常向 Master 发起心跳并报告自身在线**。
2. **配置落地阶段强依赖**：当 Master 返回 `ConfigJSON` 时（哪怕该节点尚未绑定任何代理入站，Master 也会编译出一个包含 `127.0.0.1:8080` `api` 入站和 `direct`/`block` 出站的最小有效 JSON，`compiler.go:396-416`），Agent 的 `ConfigManager.Apply`（`config_manager.go:90-171,285-329`）依次要求：
   - 本机存在可执行的 `xray`（`--xray-bin`）以通过 `xray run -test -c <tmp>`；
   - 本机存在可写的目标配置目录与日志路径（编译模板默认引用 `/var/log/xray/access.log` 与 `/var/log/xray/error.log`，`compiler.go:81-84`）；
   - 本机存在 systemd 单元 `xray.service`（`--xray-service`）以执行 `systemctl reload-or-restart xray`；
   - 重载后 `3s` 内本机 `127.0.0.1:8080`（`--xray-grpc`）gRPC `StatsService` 探活成功。任何一步失败会自动回滚 `.bak` 并将报错通过 `lastError` 回传 Master（`runner.go:333-336`）。

### 2.4 状态存储、Systemd 运行、日志、心跳与升级
- **本地状态存储（`--spool-dir /var/lib/xray-agent`）**：
  - `pending_batch.json`（`0600`）：断网/重启时的未确认流量批次缓存，Master 通过 `(node_id, batch_id)` 唯一索引保证幂等去重（`spool.go:15-144`、`node_repo.go:198-214`）。
  - `sync_state.json`（`0600`）：记录最近已收敛的 `configHash` / `usersHash` / `fullConfigHash` / `desiredUsers` / `circuitBroken`；`xray-agent` 重启时若磁盘 `config.json` 与之一致则跳过无意义 Xray 重载（`sync_state.go:14-26`、`runner.go:849-893`）。
  - `quota_state.json`（`0600`）：缓存 Master 配额策略并在断网期间按物理网卡增量记账，超限触发自主静默熔断（`quota_store.go:16-31`、`runner.go:780-824`）。
- **Systemd 运行要求**（`node_service.go:750-779`）：
  - `/etc/xray-agent/agent.env`（`0600`）存放 `MASTER_URL` 与 `AGENT_TOKEN`，通过 `EnvironmentFile=` 注入，避免在 `ps` 命令行暴露 `--token`（`node_service_test.go:1268-1271`）。
  - Unit 名为 `xray-agent.service`（注意：`LogCollector` 白名单硬编码仅允许 `"xray"` 与 `"xray-agent"`，`internal/agent/log_collector.go:36`、`node_service.go:1020-1022`）。
  - 以默认 `root` 运行（需写 `/usr/local/etc/xray/config.json`、`/usr/local/share/xray/` 及执行 `systemctl reload-or-restart xray`）。
- **日志排查**：
  - 本机：`journalctl -u xray-agent -f`、`systemctl status xray-agent`；
  - Master 面板：`NodesView` → 运维抽屉「快速日志」Tab 或 `LogsView` 选择 `xray` / `xray-agent` 异步采集最近 100 行（`NodeOpsDrawer.vue:245-287`、`log_collector.go:35-61`），节点卡片同步状态亦直接展示 `lastSyncError`。
- **心跳、重连与在线判定**：
  - 默认周期 `60s`（`runner.go:18,183-203`），Master 在线阈值 `LastSeenAt <= 180s`（3 个心跳周期，`domain/node.go:148,212`）。
  - 单次请求失败仅记录 `lastError` 并保留 `pending_batch.json`，**进程不退出**，下个 tick 自动重连并补传（`runner.go:179-194,266-281`）。
- **升级方式**：重新执行安装脚本（先 `systemctl stop xray-agent`，再 `install -m 0755` 原子替换 `/usr/local/bin/xray-agent` 并 `systemctl enable --now xray-agent`，`node_service.go:745-748,779`），或手动上传新二进制覆盖后重启服务。

---

## 3. 现有部署脚本与文档缺口审计

### 3.1 仓库现有文档与脚本盘点
- `README.md:46-108`：仅介绍 Master (`panel`) 的 `deploy/install.sh` 一键部署及 `panel` CLI 参数/环境变量，**完全未提及** `cmd/agent`、`xray-agent`、Agent CLI 参数、交叉编译命令及多节点部署流程。
- `deploy/`（`git ls-files deploy/`）：仅包含 Master 侧脚本（`deploy/install.sh`、`deploy/panel.service`、`deploy/99-xray-panel-sysctl.conf`、`deploy/nginx-sample.conf`）及 Cloudflare 边缘脚本，**无** `install-agent.sh` 或 `xray-agent.service`。
- `deploy.sh` / `deploy.ps1`：均在 `.gitignore:63,66` 中（本地私有辅助脚本），且目前仅构建和分发 `panel`，不支持构建/分发 `xray-agent`。
- `docs/ARCHITECTURE_AND_LOGIC.md:1-253`：文档仍停留在 v2.6.0 单机+中转架构图解，尚未覆盖 v2.7.0~v2.7.3 的 `xray-agent` 架构与多节点同步。

### 3.2 核心结论：目前能否仅靠现有文档/UI 把新 GCP 部署为 Agent？缺什么？
1. **仅靠仓库文档 / `deploy/`**：**不能**。仓库没有任何 Agent 部署文档、参数表或独立安装脚本。
2. **通过 `jp` Master Web UI（「新增节点」弹窗生成脚本）**：**核心闭环已存在，但直接拿到 GCP 执行存在 5 个真实缺口/卡点**：
   - **缺口 1（GCP 非 root 权限卡点）**：GCP 默认 SSH 登录用户为普通用户（如 `ubuntu`/`debian`/OS Login），而 `RenderInstallScript`（`node_service.go:699-780`）既**没有 `EUID` root 校验**，UI 文案也未提示必须用 `sudo bash` 执行，直接粘贴执行会在 `install -m 0755 ... /usr/local/bin/xray-agent` 报错退出。
   - **缺口 2（GitHub Release 拉取单点依赖 & 无离线兜底）**：`RenderInstallScript` 默认下载 `https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}`（`node_service.go:726`）。若最新版本尚未打 tag 发布（`.github/workflows/release.yml:4-6` 仅在 push `v*` tag 时触发）、或目标机访问 GitHub 异常、或需要部署本地未发布编译产物，缺乏离线/SCP 本地二进制（如 `/tmp/xray-agent`）兜底与指南。
   - **缺口 3（宿主机 Xray 最小前置依赖无指引）**：脚本在未检测到 `xray` 时只打印 `[WARN]` 并继续启动 `xray-agent`（`node_service.go:722-724`）；若 GCP 机器尚未安装 `xray` 与 `xray.service`（或无 `/var/log/xray/`），Agent 虽能心跳上线，但配置同步会在 `xray run -test` 或 `systemctl reload-or-restart xray` 时持续报 `同步失败 (error)`。
   - **缺口 4（`MASTER_URL` 浏览器来源陷阱）**：`NodeTokenModal.vue:221-223` 自动以 `window.location.origin` 预填 `MASTER_URL`。若管理员通过内网 IP、明文 HTTP 或本地 SSH 端口转发（`http://127.0.0.1:9000`）访问面板，生成脚本里的 `MASTER_URL` 将不可被 GCP 公网访问，或通过明文 HTTP 传输 Bearer Token。
   - **缺口 5（GCP 防火墙/端口模型未说明）**：缺少明确提示“Agent 仅主动出站连接 Master，GCP 无需开放 8080/9000 入站端口，仅需开放业务代理端口”。

### 3.3 最小高价值改动范围建议
1. **文档补全（必选，零运行时风险）**：
   - 更新 `README.md`（或新增 `docs/AGENT_DEPLOY.md` 并在 `README.md` 链接）：补齐 Master UI 创建节点与复制脚本步骤（强调公网 HTTPS `MASTER_URL` 与 `sudo bash`）、Agent 宿主机前置条件（需已装有可执行 `xray` 与 `xray.service`，不展开 Xray 业务配置）、`xray-agent` 参数与环境变量表、本地交叉编译 + `scp` 离线安装方法、以及 `journalctl -u xray-agent` 排障。
2. **脚本与后端模板小改（高价值、低风险，需对齐现有单测契约）**：
   - 新增独立可复用脚本 `deploy/install-agent.sh`（对齐 `deploy/install.sh` 风格：`EUID` root 检查、支持本地 `./xray-agent` 或 `/tmp/xray-agent` 优先安装、找不到时按 `uname -m` 从 `AGENT_BIN_URL` 下载、读取环境变量 `MASTER_URL` / `AGENT_TOKEN` 写入 `0600` `/etc/xray-agent/agent.env` 并托管 `xray-agent.service`）。
   - 优化 `internal/service/node_service.go` 中的 `RenderInstallScript`（`699-780`）：增加 root 检查提示（`[ "$(id -u)" -ne 0 ]`）、可选支持本地已上传二进制 fallback（需保持 `TestNodeService_RenderInstallScript_EnvFileAndInterval` 的全部断言通过，见 `node_service_test.go:1222-1277`）。
   - 前端 `NodeTokenModal.vue`（可选微调）：提示使用 `sudo bash` 执行及 `MASTER_URL` 需为远端节点公网可达的 HTTPS 地址。

---

## 4. 候选 `.trellis/spec/` 路径清单

经扫描 `.trellis/spec/` 全量文件，与本次任务（后端 `NodeService` / 文档 / 部署脚本 / 可选前端引导）相关的 spec 路径如下：

| 路径 | 职责与适用场景 |
| :--- | :--- |
| `.trellis/spec/backend/index.md` | 后端分层架构总览、版本号四处同步契约（`README.md`/`CHANGELOG.md`/`main.go`/`web/package.json`）与子规范索引 |
| `.trellis/spec/backend/directory-structure.md` | 后端 `internal/` 目录分工与层间调用边界（修改 `internal/service/node_service.go` 时参考） |
| `.trellis/spec/backend/error-handling.md` | 显式错误传播与 `domain.Err*` HTTP 映射契约 |
| `.trellis/spec/backend/quality-guidelines.md` | 并发安全、KISS 规范与反过度设计约束 |
| `.trellis/spec/backend/logging-guidelines.md` | 结构化日志与敏感凭据防泄露规范（Agent/Token 脚本不得泄露明文 Token） |
| `.trellis/spec/frontend/index.md` | 前端技术栈与构建嵌入契约（若修改 `NodeTokenModal.vue` 需跑通 `npm run build`） |
| `.trellis/spec/frontend/components-and-routing.md` | Vue SFC 组件约定与样式/图标规范 |
| `.trellis/spec/frontend/quality-guidelines.md` | 前端 `typecheck` / `build` 门禁与敏感凭据安全防线 |
| `.trellis/spec/guides/cross-layer-thinking-guide.md` | 跨层契约检查（如修改 `RenderInstallScript` 时须核对 `NodeTokenModal.vue:196-199` 的字符串替换契约与 `node_service_test.go:1239-1276` 单测断言） |
| `.trellis/spec/guides/review-3pass-guide.md` | 提交前 3-Pass 门禁检查指南 |

---

## 5. 待协调者向用户确认的产品/范围决策点

1. **交付范围确认**：本次需要做到哪一步？
   - **仅方案 A（纯文档）**：更新 `README.md` / 新增 `docs/AGENT_DEPLOY.md`，让用户可依据现有 Master UI 或手工交叉编译顺利把 GCP 部署为 Agent；
   - **方案 B（文档 + 脚本 + 模板优化，推荐）**：新增 `deploy/install-agent.sh` + 优化 `RenderInstallScript`（root 校验、支持 `/tmp/xray-agent` 本地二进制兜底）+ 更新 `README.md`（及可选 `NodeTokenModal.vue` 提示）。
2. **二进制获取方式偏好**：新 GCP 节点希望通过 **GitHub Release 在线自动拉取**（`RenderInstallScript` 默认行为），还是在本机交叉编译 `./cmd/agent` 后通过 `scp` 传到 GCP 再用本地脚本安装？
3. **当前 Master 接入状态**：`jp` Master 是否已配置好可从公网访问的 **HTTPS 域名入口**（如 Nginx 反代 443）？若目前仅用 HTTP/IP 端口访问，脚本及文档需针对性说明风险或配置方式。
