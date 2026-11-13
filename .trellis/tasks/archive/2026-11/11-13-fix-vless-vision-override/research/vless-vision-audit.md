# VLESS Vision Override Bug — Read-Only Audit

## 1. Root Causes

### 1.1 Top-level `inbounds[].settings.flow` retained in `config.json` + omitted client `flow` (`omitempty`)
1. **`VLESSAdapter.DecorateSettings` preserves `"flow": "xtls-rprx-vision"` at inbound level** ([`protocol_adapter.go:179-191`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/protocol_adapter.go#L179-L191)):
   - `compileInbound` ([`compiler.go:370-387`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler.go#L370-L387)) unmarshals `inb.SettingsJSON` into `settingsMap`, populates `settingsMap["clients"]` via `CompileClients`, and calls `DecorateSettings`.
   - `DecorateSettings` only deletes `settingsMap["flow"]` when `!strings.EqualFold(flow, "xtls-rprx-vision")` (`L186-190`). When inbound default flow is `"xtls-rprx-vision"`, compiled `/usr/local/etc/xray/config.json` keeps top-level `inbounds[].settings.flow = "xtls-rprx-vision"`.
   - Note: This check also deletes `"xtls-rprx-vision-udp443"` if configured at inbound level (whereas outbound compilation at [`compiler.go:442`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler.go#L442) and frontend `InboundFormDrawer.vue:158` allow it).
2. **Client `flow=""` is omitted from JSON via `omitempty`** ([`schema.go:52`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/schema.go#L52)):
   - `XrayClient.Flow` is tagged `json:"flow,omitempty"`.
   - For a user with `user.Flow == "none"`, `resolveVLESSClientFlow` ([`protocol_adapter.go:148-150`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/protocol_adapter.go#L148-L150)) returns `""`. As a result, that client object is serialized into `config.json` without a `"flow"` key.
3. **Xray-core backfills top-level `settings.flow` onto clients missing `"flow"` on config load**:
   - Note on verification scope: Xray-core is invoked as an external binary (`Xray 26.11.9`) and is not vendored in local `go.mod`, so `infra/conf/vless.go` (`VLessInboundConfig.Build()`) is not present in this workspace; its fallback behavior (propagating inbound `settings.flow` onto each `Client` whose `Flow` is empty) was confirmed via external reference and corroborated by live deployment symptoms.
   - When `config.json` contains `inbounds[i].settings.flow = "xtls-rprx-vision"` and `clients[0]` omits `"flow"`, Xray-core parses the file and silently assigns `"xtls-rprx-vision"` to `<user-none>`.

### 1.2 Why gRPC user update temporarily masks the bug until restart/redeploy
- **Hot-reload via gRPC (`UserService.updateUser`)**:
  - [`user_service.go:455-468`](file:///d:/code/Workspace/xray-panel/internal/service/user_service.go#L455-L468) calls `xrayManager.RemoveUser` and `xrayManager.AddUser` -> [`manager.go:36-51`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/manager.go#L36-L51) -> [`grpc_client.go:98-135`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/grpc_client.go#L98-L135) -> `VLESSAdapter.BuildAccount` ([`protocol_adapter.go:171-177`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/protocol_adapter.go#L171-L177)).
  - `AddUser` sends gRPC `AlterInbound` (`AddUserOperation`) with `proto.VLESSAccount{Flow: ""}` directly into Xray's in-memory `UserManager`, bypassing `VLessInboundConfig.Build()`.
  - [`user_service.go:488`](file:///d:/code/Workspace/xray-panel/internal/service/user_service.go#L488) then calls `configSvc.SyncUserToFile` -> `SaveConfigQuietly` ([`config_service.go:203-252,625-627`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L203-L252)), which passes `reload=false` (`L251`). Thus Xray keeps `Flow=""` in memory while `config.json` on disk remains broken.
- **Startup / Redeploy (`main.go`)**:
  - [`main.go:128-131`](file:///d:/code/Workspace/xray-panel/main.go#L128-L131) runs `configSvc.SyncFromFile(bgCtx)` (`L128`) and `configSvc.RecompileAndApply(bgCtx, "面板启动自动同步与编译配置")` (`L131`, [`config_service.go:255-270`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L255-L270)), which calls `supervisor.Reload(ctx)` and causes Xray to re-read `config.json` and overwrite `<user-none>` back to `"xtls-rprx-vision"`.

### 1.3 Priority inversions: Inbound `"none"` currently blocks User `"xtls-rprx-vision"`
1. **Xray compilation & gRPC account path (`resolveVLESSClientFlow`)** ([`protocol_adapter.go:143-155`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/protocol_adapter.go#L143-L155)):
   - `L144-147` calls `inboundFlow := accessor.ResolveVisionFlow(inbound.Protocol)` and immediately returns `""` if `inboundFlow == ""` **before** inspecting `userFlow` (`L148-153`).
   - Because `ResolveVisionFlow` ([`stream_accessor.go:116-141`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/stream_accessor.go#L116-L141)) returns `""` whenever inbound `settingsMap["flow"] == "none"` (`L134-136`), an inbound configured with `flow="none"` blocks explicit `user.Flow = "xtls-rprx-vision"` even when transport is VLESS + TCP + (`tls` | `reality`).
   - `ResolveVisionFlow` conflates two distinct concerns: **transport capability** (`VLESS + TCP + (tls|reality)`) and **inbound default policy** (`settings.flow`). Also `userFlow` is not `strings.TrimSpace`-normalized before equality checks.
2. **Subscription / Link export path (`InboundToNodeConfigWithDefaultFP`)** ([`node_converter.go:331-353`](file:///d:/code/Workspace/xray-panel/internal/protocol/node_converter.go#L331-L353)):
   - `L340-348`:
     ```go
     if strings.EqualFold(customFlow, "none") || strings.EqualFold(user.Flow, "none") {
         flow = ""
     } else if user.Flow != "" {
         flow = user.Flow
     } ...
     ```
   - `strings.EqualFold(customFlow, "none")` is evaluated with `||` **before** `user.Flow != ""`, so inbound `flow="none"` strips Vision from subscription links (`FormatLink`, `ToClash`, `ToSingBox` in [`vless.go:70-75,170-174,198-202`](file:///d:/code/Workspace/xray-panel/internal/protocol/vless.go#L70-L75)) even when `user.Flow == "xtls-rprx-vision"`.

---

## 2. Priority Matrix

Target contract from [`prd.md`](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-13-fix-vless-vision-override/prd.md) (`User explicit ("none" / "xtls-rprx-vision") > Inbound default`, gated by transport capability `VLESS + TCP + (tls | reality)`):

| Transport (`net + sec`) | Inbound `settings.flow` | User `user.Flow` | Current Result (`resolveVLESSClientFlow` & `node_converter`) | Expected Result | Status |
| :--- | :--- | :--- | :--- | :--- | :--- |
| `tcp + (reality \| tls)` | `""` (unset) or `"xtls-rprx-vision"` | `""` (follow node) | `"xtls-rprx-vision"` | `"xtls-rprx-vision"` | OK |
| `tcp + (reality \| tls)` | `"xtls-rprx-vision"` | `"none"` | `""` in gRPC, **overridden to `"xtls-rprx-vision"` on Xray reload** | `""` (omitted in `clients[]` AND no top-level `settings.flow`) | **BUG** |
| `tcp + (reality \| tls)` | `"xtls-rprx-vision"` | `"xtls-rprx-vision"` | `"xtls-rprx-vision"` | `"xtls-rprx-vision"` | OK |
| `tcp + (reality \| tls)` | `"none"` | `""` (follow node) | `""` | `""` | OK (until `SyncFromFile` trap, see §3) |
| `tcp + (reality \| tls)` | `"none"` | `"none"` | `""` | `""` | OK |
| `tcp + (reality \| tls)` | `"none"` | `"xtls-rprx-vision"` | **`""` (blocked by inbound `"none"`)** | `"xtls-rprx-vision"` | **BUG** |
| Non-TCP or `sec=none` | any | any | `""` (subscription & compiler ignore) | `""` (transport does not support Vision) | OK |

---

## 3. `SyncFromFile` Traps (Critical Regressions If Top-Level `settings.flow` Is Stripped)

In [`config_service.go:272-450`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L272-L450), `syncFromRawJSONLocked` is invoked on every panel startup via `SyncFromFile` ([`main.go:128`](file:///d:/code/Workspace/xray-panel/main.go#L128)) BEFORE `RecompileAndApply` (`L131`). Once `DecorateSettings` strips top-level `inbounds[].settings.flow` from compiled `config.json`, `rawFlow` at [`config_service.go:314`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L314) will ALWAYS be `""` when reading panel-generated `config.json`, triggering three data-loss traps:

1. **Trap A — Inbound `"xtls-rprx-vision"` silently flipped to `"none"` when all active clients use `"none"`** ([`config_service.go:337-339`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L337-L339)):
   - Condition: `if (preserveNone && !hasClientWithFlow) || (hasAnyClient && !hasClientWithFlow) { settingsMap["flow"] = "none" }`.
   - Because `hasAnyClient && !hasClientWithFlow` ignores whether DB previously had `"flow": "xtls-rprx-vision"` (or unset), if an inbound has 1+ clients and all happen to have `user.Flow = "none"`, `SyncFromFile` writes `"flow": "none"` into DB `existing.SettingsJSON` (`L362`). Future `"跟随节点"` users will inherit `"none"` instead of Vision.
2. **Trap B — Inbound `"none"` silently erased when >=1 client explicitly uses `"xtls-rprx-vision"`** ([`config_service.go:315-340`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L315-L340)):
   - Suppose DB inbound has `settings.flow = "none"` (`preserveNone == true`), User A has `flow = ""` (follow inbound -> none), and User B has `flow = "xtls-rprx-vision"` (`hasClientWithFlow == true`).
   - In compiled `config.json`, `rawFlow == ""` and `hasClientWithFlow == true`. Thus `!hasClientWithFlow` is `false`, so `L337` does NOT set `settingsMap["flow"] = "none"`.
   - `L362` overwrites DB `existing.SettingsJSON` without `"flow": "none"`. On the immediately following `RecompileAndApply` ([`main.go:131`](file:///d:/code/Workspace/xray-panel/main.go#L131)), `ResolveVisionFlow` treats missing `flow` as `"xtls-rprx-vision"`, flipping User A from none to Vision!
   - (Note: existing test [`config_service_test.go:380-478`](file:///d:/code/Workspace/xray-panel/internal/service/config_service_test.go#L380-L478) `TestConfigService_SyncFromFile_PreservesEmptyFlowAcrossRestart` only covers zero-Vision-client restarts and would miss Trap B.)
3. **Trap C — Wholesale overwrite of `existing.SettingsJSON` & `existing.StreamSettings` on startup** ([`config_service.go:357-368`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L357-L368)):
   - `L362` sets `existing.SettingsJSON = string(settingsBytes)` (persisting compiled `clients` and overwriting DB `settingsMap["flow"]` if not explicitly preserved from `existing.SettingsJSON`, including `"xtls-rprx-vision-udp443"` if stripped).
   - `L363` sets `existing.StreamSettings = string(streamBytes)` directly from `config.json`, which already lost `tlsSettings.fingerprint` and Reality panel-only fields (`publicKey`, `fingerprint`, `spiderX`, `serverName`, `shortId`, `target`) stripped during `compileInbound` ([`compiler.go:311,321-324`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler.go#L311-L324)). Live SQLite DBs on both VPS hosts already show this loss (`reality.has_pbk=False`, `reality.fp=None`).
4. **Trap D — Existing vs new user flow handling in `SyncFromFile`** ([`config_service.go:412-434`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L412-L434)):
   - Existing matched users (`L412-421`) do not have `existingUser.Flow` overwritten by `SyncFromFile`, while newly imported users (`L434`) take `Flow: flow` (`""` when omitted). For `SaveAndApplyRawConfig` ([`config_service.go:121-140`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go#L121-L140)) importing an external hand-written `config.json` that still specifies top-level `settings.flow`, `rawFlow` should still be honored when `rawFlow != ""` rather than discarded before parsing.

---

## 4. Frontend Discrepancies

1. **`getNodeFlow` defaults missing `settings.flow` to `'none'` and ignores transport** ([`useInboundList.ts:63-74`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/composables/useInboundList.ts#L63-L74)):
   - `L69-70`: `if (s.flow !== undefined) return s.flow || 'none'; return 'none'`.
   - Backend `ResolveVisionFlow` and `InboundFormDrawer.vue:1370-1371` default unset `s.flow` on VLESS+TCP+(TLS|Reality) to `'xtls-rprx-vision'`, causing a display mismatch in [`InboundTable.vue:86-91,272-277`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/components/InboundTable.vue#L86-L91) and [`InboundDetailDrawer.vue:51-54`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/components/InboundDetailDrawer.vue#L51-L54).
   - Also lacks `tcp + (tls|reality)` check: stale `s.flow` on non-TCP or `security=none` would still display as Vision.
2. **`InboundFormDrawer.vue` precedence & stale-flow initialization bugs** ([`InboundFormDrawer.vue:1364-1375`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/components/InboundFormDrawer.vue#L1364-L1375)):
   - `L1365`: `(stream.network || inb.protocol === 'vless' ? stream.network || 'tcp' : 'tcp') === 'tcp'` has a JS operator precedence bug (`||` binds tighter than `?:`, so `inb.protocol === 'vless'` is evaluated as a condition branch operand rather than fallback).
   - `L1367-1374`: checks `settings.flow` before `!isTcp || !isTlsOrReality`; if stale `settings.flow === 'xtls-rprx-vision'` exists on a non-TCP/non-TLS inbound, `form.vlessFlow` initializes to `'xtls-rprx-vision'`.
   - `L141-143,162`: copy states `"分配给该节点的用户将继承此策略"` without clarifying that per-user explicit `flow` overrides the inbound default, and labels `none` as `(无流控 - 适用 XHTTP / gRPC / WS 等)`.
3. **`sanitizeInboundPayload` & `buildSettingsJSON` inconsistencies** ([`types.ts:335-345,571-595`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/types.ts#L335-L345)):
   - `sanitizeInboundPayload` (`L336-337`) converts empty `raw.vlessFlow === ''` on TCP+(Reality|TLS) into `'none'` instead of `'xtls-rprx-vision'`.
   - `buildSettingsJSON` (`L580-595`) writes `settings.clients` into DB `settingsJson` using inbound `flowVal`, ignoring individual `user.flow` (though `XrayCompiler` replaces `clients` from `userRepo` on compile).
4. **`UserFormDrawer.vue` missing option & precedence clarity** ([`UserFormDrawer.vue:107-119`](file:///d:/code/Workspace/xray-panel/web/src/views/users/components/UserFormDrawer.vue#L107-L119)):
   - Offers `""` (跟随节点默认策略), `"xtls-rprx-vision"`, `"none"`; omits `"xtls-rprx-vision-udp443"` (present in `InboundFormDrawer.vue:158`) and does not clarify `User > Inbound` override precedence or the `TCP + TLS/REALITY` capability constraint.

---

## 5. Live Redacted Verification (`vps-us` & `vps-us2`)

Executed strictly read-only via SSH (`Xray 26.11.9` on both hosts, `sqlite3.connect("file:/usr/local/xray-panel/data/panel.db?mode=ro", uri=True)`):

### `vps-us`
- **`/usr/local/etc/xray/config.json`**:
  - `inbound[0]`: `tag=入站`, `proto=vless`, `net=tcp`, `sec=reality`
  - `settings.keys=['clients', 'decryption', 'flow']`, `settings.flow='xtls-rprx-vision'`
  - `clients_count=3`: `c[0].flow='<OMITTED>'`, `c[1].flow='xtls-rprx-vision'`, `c[2].flow='xtls-rprx-vision'`
- **`/usr/local/xray-panel/data/panel.db`**:
  - `inbound id=2`, `tag=入站`, `proto=vless`, `enabled=1`, `settings.keys=['clients', 'decryption', 'flow']`, `settings.flow='xtls-rprx-vision'`, `clients_in_settings=3`
  - `net=tcp`, `sec=reality`, `reality.has_pbk=False`, `reality.fp=None`, `tls.fp=None` (evidence of Trap C from `SyncFromFile`)
  - Users (all `enabled=1`, inbound `入站`):
    - `<user-none>` (`<u1>`): `flow='none'` (corresponds to `c[0].flow='<OMITTED>'` in `config.json`)
    - `<user-vision-1>` (`<u10>`): `flow='xtls-rprx-vision'`
    - `<user-vision-2>` (`<u11>`): `flow='xtls-rprx-vision'`

### `vps-us2`
- **`/usr/local/etc/xray/config.json`**:
  - `inbound[0]`: `tag=ipv4入口`, `proto=vless`, `net=tcp`, `sec=reality`
  - `settings.keys=['clients', 'decryption', 'flow']`, `settings.flow='xtls-rprx-vision'`
  - `clients_count=2`: `c[0].flow='<OMITTED>'`, `c[1].flow='xtls-rprx-vision'`
- **`/usr/local/xray-panel/data/panel.db`**:
  - `inbound id=1`, `tag=ipv4入口`, `proto=vless`, `enabled=1`, `settings.keys=['clients', 'decryption', 'flow']`, `settings.flow='xtls-rprx-vision'`, `clients_in_settings=2`
  - `net=tcp`, `sec=reality`, `reality.has_pbk=False`, `reality.fp=None`, `tls.fp=None` (evidence of Trap C from `SyncFromFile`)
  - Users (all `enabled=1`, inbound `ipv4入口`):
    - `<user-none>` (`<u1>`): `flow='none'` (corresponds to `c[0].flow='<OMITTED>'` in `config.json`)
    - `<user-vision-1>` (`<u2>`): `flow='xtls-rprx-vision'`

---

## 6. File-by-File Fix & Fail-Repro Test Targets

### Backend Fix Targets
1. [`internal/adapter/xray/protocol_adapter.go`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/protocol_adapter.go) & [`internal/adapter/xray/stream_accessor.go`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/stream_accessor.go):
   - **`DecorateSettings`** (`protocol_adapter.go:179-191`): unconditionally `delete(settingsMap, "flow")` for VLESS compiled `config.json` (while leaving `inb.SettingsJSON` in DB intact).
   - **Separate capability gate from inbound default**:
     - Transport capability check (`SupportsVisionFlow()` / `IsVLESSVisionCapable`): true iff `protocol == "vless" && network == "tcp" && (security == "tls" || security == "reality")`.
     - Inbound default (`ResolveVisionFlow`): if not capable -> `""`; else if `strings.EqualFold(trimmedFlow, "none")` -> `""`; else if `trimmedFlow != ""` -> `trimmedFlow` (allow `xtls-rprx-vision` and `xtls-rprx-vision-udp443`); else `"xtls-rprx-vision"`.
   - **`resolveVLESSClientFlow`** (`protocol_adapter.go:143-155`):
     1. If `!accessor.SupportsVisionFlow(inbound.Protocol)` -> return `""`.
     2. Trim `userFlow`; if `strings.EqualFold(userFlow, "none")` -> return `""`.
     3. If `userFlow != ""` -> return `userFlow` (explicit user Vision overrides inbound `"none"`).
     4. Otherwise return `accessor.ResolveVisionFlow(inbound.Protocol)` (inbound default).
2. [`internal/protocol/node_converter.go`](file:///d:/code/Workspace/xray-panel/internal/protocol/node_converter.go):
   - **`InboundToNodeConfigWithDefaultFP`** (`L331-353`): check `strings.TrimSpace(user.Flow)` **before** `customFlow`:
     - If `strings.EqualFold(uFlow, "none")` -> `flow = ""`
     - Else if `uFlow != ""` -> `flow = uFlow`
     - Else if `strings.EqualFold(customFlow, "none")` -> `flow = ""`
     - Else if `customFlow != ""` -> `flow = customFlow`
     - Else -> `flow = "xtls-rprx-vision"`
3. [`internal/service/config_service.go`](file:///d:/code/Workspace/xray-panel/internal/service/config_service.go):
   - **`syncFromRawJSONLocked`** (`L272-450`, especially `L311-368`):
     - When `rawFlow == ""` (always true for panel-compiled `config.json` once `DecorateSettings` strips `settings.flow`), **preserve existing DB `prevSettings["flow"]`** instead of deriving `"none"` from `!hasClientWithFlow` (`L337`), unless creating/importing a brand-new inbound from raw config (`existing == nil`) where `!hasClientWithFlow && hasAnyClient` applies.
     - Do **not** let `SyncFromFile` overwrite DB `existing.StreamSettings` with stripped compiled `streamSettings` (`L363`) or discard panel-only `SettingsJSON` keys (`L362`) when `existing != nil` (preserve/merge DB `SettingsJSON` and `StreamSettings` fields like Reality `publicKey`, `fingerprint`, `spiderX`, `tlsSettings.fingerprint`).

### Frontend Fix Targets
4. [`web/src/views/inbounds/composables/useInboundList.ts`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/composables/useInboundList.ts):
   - Fix `getNodeFlow` (`L63-74`) to inspect `streamSettings` (`tcp` + `tls|reality`) and default missing `s.flow` to `'xtls-rprx-vision'` (matching backend & `InboundFormDrawer.vue`), and return `'none'` when transport is non-TCP or `security === 'none'`.
5. [`web/src/views/inbounds/components/InboundFormDrawer.vue`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/components/InboundFormDrawer.vue) & [`web/src/views/inbounds/types.ts`](file:///d:/code/Workspace/xray-panel/web/src/views/inbounds/types.ts):
   - Fix `isTcp` precedence bug & order of checks in `initFormData` (`InboundFormDrawer.vue:1364-1375`).
   - Align `sanitizeInboundPayload` (`types.ts:336-337`) default on TCP+(TLS|Reality) when `vlessFlow === ''` to `'xtls-rprx-vision'`.
   - Clarify UI copy that user-level `flow` overrides the inbound default.
6. [`web/src/views/users/components/UserFormDrawer.vue`](file:///d:/code/Workspace/xray-panel/web/src/views/users/components/UserFormDrawer.vue):
   - Consider adding `xtls-rprx-vision-udp443` option (parity with `InboundFormDrawer.vue:158`) and clarifying `User > Inbound` precedence + `TCP + TLS/REALITY` capability scope.

### Fail-Repro Test Targets (Fail-Repro First)
1. [`internal/adapter/xray/compiler_test.go`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler_test.go):
   - Inbound `flow="xtls-rprx-vision"` (or unset) + mixed users (`userA.Flow="none"`, `userB.Flow="xtls-rprx-vision"`, `userC.Flow=""`): assert compiled `inbounds[0].settings["flow"] == nil` (no top-level `settings.flow` emitted!), `userA` has `flow == ""` (omitted), `userB` & `userC` have `"xtls-rprx-vision"`. (Update/preserve [`L740-776`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler_test.go#L740-L776) & [`L1552-1624`](file:///d:/code/Workspace/xray-panel/internal/adapter/xray/compiler_test.go#L1552-L1624).)
   - Inbound `flow="none"` on TCP+Reality + user `user.Flow="xtls-rprx-vision"`: assert compiled client `flow == "xtls-rprx-vision"` (currently fails -> returns `""`), AND assert non-TCP (`ws`) + `user.Flow="xtls-rprx-vision"` still compiles to `""`.
   - Inbound `flow="xtls-rprx-vision-udp443"` + `user.Flow=""`: assert client gets `"xtls-rprx-vision-udp443"` while top-level `settings["flow"]` is absent.
2. [`internal/protocol/node_converter_test.go`](file:///d:/code/Workspace/xray-panel/internal/protocol/node_converter_test.go) (or new test in `internal/protocol`):
   - Inbound `flow="none"` on TCP+Reality + `user.Flow="xtls-rprx-vision"`: assert exported `NodeConfig.Flow == "xtls-rprx-vision"` (and `FormatLink`/`ToClash`/`ToSingBox` include Vision).
   - Inbound `flow="xtls-rprx-vision"` + `user.Flow="none"`: assert `NodeConfig.Flow == ""`.
3. [`internal/service/config_service_test.go`](file:///d:/code/Workspace/xray-panel/internal/service/config_service_test.go):
   - **Mixed users under Inbound `"none"` across `RecompileAndApply` + `SyncFromFile`**: DB inbound `flow="none"`, `userA.Flow=""`, `userB.Flow="xtls-rprx-vision"` -> compile `config.json` (no top-level `settings.flow`) -> `SyncFromFile` -> recompile: assert DB `settings.flow` stays `"none"` and `userA` stays `""`, `userB` stays `"xtls-rprx-vision"`.
   - **All users `"none"` under Inbound `"xtls-rprx-vision"` across `RecompileAndApply` + `SyncFromFile`**: assert DB `settings.flow` remains `"xtls-rprx-vision"` (not overwritten to `"none"`).
   - **DB `StreamSettings` preservation across `SyncFromFile`**: assert Reality `publicKey`, `fingerprint`, `spiderX` and `tlsSettings.fingerprint` survive `RecompileAndApply` -> `SyncFromFile`.
4. Frontend unit test (e.g. `web/` Vitest if configured, or component test for `getNodeFlow`):
   - `getNodeFlow` with `{ protocol: 'vless', settingsJson: '{"decryption":"none"}', streamSettings: '{"network":"tcp","security":"reality"}' }` returns `'xtls-rprx-vision'`, and with `network: 'ws'` returns `'none'`.
