# Fix VLESS Vision Override on Panel Redeploy

## Problem Statement
Every panel redeploy (`deploy.ps1` / `deploy.sh`) or restart causes VLESS inbounds/users on `vps-us` (`tag=入站`) and `vps-us2` (`tag=ipv4入口`) configured with `flow = "none"` to behave as `xtls-rprx-vision` in Xray, while the Panel UI displays `none` for the user. Re-saving the user in UI ("重新部署用户") temporarily masks the bug via gRPC `AddUser` + `SaveConfigQuietly(reload=false)` until the next panel redeploy/restart reloads `config.json`.

Verified root causes (see `research/vless-vision-audit.md`):
1. `VLESSAdapter.DecorateSettings` keeps top-level `inbounds[].settings.flow = "xtls-rprx-vision"` in `config.json` while `XrayClient.Flow` (`omitempty`) omits `flow` when `""`. On config reload, Xray-core `VLessInboundConfig.Build()` backfills top-level `settings.flow` onto clients missing `"flow"`.
2. `resolveVLESSClientFlow` (`protocol_adapter.go`) and `InboundToNodeConfigWithDefaultFP` (`node_converter.go`) block explicit user `"xtls-rprx-vision"` when Inbound default is `"none"`.
3. `ConfigService.syncFromRawJSONLocked` (`SyncFromFile`) has traps when top-level `settings.flow` is stripped (inverting Inbound `"none"` vs `"xtls-rprx-vision"`, plus wholesale overwriting DB `StreamSettings` and wiping Reality `publicKey`/`fingerprint` — observed on both live VPS DBs).
4. Frontend `getNodeFlow` (`useInboundList.ts`), `InboundFormDrawer.vue` (`isTcp` operator precedence & check order), and `sanitizeInboundPayload` (`types.ts`) disagree with backend default/precedence rules.

## Acceptance Criteria
- **AC1 (No Top-Level Inbound `flow` in Compiled Xray Config)**: Compiled `config.json` never emits `inbounds[].settings.flow`; `flow` is emitted strictly per client (`inbounds[].settings.clients[].flow`).
- **AC2 (Unified Precedence Across Compiler, gRPC, Subscription Export)**: Effective flow strictly follows: Transport capability (`VLESS + TCP + (tls|reality)`) -> User explicit (`"none"` -> `""`, `"xtls-rprx-vision"` / `"xtls-rprx-vision-udp443"` -> explicit) -> Inbound default (`"none"` -> `""`, explicit -> explicit, unset/empty -> `"xtls-rprx-vision"`).
- **AC3 (`SyncFromFile` & Raw Config Safety)**: When `rawFlow == ""` and `existing != nil`, `syncFromRawJSONLocked` preserves DB `SettingsJSON["flow"]` verbatim (never mutating from `!hasClientWithFlow` / `hasClientWithFlow`), and preserves panel-only `StreamSettings` client fields (`realitySettings.{publicKey,fingerprint,spiderX}`, `tlsSettings.fingerprint`). Explicit `rawFlow != ""` (e.g. via `SaveAndApplyRawConfig`) and brand-new inbound imports (`existing == nil`) still work as expected.
- **AC4 (Frontend Alignment)**: `getNodeFlow`, `InboundFormDrawer.vue`, `sanitizeInboundPayload`, and `UserFormDrawer.vue` align with AC2 capability/precedence rules and defaults.
- **AC5 (Fail-Repro Tests & Quality Gates)**: Failing tests reproduce the bugs first, then pass alongside `go test ./...`, `go vet ./...`, `go build .`, and `cd web && npm run build`.
