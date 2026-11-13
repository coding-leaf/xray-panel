# Design: Fix VLESS Vision Override & Flow Consistency

## 1. Xray Adapter (`internal/adapter/xray/stream_accessor.go` & `protocol_adapter.go`)
- **`StreamAccessor.SupportsVisionFlow(protocol string) bool`**:
  - Separate transport capability check from inbound default policy. Returns `true` iff `strings.EqualFold(protocol, "vless")`, network is `"tcp"`, and security is `"tls"` or `"reality"`.
- **`StreamAccessor.ResolveVisionFlow(protocol string) string`**:
  - First check `SupportsVisionFlow(protocol)`; if false, return `""`.
  - Read trimmed `settings.flow`: `"none"` -> `""`; non-empty (`xtls-rprx-vision`, `xtls-rprx-vision-udp443`, etc.) -> trimmed value; empty -> `"xtls-rprx-vision"`.
- **`resolveVLESSClientFlow(inbound, user, accessor)`**:
  1. If `!accessor.SupportsVisionFlow(inbound.Protocol)` -> return `""`.
  2. Trim `user.Flow`; if `strings.EqualFold(userFlow, "none")` -> return `""`.
  3. If `userFlow != ""` -> return `userFlow` (explicit user Vision overrides Inbound `"none"`).
  4. Otherwise return `accessor.ResolveVisionFlow(inbound.Protocol)` (Inbound default).
- **`VLESSAdapter.DecorateSettings`**:
  - Unconditionally `delete(settingsMap, "flow")` when compiling `config.json` (DB `inbound.SettingsJSON` retains Inbound default policy intact).

## 2. Subscription Node Converter (`internal/protocol/node_converter.go`)
- In `InboundToNodeConfigWithDefaultFP` (`L331-353`):
  - Only apply flow when transport supports Vision (`net == "tcp"` and `security == "tls" || security == "reality"`).
  - Evaluate trimmed `user.Flow` **before** `customFlow`:
    - `strings.EqualFold(uFlow, "none")` -> `flow = ""`
    - `uFlow != ""` -> `flow = uFlow`
    - `strings.EqualFold(cFlow, "none")` -> `flow = ""`
    - `cFlow != ""` -> `flow = cFlow`
    - otherwise -> `flow = "xtls-rprx-vision"`

## 3. Startup & Raw Config Sync (`internal/service/config_service.go`)
- In `syncFromRawJSONLocked`:
  - **Inbound `settings.flow` preservation**:
    - When `strings.TrimSpace(rawFlow) == ""` and `existing != nil`: unmarshal `existing.SettingsJSON` and preserve `prevSettings["flow"]` verbatim (`settingsMap["flow"] = pf` if present, else keep absent). Do **not** infer `"none"` from `!hasClientWithFlow` on an already-known DB inbound.
    - Only when `existing == nil` (importing an unknown inbound from disk) and `strings.TrimSpace(rawFlow) == ""`: apply `if hasAnyClient && !hasClientWithFlow { settingsMap["flow"] = "none" }`.
    - When `rawFlow != ""` (explicit raw config edit): keep `rawFlow`.
  - **Panel-only `StreamSettings` field preservation (`existing != nil`)**:
    - Merge panel-only client hints from `existing.StreamSettings` into incoming `ib["streamSettings"]` before marshaling `streamBytes`:
      - `tlsSettings.fingerprint` (if missing in incoming `tlsSettings`)
      - `realitySettings.{publicKey, fingerprint, spiderX}` (if missing in incoming `realitySettings` and `privateKey` matches or wasn't rotated).

## 4. Frontend Alignment (`web/src/views/inbounds/` & `users/`)
- `web/src/views/inbounds/composables/useInboundList.ts` (`getNodeFlow`):
  - Parse `streamSettings`: if network != `'tcp'` or security not in `('tls', 'reality')`, return `'none'`.
  - Otherwise return `s.flow || 'xtls-rprx-vision'` (so `'none'` stays `'none'`, unset/`''` displays `'xtls-rprx-vision'`).
- `web/src/views/inbounds/components/InboundFormDrawer.vue` (`initFormData` & UI hint):
  - Fix JS precedence: `((stream.network || (inb.protocol === 'vless' ? 'tcp' : '')) === 'tcp')`.
  - Check `!isTcp || !isTlsOrReality` before reading `settings.flow` so non-TCP/non-TLS inbounds initialize `vlessFlow = 'none'`.
  - Clarify helper copy that user-level explicit `flow` overrides the inbound default.
- `web/src/views/inbounds/types.ts` (`sanitizeInboundPayload`):
  - When `vlessFlow === ''` on TCP+(TLS|Reality), default to `'xtls-rprx-vision'`.
- `web/src/views/users/components/UserFormDrawer.vue`:
  - Add `xtls-rprx-vision-udp443` option and clarify `用户显式设置优先于节点默认（仅 VLESS + TCP + TLS/REALITY 生效）`.
