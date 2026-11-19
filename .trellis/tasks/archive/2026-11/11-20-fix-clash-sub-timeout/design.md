# Technical Design: 修复 Clash REALITY ML-KEM 握手超时与模板卫生

## 1. Exact Code Changes

### 1.1 `internal/protocol/helpers.go` (`AttachClashTLS`)
在 `strings.EqualFold(node.Security, "reality")` 分支构造 `realityOpts` 时：
```go
if len(realityOpts) > 0 {
    realityOpts["support-x25519mlkem768"] = true
    proxy["reality-opts"] = realityOpts
}
```
- **兼容性**：
  - Mihomo / Clash Verge Rev（支持 Xray `26.9.8+` / `26.11.9`）：开启 `X25519MLKEM768` 混合后量子密钥交换，与 `client-fingerprint: chrome` 配合通过 Xray REALITY 严格校验；
  - 旧版 Clash / Mihomo：YAML 反序列化忽略未知键，无破坏性；
  - 非 REALITY（`tls` / `none`）节点：不进入该分支，零影响。

### 1.2 `internal/sub/clash.go`
1. **识别虚拟信息节点并从 `AUTO` 测速组排除**：
   - 增加轻量纯判断辅助函数 `isVirtualInfoNode(n *protocol.NodeConfig) bool`：
     ```go
     func isVirtualInfoNode(n *protocol.NodeConfig) bool {
         return n != nil &&
             n.Address == "127.0.0.1" &&
             n.Port == 10000 &&
             n.UUID == "00000000-0000-0000-0000-000000000000"
     }
     ```
   - 遍历 `nodes` 构建 `proxies` 时维护两份名称列表：
     - `allProxyNames`：所有成功转换的节点（含虚拟信息节点），供 `PROXIES`（`select`）使用；
     - `realProxyNames`：仅真实代理节点（`!isVirtualInfoNode(n)`），供 `AUTO`（`url-test`）使用；
   - 若 `len(realProxyNames) == 0`（无真实可用节点），按现有 `DIRECT` 兜底逻辑安全处理（不生成空 `url-test` 组）。
2. **移除重复 `Port` 监听声明**：
   - `ClashConfig` 仅赋值 `MixedPort: mixedPort`，不设置 `Port: mixedPort`（`Port` 字段已有 `yaml:"port,omitempty"`，留零值自动省略），彻底消除原生 Mihomo CLI 监听 `7890` 端口冲突。

## 2. Fail-Repro First Testing Plan
1. `internal/protocol/protocol_test.go`：先新增测试断言 VLESS+REALITY `ToClash()` 包含 `reality-opts["support-x25519mlkem768"] == true`（红灯复现），且普通 TLS 节点无 `reality-opts`。
2. `internal/sub/clash_test.go`：先新增测试断言：
   - REALITY 节点导出 YAML 含 `support-x25519mlkem768: true`；
   - 存在虚拟信息节点 + 真实节点时，`PROXIES` 包含全部节点、`AUTO` 仅包含真实节点（不含虚拟信息节点）；
   - 仅输出 `mixed-port: 7890` 且无 `port: 7890`。
3. `internal/delivery/http/handler_sub_test.go`：验证 Clash Verge UA 拉取默认 `/sub/:token` 返回的 YAML 包含 `support-x25519mlkem768: true`。

## 3. Rollout & Verification
- 仅改动后端 Master 订阅导出层，无需重新部署 3 台远端 Agent（远端 Agent 仅负责跑 Xray 与同步，不参与订阅生成）；
- 修复合并验证通过后，重新交叉编译 `panel`（`linux/amd64`）备份替换 `vps-jp` 并拉取本地脱敏 YAML 确认 `support-x25519mlkem768: true` 生效，最后由用户在 Clash Verge 刷新订阅实测。
