# 技术方案设计 (Design) - 协议契约准确性与配置语义收敛

## 1. 架构边界与受影响模块 (Boundaries & Scope)

本次改造覆盖三层结构：
1. **Presentation (Web 前端)**：
   - `web/src/views/inbounds/types.ts` & `InboundFormDrawer.vue`
   - `web/src/views/outbounds/types.ts` & `OutboundFormDrawer.vue`
2. **Adapter (Xray 配置编译与契约适配)**：
   - `internal/adapter/xray/compiler.go`
   - `internal/adapter/xray/protocol_adapter.go`
3. **Domain & Service (监控与评估)**：
   - `internal/domain/reality_evaluator.go`
   - `internal/service/reality_monitor_service.go`

---

## 2. 核心契约与技术决策 (Contracts & Decisions)

### 2.1 契约清理：移除 `allowInsecure`
- **事实**：Xray-core TLS/streamSettings 中移除了 `allowInsecure`。
- **改动**：
  - 从 `OutboundFormData` 中移除 `tlsAllowInsecure`；
  - `buildStreamSettingsJSON` 移除 `allowInsecure: form.tlsAllowInsecure === true`；
  - `OutboundFormDrawer.vue` 移除复选框 UI。

### 2.2 协议套件收敛：SS2022 Multi-User
- **事实**：SS2022 规范中，multi-user (sub-keys 模式) 仅支持：
  - `2022-blake3-aes-128-gcm`
  - `2022-blake3-aes-256-gcm`
- **改动**：
  - 前端 `InboundFormDrawer.vue`：加密下拉项移除 `2022-blake3-chacha20-poly1305`。
  - 后端 `Shadowsocks2022Adapter.CompileClients`：
    ```go
    method := accessor.ResolveShadowsocksMethod()
    switch method {
    case "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm":
        // 允许
    default:
        return nil, fmt.Errorf("%w: shadowsocks-2022 multi-user mode only supports 2022-blake3-aes-128-gcm and 2022-blake3-aes-256-gcm, got %q", domain.ErrInvalidInput, method)
    }
    ```

### 2.3 REALITY 拒绝替用户猜配置 (No Fallback, Strict Validation)
- **事实**：隐式兜底 `example.com` 和 `dest ↔ serverNames` 互推导致用户即使漏填也能生成“合法但无效”的配置。
- **改动**：
  - **后端 `XrayCompiler.compileInbound`**：
    当 `streamSettings.Security == "reality"` 时：
    1. 校验 `r.Dest != ""` 且必须包含端口（若仅写 host 则需明确要求 `host:port` 格式）；
    2. 校验 `len(r.ServerNames) > 0` 且元素非空；
    3. 校验 `r.PrivateKey != ""`；
    4. 移除所有互推（`r.Dest` 推导 `r.ServerNames`，或反之）和兜底逻辑。
  - **后端 `reality_evaluator.go` & `reality_monitor_service.go`**：
    删除 `if dest == "" && len(snList) > 0 { dest = snList[0] }` 和 `if len(serverNames) == 0 && dest != "" { serverNames = []string{dest} }`。真实是什么就输出什么，不猜。
  - **前端 `types.ts` & `FormDrawer.vue`**：
    1. 表单默认值 `realityTarget`、`realityServerNames` 默认为空；
    2. 在 `onSave` 提交前校验：若 `security === 'reality'`，若 `realityTarget`、`realityServerNames` 或 `realityPrivateKey` 为空，直接 `toast.error` 拦截。

### 2.4 文案与策略描述矫正
- **修改位置**：
  - `InboundFormDrawer.vue`：
    - Socks / HTTP / dokodemo-door：“该协议在面板中作为应用层原生代理/端口转发管理，目前面板策略暂未开放传输层流控与 TLS/REALITY 包装”。
  - `OutboundFormDrawer.vue`：
    - 针对 VMess / Shadowsocks 不支持 REALITY 的选项控制，提示文本：“面板策略当前仅开放 VLESS / Trojan 协议搭配 REALITY 伪装出站”。

---

## 3. 回归与兼容性评估 (Compatibility & Risks)

- **现有单测兼容性**：
  - 既有测试用例中如果有将 `serverNames` 依赖于 `dest` 自动推导或者依赖 `example.com` 兜底的测试（例如 `compiler_test.go`），需更新为显式提供完整有效的 REALITY 配置。
- **向后兼容**：
  - 数据库中已保存的合法 Inbound 包含完整字段不受影响；
  - 历史不完整/错误配置将在下次保存或重载时触发拦截，引导用户填写正确参数。
