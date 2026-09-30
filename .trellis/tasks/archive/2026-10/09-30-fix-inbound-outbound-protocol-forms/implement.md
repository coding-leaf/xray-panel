# Execution Plan: 前端入站出站协议匹配与参数表单规范化重构

## 步骤清单与执行顺序

### Step 1: 完善出站类型定义与清洗逻辑 (`web/src/views/outbounds/types.ts`)
- [ ] 扩展出站协议支持 `dns`；
- [ ] 修复 `buildStreamSettingsJSON`，将 `socks`、`http`、`dns` 纳入无传输安全层列表，杜绝输出多余的 Reality/XHTTP JSON；
- [ ] 确保 `populateOutboundForm` 和 `buildSettingsJSON` 对 `dns`、`socks`、`http` 等协议解析无异常。

### Step 2: 改造出站表单组件 (`web/src/views/outbounds/components/OutboundFormDrawer.vue`)
- [ ] 增加 `dns` 出站协议选项及友好提示；
- [ ] 针对 `vmess` 和 `shadowsocks` 限制安全协议选项（不可选择 `reality`）；
- [ ] 当切换协议至非代理或无安全层协议时，自动清洗 streamNetwork 与 streamSecurity。

### Step 3: 完善入站类型定义与清洗逻辑 (`web/src/views/inbounds/types.ts`)
- [ ] 在 `InboundFormData` 中补充 `ssMethod`、`ssPassword`；
- [ ] 在 `getDefaultInboundFormData` 中提供合理的默认值；
- [ ] 重构 `buildSettingsJSON`：
  - Trojan 客户端格式修正为 `{ password: user.uuid, email: user.email, level: 0 }`；
  - Shadowsocks 支持按指定 `ssMethod` 和选中用户生成 clients，或单用户密码结构；
  - VLESS 客户端严格保留 flow，非 VLESS 协议禁止携带 flow；
- [ ] 修复 `buildStreamSettingsJSON`：
  - 对 `socks`、`http`、`dokodemo-door` 直接返回 `""`；
- [ ] 完善 `sanitizeInboundPayload`：
  - 严格限制不同协议下的网络传输方式与安全加密方式。

### Step 4: 补全入站表单 UI 控件与联动 (`web/src/views/inbounds/components/InboundFormDrawer.vue`)
- [ ] 新增 Socks 专属配置区块（认证方式、用户列表/单用户、UDP 转发开关）；
- [ ] 新增 HTTP 专属配置区块（用户名、密码认证）；
- [ ] 新增 dokodemo-door 专属配置区块（转发目标 IP/域名、目标端口、转发网络类型）；
- [ ] 新增 Shadowsocks 专属配置区块（加密方式下拉：2022-blake3 系列、AEAD 系列等；单用户密码或用户关联说明）；
- [ ] 协议切换联动：
  - 切换为 Socks/HTTP/dokodemo-door 时，隐藏传输层与安全层选择（或置灰提示为原生 TCP/UDP）；
  - 切换为 VMess 时，安全协议禁止选 REALITY；
  - 传输层与安全层变动时，根据合法组合自动修正流控及附属参数；
- [ ] 编辑已有节点时的数据回填与解析还原（针对 SS/Socks/HTTP/dokodemo-door）。

### Step 5: 详情抽屉检视同步 (`InboundDetailDrawer.vue` & `OutboundDetailDrawer.vue`)
- [ ] 确保详情抽屉在查看 Socks、HTTP、dokodemo-door、Shadowsocks 节点时能够展示其专属配置要点，无运行时异常。

### Step 6: 门禁检查与全量测试
- [ ] 运行前端类型检查与打包：`cd web && mise x -- npm run build`；
- [ ] 运行后端单元测试与检查：`mise x -- go test ./...` 与 `mise x -- go vet ./...`；
- [ ] 验证所有协议生成的 JSON 符合 Xray 官方格式契约。
