# Implementation Plan: Fix VLESS Vision Override

## Ordered Checklist (Fail-Repro First)

1. **Step 1 (Red — Backend Fail-Repro Tests)**
   - `internal/adapter/xray/compiler_test.go`:
     - Inbound `flow="xtls-rprx-vision"` (or unset) + mixed users (`none`, `xtls-rprx-vision`, `""`): assert compiled `inbounds[0].settings["flow"] == nil` (no top-level `flow`) and per-client `flow` values match AC2.
     - Inbound `flow="none"` on TCP+Reality + user `flow="xtls-rprx-vision"`: assert compiled client `flow == "xtls-rprx-vision"`; non-TCP (`ws`) + user `flow="xtls-rprx-vision"` still compiles to `""`.
     - Inbound `flow="xtls-rprx-vision-udp443"` + user `flow=""`: assert client gets `"xtls-rprx-vision-udp443"` and top-level `settings["flow"]` is absent.
   - `internal/protocol/` (`node_converter_test.go` or existing protocol test):
     - Inbound `flow="none"` on TCP+Reality + user `flow="xtls-rprx-vision"` -> exported `NodeConfig.Flow == "xtls-rprx-vision"`.
     - Inbound `flow="xtls-rprx-vision"` + user `flow="none"` -> exported `NodeConfig.Flow == ""`.
   - `internal/service/config_service_test.go`:
     - Mixed users under Inbound `flow="none"` across `RecompileAndApply` + `SyncFromFile` preserves DB `settings.flow = "none"`.
     - All `"none"` users under Inbound `flow="xtls-rprx-vision"` across `RecompileAndApply` + `SyncFromFile` preserves DB `settings.flow = "xtls-rprx-vision"`.
     - Reality `publicKey`, `fingerprint`, `spiderX` and `tlsSettings.fingerprint` survive `RecompileAndApply` -> `SyncFromFile`.
   - Run `mise x -- go test ./internal/adapter/xray/... ./internal/protocol/... ./internal/service/...` to confirm Red (fail-repro).

2. **Step 2 (Green — Backend Implementation)**
   - `internal/adapter/xray/stream_accessor.go` & `internal/adapter/xray/protocol_adapter.go`: implement `SupportsVisionFlow`, `resolveVLESSClientFlow` priority fix, and unconditional `delete(settingsMap, "flow")` in `DecorateSettings`.
   - `internal/protocol/node_converter.go`: reorder `user.Flow` > `customFlow` in `InboundToNodeConfigWithDefaultFP`.
   - `internal/service/config_service.go`: fix `syncFromRawJSONLocked` to preserve DB `settings.flow` and panel-only `StreamSettings` fields for existing inbounds.
   - Re-run `mise x -- go test ./...` until Green.

3. **Step 3 (Frontend Alignment)**
   - Update `web/src/views/inbounds/composables/useInboundList.ts` (`getNodeFlow`).
   - Update `web/src/views/inbounds/components/InboundFormDrawer.vue` (`initFormData` precedence/order + helper copy).
   - Update `web/src/views/inbounds/types.ts` (`sanitizeInboundPayload`).
   - Update `web/src/views/users/components/UserFormDrawer.vue` (`xtls-rprx-vision-udp443` + helper copy).

4. **Step 4 (Full Quality Gates)**
   - `mise x -- go test ./...`
   - `mise x -- go vet ./...`
   - `mise x -- go build .`
   - `cd web && mise x -- npm run build`

## Scope Note
Backend (`internal/**`) and Frontend (`web/**`) share one tight flow contract; recommended Swarm execution uses `1 Author + 1 Verifier` (or `Inline` direct execution).
