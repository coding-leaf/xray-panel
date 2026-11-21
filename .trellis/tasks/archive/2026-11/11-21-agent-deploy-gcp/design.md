# Technical Design: Agent 节点部署指南与脚本完善

## 1. Architecture & Boundaries
本次改造保持现有 Master (`panel`) 与 Thin Agent (`xray-agent`) 的控制面/数据面解耦架构，**不修改**任何公共 API 路由、请求/响应字段、数据库 Schema 或 Agent 同步协议，仅在以下 4 个边界做最小高价值增强：

1. **文档层 (`README.md`)**：补全 Master 编译前置说明 + 新增 Agent 部署、参数与排障说明。
2. **运维脚本层 (`deploy/install-agent.sh`)**：新增无外部依赖的 Bash Systemd 安装/升级脚本，对齐 `deploy/install.sh` 风格并与 `RenderInstallScript` 共享一致的下载源、运行目录、权限与 systemd 契约。
3. **后端模板层 (`internal/service/node_service.go` + `node_service_test.go`)**：在 `NodeService.RenderInstallScript` 增加 root 权限前置校验并补充单测。
4. **前端展示层 (`web/src/views/nodes/components/NodeTokenModal.vue`)**：补充非 root `sudo bash` 与公网 `MASTER_URL` 简明提示。

## 2. Cross-Layer Contracts & Compatibility

### 2.1 Token 注入与安全契约（Critical）
- 后端 `RenderInstallScript` 必须**逐字节保留**占位符行：
  `NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"`
- 原因：前端 `NodeTokenModal.vue:196-199` 依赖精确字符串匹配替换一次性明文 Token；后端单测 `TestNodeService_RenderInstallScript_EnvFileAndInterval`（`node_service_test.go:1262-1264`）断言后端输出绝不泄露明文 Token。
- `xray-agent.service` 必须通过 `EnvironmentFile=/etc/xray-agent/agent.env`（文件权限 `0600`）注入 `MASTER_URL` 与 `AGENT_TOKEN`，严禁在 `ExecStart` 命令行传递 `--token`。

### 2.2 纯净的 Release 下载契约
- `RenderInstallScript` 与 `deploy/install-agent.sh` 均统一使用：
  `AGENT_BIN_URL="${AGENT_BIN_URL:-https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}}"`
- 经 `curl -fsSL` / `wget -qO` 下载到 `mktemp /tmp/xray-agent.XXXXXX`（带 `trap 'rm -f "$TMP_BIN"' EXIT`），校验 `[ -s "$TMP_BIN" ]` 后停止旧服务并 `install -m 0755 "$TMP_BIN" /usr/local/bin/xray-agent`。
- **不引入本地文件扫描兜底**：所有普通用户都应直接从官方 Release（或显式自定义的 `AGENT_BIN_URL` 镜像）拉取，避免隐式命中宿主机残留旧二进制文件导致版本漂移。

### 2.3 升级幂等性（`deploy/install-agent.sh`）
- 首次安装（`/etc/xray-agent/agent.env` 不存在）：`MASTER_URL` 与 `AGENT_TOKEN` 均必填，缺失时报错退出并打印用法示例。
- 升级重跑（`/etc/xray-agent/agent.env` 已存在）：
  - 若环境变量同时传入了非空 `MASTER_URL` 和 `AGENT_TOKEN`，则安全覆写 `agent.env`（`umask 077` + `chmod 0600`）；
  - 若均未传入，则保留现有 `/etc/xray-agent/agent.env`，仅原子更新二进制与服务定义；
  - 若只传了其中一个，报错提示需要同时提供或都不提供（避免配置半覆盖）。

### 2.4 Systemd Unit 命名契约
- Service 名称固定为 `xray-agent.service`，与 `internal/agent/log_collector.go:36` 及 `internal/service/node_service.go:1020-1022` 中的远程日志采集白名单（`xray`、`xray-agent`）保持一致。
- 默认 `--interval 60s`、`--spool-dir /var/lib/xray-agent`，与现有单测及 Master 在线阈值 `180s`（3 个心跳周期）保持一致。

## 3. Trade-offs & Decisions
- **保持纯 Release 下载（拒绝隐式本地兜底）**：普通用户新机上不存在本地预置二进制；若需要镜像源或自定义二进制 URL，已有 `AGENT_BIN_URL` 环境变量即可覆盖，零额外复杂度。
- **不新增独立 `docs/AGENT_DEPLOY.md`**：遵循 KISS，直接集中维护在 `README.md`，避免文档分散漂移。
- **不改动 Xray 安装与配置流程**：明确遵守用户边界约束，仅保留原有的 `xray` 二进制缺失 `[WARN]` 提示与一行最小宿主机前提说明。

## 4. Rollback Considerations
- 所有改动均位于文档、独立新增脚本、字符串模板及 UI 提示文案，无数据库迁移、无 API 破坏性变更，回滚仅需 `git revert` 对应文件即可零副作用恢复。
