# Codebase Audit: `xray-agent` gRPC User Hot-Update Drift & Single Authoritative Source of Truth

> Task: `11-21-fix-agent-grpc-user-sync-drift`  
> Scope: Read-only audit of Master control plane, Xray compiler/gRPC adapters, `cmd/agent`, `internal/agent`, and `.trellis/spec/` contracts.  
> Git working tree note: `git status` shows `Cherry-pick currently in progress` (empty `git diff`, no modified tracked files; 3 untracked directories under `.trellis/tasks/`: `11-19-m4-invite-code-onboarding/`, `11-19-multi-node-direct-agent/`, `11-21-fix-agent-grpc-user-sync-drift/`).

---

## 1. Candidate `.trellis/spec/` Paths for `implement.jsonl` & `check.jsonl`

From `.trellis/spec/backend/index.md` and `.trellis/spec/guides/index.md`, the authoritative spec files governing this task are:

| Spec Path | Coverage / Relevance |
|---|---|
| `.trellis/spec/backend/index.md` | Layering topology and version source-of-truth index |
| `.trellis/spec/backend/directory-structure.md` | `cmd/agent` & `internal/agent` dependency isolation (lines 29–33: no `gin`/`gorm`/`sqlite`/`telegram`), `NodeID == 0` local vs remote separation |
| `.trellis/spec/backend/error-handling.md` | No silent error swallowing (`_ = ...`), `%w` wrapping, ignorable Xray gRPC error classification vs non-UserManager protocol bypass (lines 9–19, 66–73) |
| `.trellis/spec/backend/quality-guidelines.md` | Concurrency/race safety, KISS anti-bloat (lines 7–26); VLESS flow invariants (lines 91–145); Multi-Node Control Plane & Agent Sync invariants (lines 256–326); Async NodeCommand & Runner invariants (lines 329–389); `NodeRepository` KISS (lines 393–400) |
| `.trellis/spec/backend/logging-guidelines.md` | `slog` conventions, English log messages, zero secret leakage |
| `.trellis/spec/guides/index.md` | Thinking guides index and pre-modification search rule |
| `.trellis/spec/guides/cross-layer-thinking-guide.md` | Cross-layer boundary contracts (Master ↔ Agent HTTP protocol ↔ Xray disk `config.json` ↔ Xray gRPC runtime) |
| `.trellis/spec/guides/code-reuse-thinking-guide.md` | Avoiding duplicate JSON patching/compilation or duplicated traffic/protocol logic |

---

## 2. Inventory: `cmd/agent`, `internal/agent`, `internal/adapter/xray`, and `go.mod`

### 2.1 `go.mod` & Xray-core Protobuf Availability
- `go.mod:15-16` includes `google.golang.org/grpc v1.83.2` and `google.golang.org/protobuf v1.36.12`, with **no** `github.com/xtls/xray-core` Go dependency.
- All Xray gRPC definitions live as local hand-trimmed `.proto` + generated `.pb.go` files under `internal/adapter/xray/proto/`:
  - `internal/adapter/xray/proto/proxyman.proto:24-26` & `proxyman_grpc.pb.go:21-30`: `HandlerService` currently exposes **only** `AlterInbound(AlterInboundRequest) returns (AlterInboundResponse)`. `GetInboundUsers` and `GetInboundUsersCount` are **not** currently declared.
  - `internal/adapter/xray/proto/stats.proto:20-22` & `stats_grpc.pb.go:21-30`: `StatsService` currently exposes **only** `QueryStats(QueryStatsRequest) returns (QueryStatsResponse)`. `GetSysStats` is **not** currently declared.
  - `internal/adapter/xray/proto/protocol.proto:21-25`: `User{level, email, account}`.
  - `internal/adapter/xray/proto/proxy.proto:8-42` & `typed_message.go:9-45`: supports `VLESSAccount`, `VMessAccount`, `TrojanAccount`, `ShadowsocksAccount`, `Shadowsocks2022Account`.
- Crucially, **both** Master's remote compiler (`internal/adapter/xray/compiler.go:86-89`) and Agent's circuit-breaker silence config (`internal/agent/config_manager.go:293-296`) already enable `"HandlerService"` and `"StatsService"` in Xray's `api.services` block. Therefore, Xray-core's running gRPC server already serves the full `HandlerService` (including `GetInboundUsers`) and `StatsService` (including `GetSysStats`) on `127.0.0.1:8080` (`compiler.go:349`, `cmd/agent/main.go:43`) without needing any `config.json` API service changes or importing `github.com/xtls/xray-core`.

### 2.2 `internal/adapter/xray` Client & Compiler
- `internal/adapter/xray/grpc_client.go:18-223`:
  - `AddUser(ctx, inbound, user)` (`98-135`): builds account via `BuildAccountMessage(inbound, user)`, hardcodes `proto.User{Level: 0, Email: user.Email}` (`109-113`), calls `HandlerService.AlterInbound` with 5s default RPC timeout (`77-82`).
  - `RemoveUser(ctx, inboundTag, email)` (`138-164`): calls `HandlerService.AlterInbound` with `RemoveUserOperation`.
  - `QueryTrafficStats(ctx, reset)` (`167-223`): calls `StatsService.QueryStats` with `Pattern: ""` and filters out `value == 0` (`189-191`), so zero-traffic users cannot be enumerated via `QueryStats`.
- `internal/adapter/xray/manager.go:36-77`:
  - Master's local `xray.Manager` enforces `!inbound.IsLocal()` (`44, 63`) so Master never invokes local gRPC for remote inbounds (`NodeID > 0`). On Master local mutations, `UserService` calls `xrayManager.AddUser/RemoveUser` and `configSvc.SaveConfigQuietly()` (persisting updated `config.json` without restarting local Xray).
- `internal/adapter/xray/compiler.go`:
  - `CompileRemoteNodeFull` / `CompileRemoteNodeFullToJSON` (`366-433`): compiles DNS, sorted enabled inbounds (with `users` + `staticClients` merged via `mergeStaticClients` at `474-544`), fixed `api` inbound on `127.0.0.1:8080` (`407`, `110-124`), outbounds (ensuring `direct` + `block`), and routing (Layer 1 `api -> api` + Layer 2 rules).
  - `BuildRemoteNodeUsers` (`436-468`): iterates sorted enabled inbounds where `inb.Tag != "api" && domain.SupportsDynamicUser(inb.Protocol)` (`441`), resolves `Method` for Shadowsocks/SS2022 (`446-450`) and per-client `Flow` for VLESS (`452-455`), and emits `[]domain.AgentSyncUser` for active DB users only. **Explicitly excludes** static relay clients (`relay-auto:<tag>@internal`) and Trojan's fixed `settings.password` client (`<tag>@trojan.local` injected by `TrojanAdapter.CompileClients` at `internal/adapter/xray/protocol_adapter.go:243-259`).
- `internal/domain/inbound.go:86-93`: `SupportsDynamicUser` returns `true` for `vless`, `vmess`, `trojan`, `shadowsocks`, `shadowsocks-2022`, `ss`, `ss-2022`, `ss2022`.

### 2.3 `cmd/agent` & `internal/agent` Components
- `cmd/agent/main.go:37-146`: Wires `Client`, `Spool`, `QuotaStore`, `xray.NewGRPCClient`, `XrayGRPCAdapter`, `ConfigManager`, `UserSyncer`, `HTTPGeoDataUpdater`, and `Runner`. Persistence directory is `--spool-dir` (`/var/lib/xray-agent` by default, `46`), used by `Spool` (`spool.go:31`) and `QuotaStore` (`quota_store.go:50`).
- `internal/agent/protocol.go:17-113`: Defines `SyncRequest`, `SyncResponse`, `RemoteUser`, `QuotaPolicy`, `Command`, `CommandResult`.
- `internal/agent/client.go:16-136`: `Client.Sync` (`52-96`) posts `SyncRequest` to `/api/agent/sync` (15s HTTP timeout, 16 MiB response cap); `Client.ReportCommandResult` (`99-136`) posts to `/api/agent/commands/:id/result`.
- `internal/agent/config_manager.go:38-330`:
  - `Apply(ctx, configJSON)` (`88-146`): writes `.tmp.json` (`0644`, `tempConfigPath` at `251-257`), runs `xray run -test -c <tmp>` (`105`), backs up existing `config.json` to `config.json.bak` (`0600`, `111-118`), `os.Rename(tmpPath, ConfigPath)` (`124`), calls `reloadService` (`186-200`, defaults to `systemctl reload-or-restart <service>`), then `waitForProbe` (`202-230`, `ProbeTimeout=3s`, `ProbeInterval=200ms`). On reload or probe failure, restores `.bak` and reloads (`rollbackLocked` at `232-247`).
  - `ApplyCircuitBreaker(ctx)` (`150-156`): generates minimal silence JSON (`buildSilenceConfigJSON` at `278-330`) and calls `m.Apply`.
  - `ReloadAndProbe(ctx)` (`159-170`): reloads systemd service and probes gRPC; invoked by `HTTPGeoDataUpdater.Update` (`geodata_updater.go:143`) after swapping `geoip.dat`/`geosite.dat`.
- `internal/agent/xray_grpc_adapter.go:21-131`:
  - `Probe(ctx)` (`34-37`): calls `a.client.QueryTrafficStats(ctx, false)`.
  - `AddUser(ctx, user)` (`40-67`): constructs a synthetic `domain.Inbound` from `RemoteUser`: sets `StreamSettings = '{"network":"tcp","security":"tls"}'` only when `user.Flow != ""` (`41-44`), and `SettingsJSON = '{"method":...}'` only when `user.Method != ""` (`45-54`).
  - `RemoveUser(ctx, inboundTag, email)` (`70-72`): forwards to `GRPCClient.RemoveUser`.
  - `CollectTrafficStats(ctx)` (`75-131`): queries + resets stats (`QueryTrafficStats(ctx, true)`).
- `internal/agent/user_sync.go:18-129`:
  - Holds `applied map[string]RemoteUser` keyed by `inboundTag + "\x00" + email` (`21, 35-37`) purely in memory.
  - `ResetBaseline(users)` (`40-52`) overwrites `s.applied`.
  - `Reconcile(ctx, desired)` (`56-116`) diffs `desired` vs `s.applied` on `UUID`, `Flow`, `Protocol`, `Method` (`77`), calls `RemoveUser` then `AddUser`, updates `s.applied` per succeeded operation, and joins errors.
  - **Defect found (`user_sync.go:105-113` & `118-129`)**: `AddUser` uses the same `isIgnorableXrayError` as `RemoveUser`, which ignores `"handler not found"`, `"failed to get handler"`, `"not found"`, and `"not exist"`. Compare Master's `internal/service/user_service.go:88-108`, where `isXrayIgnorableAddUserError` **only** ignores `"not a usermanager"` / `"does not support dynamic account operations"`. In `UserSyncer`, if an inbound handler is absent from Xray (e.g., Xray restarted on silence/stale config), `AddUser` fails with `handler not found`, `UserSyncer` silently ignores it, marks the user in `s.applied`, and returns `err == nil`!
- `internal/agent/runner.go:25-627`:
  - Holds `configHash`, `usersHash`, `lastError`, `circuitBroken` solely in memory (`40-43`).
  - `reconcileSyncResponse` (`233-296`):
    1. If `DesiredState == "circuit_broken"`: applies silence config once (`alreadyBroken`), resets `userSync` to `nil`, clears `configHash`/`usersHash` (`234-256`).
    2. Else if `!resp.ConfigUnchanged && resp.ConfigJSON != ""`: calls `cfgMgr.Apply`, updates `r.configHash`, and **only if `!resp.UsersUnchanged`** calls `r.userSync.ResetBaseline(resp.Users)` and updates `r.usersHash` (`258-274`).
    3. Else if `!resp.UsersUnchanged`: calls `userSync.Reconcile(ctx, resp.Users)` and updates `r.usersHash` (`276-289`). **Does NOT touch `config.json` on disk.**
    4. Else (`ConfigUnchanged && UsersUnchanged`): clears `circuitBroken`/`lastError`, returns `applied=false, nil` (`291-295`).
  - If `applied == true`, immediately sends a lightweight ack heartbeat (`HostMetrics: nil, TrafficBatch: nil`) via `sendConfigAck` (`224-226, 521-542`).

---

## 3. Master-Side Sync Protocol, Hash Calculation & Compatibility

### 3.1 How Master Builds `config.json` and Hashes
In `internal/service/node_service.go:919-1015` (`getOrBuildSnapshot`):
1. `structuralBytes` (`984`): `CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, nil, staticClients)` -> `ConfigHash = sha256(structuralBytes)` (`1002`). Excludes dynamic DB users (`users = nil`), so user mutations do **not** change `ConfigHash`. Includes node inbounds, custom outbounds/routing/DNS, and static relay clients (`extractStaticRelayClients` at `1502-1594`).
2. `fullConfigBytes` (`988`): `CompileRemoteNodeFullToJSON(nodeInbounds, outbounds, routing, dns, allUsers, staticClients)` -> `ConfigJSON = string(fullConfigBytes)` (`1004`). **Master already compiles the full `config.json` including all active DB users + static clients.**
3. `syncUsers` (`992`): `BuildRemoteNodeUsers(nodeInbounds, allUsers)` -> `UsersHash = sha256(json.Marshal(syncUsers))` (`996, 1003`).
4. Cache invalidation (`node_service.go:244-262`):
   - `ConfigService` calls `InvalidateNode(nodeID)` or `InvalidateAll()` (`config_service.go:101-111`) on inbound/outbound/routing/DNS/raw-config changes.
   - `UserService` calls `InvalidateAll()` (`user_service.go:82-86`) on user mutations.
   - `NodeService` calls `InvalidateNode` on node host change (`506-508`) and `InvalidateAll` when a traffic batch pushes a user over quota (`805-810`).

### 3.2 `HandleAgentSync` & Node Sync Status Tracking
- `internal/delivery/http/handler_agent.go:22-54` binds `domain.AgentSyncRequest` and delegates to `NodeService.HandleAgentSync` (`node_service.go:785-917`).
- `HandleAgentSync` computes (`881-882`):
  - `configUnchanged := req.ConfigHash != "" && req.ConfigHash == snap.ConfigHash`
  - `usersUnchanged := req.UsersHash != "" && req.UsersHash == snap.UsersHash`
- Payload omission (`906-911`):
  - `if !configUnchanged { resp.ConfigJSON = snap.ConfigJSON }`
  - `if !usersUnchanged { resp.Users = snap.Users }`
- `UpdateHeartbeat` (`internal/adapter/repository/node_repo.go:70-80`) persists `req.ConfigHash` -> `nodes.last_config_hash`, `req.UsersHash` -> `nodes.last_users_hash`, and `req.LastError` -> `nodes.last_sync_error`.
- `Node.RefreshSyncStatus` (`internal/domain/node.go:221-235`) compares `n.LastConfigHash == expectedConfigHash` (which is `snap.ConfigHash` from `resolveExpectedConfigHash`, `node_service.go:264-280`) to compute `ConfigSyncStatus` (`"synced"` / `"pending"` / `"error"`).

### 3.3 Backward / Forward Compatibility Constraints
- `AgentSyncRequest` / `SyncRequest` (`domain/node.go:366-375`, `agent/protocol.go:17-26`) and `AgentSyncResponse` / `SyncResponse` (`domain/node.go:398-410`, `agent/protocol.go:58-70`) are **locked HTTP JSON contracts** (`handler_agent_test.go`, `agent_e2e_test.go`, `node_service_test.go`).
- Key invariants verified by existing tests:
  1. When `req.ConfigHash == snap.ConfigHash && req.UsersHash == snap.UsersHash`, Master must omit `configJson` and `users` and set `configUnchanged: true, usersUnchanged: true` (`agent_e2e_test.go:407-416`, `node_service_test.go:246-258`).
  2. When only DB users change (`req.ConfigHash == snap.ConfigHash && req.UsersHash != snap.UsersHash`), Master sets `configUnchanged: true, usersUnchanged: false` and sends `Users` without `ConfigJSON` (`node_service_test.go:260-279`, `handler_agent_test.go`).
  3. On Agent side, `Runner` must NOT restart Xray on user-only mutations (`agent_e2e_test.go:418-431`).
  4. `Node.LastConfigHash` reported by Agent must match `snap.ConfigHash` (`structuralBytes` hash) so `ConfigSyncStatus` resolves to `"synced"` (`node_service_test.go:934-953`).
  5. Adding optional fields (e.g., `omitempty` in JSON) or new optional fields in `AgentSyncRequest`/`AgentSyncResponse` is backward-compatible with old Agents/Masters because Go `encoding/json` ignores unknown fields, **unless** existing tests forbid unexpected keys on short-circuit responses (`agent_e2e_test.go:414-416` checks `configJson` and `users` are omitted).

---

## 4. Root-Cause Diagnosis: Why Drift Occurs Today

### 4.1 Five Disconnected State Copies (Split-Brain)
Today, remote node state exists in **five** separate places without closed-loop reconciliation:
1. **Master SQLite + compiled `cachedNodeSnapshot`** (`ConfigJSON` + `Users` + `ConfigHash` + `UsersHash`).
2. **Agent Disk `/usr/local/etc/xray/config.json`** — updated **only** when `ConfigHash` changes (`runner.go:258-274`) or during circuit breaking (`240`). Never updated during user-only gRPC syncs (`276-289`).
3. **Agent Memory** (`Runner.configHash`, `Runner.usersHash`, `Runner.circuitBroken`, `UserSyncer.applied`) — ephemeral; wiped on `xray-agent` restart; blind to external `xray` restarts.
4. **Xray Runtime Memory** (in-process `UserManager` handlers) — initialized from disk `config.json` on Xray startup/restart, then mutated in-memory via gRPC `AlterInbound`.
5. **Master `nodes` DB columns** (`last_config_hash`, `last_users_hash`, `last_sync_error`) — blindly records whatever hashes Agent claimed in `SyncRequest`.

### 4.2 Concrete Failure Scenarios
1. **User hot-update lost on Xray restart**:
   - User added/edited/removed on Master -> `UsersHash` changes, `ConfigHash` unchanged -> Agent runs `UserSyncer.Reconcile` (gRPC only) and updates `r.usersHash`. Disk `config.json` stays stale.
   - Next Xray restart (crash/OOM, `systemctl restart xray`, server reboot, or `geodata_update` calling `ConfigManager.ReloadAndProbe` at `geodata_updater.go:143`) reloads stale `config.json`: gRPC-added users vanish, gRPC-removed/expired users resurrect!
2. **No self-healing when Xray restarts while Agent stays running**:
   - Agent's in-memory `r.configHash`, `r.usersHash`, and `userSync.applied` still match Master's hashes -> Master returns `ConfigUnchanged: true, UsersUnchanged: true` -> Agent skips both `ConfigManager` and `UserSyncer` (`runner.go:291-295`). Even if a *future* user change occurs, `UserSyncer.Reconcile` diffs against stale `s.applied`, leaving all previously hot-updated users missing/stale!
3. **Unnecessary Xray restart on `xray-agent` restart**:
   - Agent restarts -> `r.configHash == ""`, `r.usersHash == ""` -> Master sends full `ConfigJSON` -> `ConfigManager.Apply` unconditionally overwrites `config.json` and executes `systemctl reload-or-restart xray`, disrupting active proxy connections even when `config.json` on disk and Xray runtime were already healthy and up to date.
4. **Silent `handler not found` poisoning in `UserSyncer.Reconcile`**:
   - Because `user_sync.go:106` uses `isIgnorableXrayError` for `AddUser`, if Xray is running a silence config or missing an inbound handler, `AddUser` fails with `handler not found`, `UserSyncer` ignores the error, inserts the user into `s.applied`, and `Runner` advances `r.usersHash` — silently dropping the user permanently.

---

## 5. Architectural Analysis & Design Trade-Offs

### 5.1 Establishing a Single Authoritative Source of Truth
- **Control-Plane Authority**: Master's SQLite + `XrayCompiler` (`cachedNodeSnapshot`) is the **sole authoritative source of truth** for desired node state (`ConfigJSON` + `Users`). Agent must never synthesize or diverge on protocol/client compilation rules (e.g., VLESS flow priority, SS-2022 ciphers, Trojan fixed password, `relay-auto:*@internal` static clients).
- **Agent Local Execution Authority**:
  - **Cold-start / on-disk SSOT**: `/usr/local/etc/xray/config.json` (plus an atomic Agent state file in `spoolDir`, e.g. `sync_state.json`, or `sha256` / metadata of applied state). Every synced state transition must converge **both** Xray runtime memory **and** disk `config.json` to match Master's authoritative snapshot.
  - **Runtime SSOT verification**: Xray's live gRPC state is the **ground truth** for what Xray is currently serving; Agent memory (`applied`) must be treated as a cache that is verified/rebuilt from Xray gRPC reality rather than trusted blindly.

### 5.2 Disk Persistence Without Restarting Xray (When Only Users Change)
How should Agent persist user changes to `config.json` on disk **without** calling `reloadService` (`systemctl reload-or-restart`)?

| Option | Mechanism | Pros | Cons / Risks |
|---|---|---|---|
| **A. Master-rendered `ConfigJSON` delivery (Recommended)** | When `!UsersUnchanged` (or Agent requests full config), Master sends `snap.ConfigJSON` (which `node_service.go:988` **already compiles and caches** in `cachedNodeSnapshot`!). Alternatively, Agent sends `ConfigHash: ""` when it needs `ConfigJSON`, OR Master also includes `ConfigJSON` whenever `!UsersUnchanged` (or via a `NeedConfig` flag). Agent verifies `xray run -test`, writes `config.json` atomically (`0644` + `.bak`), and **does NOT restart Xray** if structural `ConfigHash` matches what Xray is already running (only gRPC `UserSyncer` runs). | 1. **Zero dual JSON generation logic**: Master's `XrayCompiler` remains the 100% single compiler (`VLESSAdapter`, `TrojanAdapter` fixed password, `mergeStaticClients` `relay-auto:*`, `SS2022`).<br>2. **Zero drift risk** between `config.json` and `XrayCompiler`.<br>3. `snap.ConfigJSON` is already computed and cached in `NodeService.getOrBuildSnapshot` (`node_service.go:988,1004`). | 1. Payload size on user changes: `ConfigJSON` is typically 2–20 KB for typical node configs (16 MiB cap in `client.go:18`), negligible at 60s intervals.<br>2. **Compatibility check**: Existing tests (`node_service_test.go:274-276`, `runner_test.go:93-105`, `agent_e2e_test.go:411-416`) test that when `req.ConfigHash == snap.ConfigHash && !UsersUnchanged`, Master returns `ConfigUnchanged: true` (or tests expect `ConfigJSON == ""`?). Let's check: `node_service_test.go:274` only asserts `resp3.ConfigUnchanged == true && resp3.UsersUnchanged == false` (it does NOT assert `resp3.ConfigJSON == ""` on `resp3`, though `handler_agent_test.go:222` checks short-circuit when BOTH are unchanged). Even cleaner: if Agent needs `ConfigJSON` to persist disk (or Master omits `ConfigJSON` only when `ConfigUnchanged && UsersUnchanged`, OR Agent sends an explicit request/flag or Master sends `ConfigJSON` when `!UsersUnchanged` while keeping `ConfigUnchanged: true` as the "no structural restart needed" signal—or Agent requests `ConfigJSON` on disk sync). |
| **B. Agent-side JSON patching of `config.json`** | Agent parses local `config.json`, finds each `inbounds[].settings.clients`, and patches from `resp.Users`. | Smaller HTTP payload on user updates. | 1. **Violates SSOT & duplicates compiler logic**: Agent must replicate `VLESS` flow, `Trojan` `password` vs `id`, preserving `<tag>@trojan.local` (`protocol_adapter.go:255`) and `relay-auto:*@internal` (`node_service.go:1591`), `Shadowsocks` `password`+`method` vs `SS2022` `password`-only (`protocol_adapter.go:284-288,346-350`), and `clients: []` zero-retention.<br>2. If `config.json` on disk was missing/corrupted/circuit-broken (`buildSilenceConfigJSON` wiped business inbounds!), Agent **cannot** patch it because business inbounds are gone! |

**Conclusion on 5.2**: **Option A (Master-rendered `ConfigJSON` delivery)** strictly preserves the Single Authoritative Source of Truth and avoids duplicating `XrayCompiler` inside `internal/agent`.
- Furthermore, notice `ConfigHash` semantics: `ConfigHash` is already `sha256(structuralBytes)` (`node_service.go:984,1002`), meaning `ConfigUnchanged: true` specifically means **"structural config unchanged — Xray restart not required"**.
- If Master includes `ConfigJSON: snap.ConfigJSON` whenever `!configUnchanged || !usersUnchanged` (or when Agent explicitly indicates it has no structural match / needs disk sync, e.g. `ConfigUnchanged && !UsersUnchanged`), wait: check `TestNodeService_HandleAgentSync` and `TestRunner_EndToEnd...`!
  - In `runner_test.go:92-105` and `runner_test.go:763-776`, unit tests mock Master returning `ConfigUnchanged: true, UsersUnchanged: false, Users: [...]` **with `ConfigJSON: ""`**!
  - And in `agent_e2e_test.go:418-438`, `TestMasterAgent_E2E_Integration` tests adding `bob@example.com` and checks `cmdRunner.reloadCount() == 1` (zero config reloads).
  - Therefore, **Agent must remain backward-compatible with an older Master** that returns `ConfigUnchanged: true, UsersUnchanged: false, ConfigJSON: ""` (gracefully falling back to gRPC-only or patching/skipping disk write when `ConfigJSON == ""`), while a **new Master** can supply `ConfigJSON` on `!UsersUnchanged` (or on an explicit `WantConfigJSON` / `DiskUsersHash` mismatch) so Agent writes `config.json` atomically (`xray run -test` -> `.bak` -> rename) **without** calling `reloadService` when `ConfigUnchanged == true`!
  - Note also: if Agent keeps an on-disk `config.json` that was updated by `ConfigJSON` during user-only updates, `ConfigHash` in `SyncRequest` must STILL report `snap.ConfigHash` (`structuralBytes` hash) so Master's `RefreshSyncStatus` (`domain/node.go:230`) continues to report `"synced"`! And Agent should verify `sha256(disk config.json)` (e.g., against an expected `fullConfigHash` persisted in `sync_state.json`, or by comparing `diskConfigHash`) to detect if disk `config.json` itself was edited, corrupted, or still holds the silence config!

### 5.3 Xray Restart & Runtime Drift Self-Healing Mechanisms
How should Agent detect that Xray restarted or drifted from expected users, and heal itself?

Let's evaluate the available mechanisms in this codebase:

1. **Xray gRPC `StatsService.GetSysStats` (Uptime / Restart Detection)**
   - **Availability**: Not in `internal/adapter/xray/proto/stats.proto` yet, but `StatsService` is **already enabled** in every Xray `config.json` (`compiler.go:88`, `config_manager.go:295`). Adding `GetSysStats(GetSysStatsRequest) returns (SysStatsResponse)` to `stats.proto` (returning `uptime` uint32 seconds, `num_goroutine`, etc.) is ~15 lines of standard Xray protobuf, zero new Go dependencies.
   - **How it works**: Every tick (and before/after `Probe`), Agent calls `GetSysStats`. If `uptime` decreases compared to the last observed `uptime` (or `uptime < expectedMin` / Xray PID/start-epoch changed), Xray definitely restarted!
   - **Portability**: Pure gRPC — works identically on systemd Linux, Docker/Alpine/OpenRC, bare binary, and Windows/macOS unit tests (mock gRPC server)! Compare `systemctl show -p ActiveEnterTimestamp` which breaks in Docker, non-systemd Linux, and Windows tests.
   - **Limitation**: `GetSysStats` alone only tells you Xray restarted; it does **not** detect partial gRPC failure drift or if Xray was running with an older `config.json` before Agent started (e.g. both Agent and Xray restarted, or Xray was restarted while Agent was briefly down, or someone manually tampered with Xray).

2. **Xray gRPC `HandlerService.GetInboundUsers` (Direct Runtime User Inspection)**
   - **Availability**: Not in `internal/adapter/xray/proto/proxyman.proto` yet, but `HandlerService` is **already enabled** in every Xray `config.json` (`compiler.go:88`, `config_manager.go:295`). Xray-core's `xray.app.proxyman.command.HandlerService` exposes `GetInboundUsers(GetInboundUsersRequest{Tag: tag}) returns (GetInboundUsersResponse{Users: repeated xray.common.protocol.User})`.
   - **Wait — check proto package naming & Xray-core compatibility!** Look carefully at `proxyman.proto:1-26` and `typed_message.go:9-17`:
     - In `typed_message.go:10-16`, `ToTypedMessage` maps local Go structs to Xray's canonical protobuf type URLs (`xray.app.proxyman.command.AddUserOperation`, etc.), while gRPC method paths in `proxyman_grpc.pb.go:22` use `"/xray.app.proxyman.command.HandlerService/AlterInbound"` (note `proxyman.proto:3` has `package xray.panel.proxyman;`, wait: how did `proxyman_grpc.pb.go:22` get `xray.app.proxyman.command.HandlerService`? Let's check — `proxyman_grpc.pb.go` was likely edited or generated from Xray's package name!).
     - In Xray-core's `HandlerService`, `GetInboundUsers` is `/xray.app.proxyman.command.HandlerService/GetInboundUsers`, taking `GetInboundUsersRequest{string tag = 1;}` and returning `GetInboundUsersResponse{repeated User users = 1;}` (where `User` has `level`, `email`, `account` as `TypedMessage`).
   - **Strength**: Directly queries Xray's **actual live runtime user list** per inbound tag!
     - If Xray restarted on a stale `config.json`, or started on silence config, or a previous `Reconcile` partially failed, or a user's UUID/Flow/Cipher changed, `GetInboundUsers(tag)` exposes the **exact runtime truth** (including missing users, zombie users, and credential/account mismatches via `TypedMessage.GetInstance()`!).
     - Even on **Agent startup**, querying `GetInboundUsers` + checking disk `config.json` allows Agent to rebuild `UserSyncer`'s baseline from Xray's actual runtime state — **zero unnecessary Xray restarts** when disk + runtime already match Master!
   - **Caveat / Edge Cases to verify**:
     - Non-UserManager inbounds (`dokodemo-door`, `socks`, `http`, `api`): `GetInboundUsers` returns an error (e.g., `proxy is not a UserManager` or `handler not found` if inbound missing). Agent must filter to `SupportsDynamicUser(protocol)` inbounds (and distinguish `handler not found` -> structural config out-of-sync!).
     - **Static clients in Xray runtime**: Remember `TrojanAdapter` (`<tag>@trojan.local`) and `mergeStaticClients` (`relay-auto:<tag>@internal`) ARE loaded into Xray's inbound runtime from `config.json`, so `GetInboundUsers` WILL return them! Agent's reconciler MUST **ignore/preserve** static clients (never `RemoveUser` them!) — e.g. by knowing static client emails from `config.json` or `relay-auto:*@internal` / `*.trojan.local` rules, or by tracking expected DB vs static sets. (Check `agent_e2e_test.go:1289-1315` which explicitly tests that `BuildRemoteNodeUsers` excludes `relay-auto:*` and user sync never removes it!)
     - **Xray version support**: `GetInboundUsers` has been in Xray-core `HandlerService` for years (since v1.8.x / v24+ / v25+ / v26+), whereas `GetSysStats` has existed since v2ray-core. Still, handling `codes.Unimplemented` gracefully as fallback is good defensive practice.

3. **Combined Strategy (`GetSysStats` + `GetInboundUsers` + Persistent Local State)**
   - **Fast-path / Periodic Drift Check**:
     - Persist last applied `ConfigHash`, `UsersHash`, `FullConfigHash` (sha256 of disk `config.json`), and `DesiredState` in `spoolDir/sync_state.json` (same atomic pattern as `QuotaStore`/`Spool`).
     - On every `SyncOnce` (even when Master replies `ConfigUnchanged: true, UsersUnchanged: true`), verify:
       1. Local disk `config.json` hash matches expected `FullConfigHash` (and file exists, isn't silence config when `DesiredState != circuit_broken`).
       2. Xray gRPC runtime matches expected state:
          - Lightweight check every tick: `GetSysStats()` checks `uptime` drop (restart detected) or gRPC reachability, AND/OR `GetInboundUsers()` per dynamic inbound verifies live runtime users match desired users (detects both Xray restart AND any partial/manual drift).
          - Note: `GetInboundUsers` is a tiny local loopback gRPC call (`127.0.0.1:8080`) once every 60s; for typical 1–10 inbounds and tens/hundreds of users it takes <1–2ms. Comparing live runtime users against Master's desired `Users` (or cached desired `Users` persisted in `sync_state.json` when Master short-circuits!) enables **instant self-healing on the very next tick** AND **even when Master is temporarily unreachable** (if desired users + `config.json` are cached locally)!
     - Note the subtle consequence of Master short-circuiting: When Master returns `ConfigUnchanged: true, UsersUnchanged: true`, `resp.Users` is **empty** (`node_service.go:909-911`)! So if Xray restarts and Agent wants to heal Xray without forcing Master to resend `Users` on every tick, Agent MUST either:
       - Persist the last received `Users` (or extract dynamic users / keep `sync_state.json` with `ConfigHash`, `UsersHash`, `FullConfigHash`, `DesiredUsers`) on disk, **OR**
       - Clear `r.usersHash = ""` / `r.configHash = ""` upon detecting Xray restart/drift so the next (or immediate) `SyncOnce` re-fetches `Users` (and `ConfigJSON`) from Master!
       - Combining both (persisting `DesiredUsers` + `FullConfigHash` in `sync_state.json` for immediate local self-healing + resetting hashes to re-pull from Master if local state is missing/corrupted) gives **1-tick / offline-capable self-healing** with **zero extra Master load** in steady state!

---

## 6. Edge Cases & Failure Modes Checklist

1. **Xray down or restarting during `SyncOnce`**:
   - If Xray is down when `CollectTrafficStats` runs (`runner.go:167`), it logs a warning and proceeds with `Sync`.
   - If `reconcileSyncResponse` runs `UserSyncer.Reconcile` while Xray is down/restarting, gRPC returns `Unavailable` / `connection refused`. Agent must **not** advance `usersHash` (or `sync_state.json`), must set `lastError`, and **must NOT** mark failed users in `applied`.
   - Crucially: fixing `user_sync.go:106` so `AddUser` does NOT ignore `handler not found` / `not found` prevents poisoning `applied` if Xray is mid-restart or running a silence config.
2. **Ordering: `config.json` write + `xray run -test` vs gRPC `AlterInbound`**:
   - Suppose only users changed (`ConfigUnchanged == true, UsersUnchanged == false`).
   - If Agent receives `ConfigJSON` (or updates disk `config.json`) **before** gRPC `Reconcile`:
     - Step 1: Validate `ConfigJSON` with `xray run -test -c <tmp>` (`config_manager.go:105`). If `xray run -test` fails, abort **before** overwriting `config.json` and **before** mutating Xray runtime! (Keeps disk and runtime consistent, prevents saving broken config).
     - Step 2: Atomically backup `config.json` -> `.bak` and rename `.tmp.json` -> `config.json` (NO `reloadService`!).
     - Step 3: Execute gRPC `UserSyncer.Reconcile`. If gRPC fails (e.g., Xray restarted during Step 2 or transient error), Xray on disk already has the new users (good on next restart), and `usersHash` is NOT updated so next tick retries gRPC reconciliation against live runtime! (Or if `GetInboundUsers` shows runtime already differs, next tick heals it.)
   - Conversely, if gRPC ran first and `config.json` write failed, runtime and disk would diverge; validating + writing `config.json` first (with `xray -test` gate) guarantees `config.json` is valid and persisted before acknowledging `usersHash`.
   - Note on `ConfigManager.Apply` vs disk-only persist: `ConfigManager` currently has `Apply` (write + test + backup + rename + reload + probe) and `ReloadAndProbe`. Adding a `PersistOnly` / `WriteAndValidate` method on `ConfigManager` (guarded by `m.mu.Lock()`) reuses `tempConfigPath`, `writeFileAtomicSync`, `xray run -test`, and `.bak` backup without duplicating file-locking or atomic-write logic!
3. **`xray run -test` failure**:
   - Currently tested in `config_manager_test.go:102` (`TestConfigManager_XrayTestFailureDoesNotOverwrite`): `config.json` must remain untouched, error is returned and reported via `lastError`.
4. **Empty user list (`len(Users) == 0`)**:
   - Notice `compiler.go:639-641` & `protocol_adapter.go:159`: when active users = 0, `CompileClients` returns non-nil empty slice `[]XrayClient{}` so `settings.clients` is `[]` (Zero Clients Retention invariant in `error-handling.md:64`).
   - In `AgentSyncResponse` (`domain/node.go:407`), `Users` has `json:"users,omitempty"`. When Master has 0 users and `!UsersUnchanged`, `Users` serializes as omitted/empty (`nil` on Agent decode). `UserSyncer.Reconcile(ctx, nil)` MUST remove all previously applied dynamic users (while preserving static `relay-auto:*` / `@trojan.local` clients!).
5. **Multiple inbounds & same user email across multiple inbounds**:
   - `userSyncKey` (`user_sync.go:35-37`) uses `inboundTag + "\x00" + email`, correctly scoping users per inbound tag.
   - `GetInboundUsers` must be queried per dynamic inbound tag (`inbound.Tag`). If a user belongs to 2 inbounds and was added to inbound A but failed on inbound B, per-inbound inspection catches the partial drift on inbound B.
6. **Non-user inbounds (`api`, `dokodemo-door`, `socks`, `http`)**:
   - Master's `BuildRemoteNodeUsers` (`compiler.go:441`) already skips `!domain.SupportsDynamicUser(inb.Protocol)` and `Tag == "api"`.
   - Agent's `GetInboundUsers` check must likewise skip `api` and non-dynamic protocols (and if `GetInboundUsers` returns `not a usermanager`, treat as non-dynamic).
7. **Concurrent `geodata_update` vs `ConfigManager.Apply` / `SyncOnce`**:
   - `HTTPGeoDataUpdater.Update` (`geodata_updater.go:94-152`) runs in a background goroutine (`runner.go:365-439`) and calls `cfgMgr.ReloadAndProbe` (`143`), which acquires `cfgMgr.mu` (`config_manager.go:160`).
   - If `geodata_update` reloads Xray right after a user hot-update:
     - With disk `config.json` kept up to date, `ReloadAndProbe` loads the up-to-date `config.json` (no user loss!).
     - Even if `ReloadAndProbe` raced with an in-flight `Reconcile`, the next `SyncOnce` uptime/runtime check heals any drift.
8. **Circuit breaker & offline recovery**:
   - `ApplyCircuitBreaker` (`config_manager.go:150-156`) overwrites `config.json` with `silenceCfg` (only `api` inbound).
   - While `circuitBroken == true` (either online via `DesiredStateCircuitBroken` or offline via `QuotaStore.LocalCircuitBroken` at `runner.go:592-608`), Agent must **not** try to sync business users into Xray (there are no business inbounds!).
   - Also note: on Agent restart while `QuotaStore.State().LocalCircuitBroken == true` and Master is unreachable, `Runner.circuitBroken` initializes to `false` (`runner.go:43`). Syncing `Runner.circuitBroken` / `sync_state.json` with `QuotaStore` on startup prevents stale state confusion.
   - When recovering from `circuit_broken` -> `active`, Master returns `!ConfigUnchanged` + `ConfigJSON` (because `Runner` cleared `configHash` & `usersHash` at `runner.go:247-248, 601-602`), restoring full business `config.json` and resetting `UserSyncer` baseline.
9. **Rolling upgrade compatibility (Old Master + New Agent, New Master + Old Agent)**:
   - **New Master + Old Agent**: Old Agent sends `SyncRequest{ConfigHash, UsersHash}` without any new fields. Master continues computing `ConfigHash = sha256(structuralBytes)` and `UsersHash = sha256(usersBytes)` and returning `ConfigUnchanged` / `UsersUnchanged` as before. Even if Master optionally includes `ConfigJSON` when `!UsersUnchanged`, wait: check Old Agent (`runner.go:258`)!
     - **CRITICAL COMPATIBILITY TRAP**: Look at `runner.go:258` in the current (old) Agent:
       ```go
       if !resp.ConfigUnchanged && resp.ConfigJSON != "" {
           if applyErr := r.cfgMgr.Apply(ctx, resp.ConfigJSON); applyErr != nil { ... }
       ```
       Old Agent guards `Apply` with `!resp.ConfigUnchanged && resp.ConfigJSON != ""`! So as long as Master keeps `ConfigUnchanged: true` when `ConfigHash == snap.ConfigHash`, an Old Agent ignores `ConfigJSON` (does NOT restart Xray) and runs `if !resp.UsersUnchanged` (`276`)!
       HOWEVER, check existing unit tests on Master/HTTP: in `agent_e2e_test.go:414` and `node_service_test.go:256`, `ConfigJSON == ""` is asserted when BOTH `ConfigUnchanged && UsersUnchanged` are true. When `ConfigUnchanged == true && UsersUnchanged == false`, sending `ConfigJSON: snap.ConfigJSON` (or only when requested via a new `SyncRequest` field like `WantConfigOnUserChange bool` or `DiskConfigHash string`) avoids changing payload size for Old Agents altogether!
     - Using an explicit opt-in capability field on `SyncRequest` (e.g., `DiskConfigHash string` or `SupportHotPersist bool` / `WantFullConfig bool`) makes **New Master + Old Agent** 100% wire-identical to today, while **New Master + New Agent** gets `ConfigJSON` whenever disk `config.json` needs updating!
   - **Old Master + New Agent**: Old Master does not know about new `SyncRequest` fields (Go JSON unmarshaler ignores them) and returns `ConfigUnchanged: true, UsersUnchanged: false, ConfigJSON: ""` on user changes. New Agent must handle `ConfigJSON == ""` gracefully (still performs gRPC `Reconcile` and runtime self-healing via `GetInboundUsers`/`GetSysStats`, and optionally falls back or logs that disk persistence requires updated Master — or if `Users` changed and `ConfigJSON == ""`, only updates Xray runtime via gRPC without crashing).

---

## 7. Key Files & Line Anchors Summary

- `go.mod:15-16`
- `cmd/agent/main.go:37-146`
- `internal/adapter/xray/proto/proxyman.proto:9-26`, `proxyman_grpc.pb.go:21-30`
- `internal/adapter/xray/proto/stats.proto:6-22`, `stats_grpc.pb.go:21-30`
- `internal/adapter/xray/proto/typed_message.go:9-45`
- `internal/adapter/xray/grpc_client.go:18-223`
- `internal/adapter/xray/manager.go:36-77`
- `internal/adapter/xray/compiler.go:86-90, 349, 366-468, 474-664`
- `internal/adapter/xray/protocol_adapter.go:136-443`
- `internal/domain/inbound.go:86-93`
- `internal/domain/node.go:107-235, 356-410`
- `internal/delivery/http/handler_agent.go:22-54`, `router.go:86-93`
- `internal/service/node_service.go:244-280, 785-1015, 1502-1594`
- `internal/service/config_service.go:101-111`
- `internal/service/user_service.go:82-108`
- `internal/adapter/repository/node_repo.go:70-122`
- `internal/agent/protocol.go:17-113`
- `internal/agent/client.go:16-136`
- `internal/agent/config_manager.go:38-330`
- `internal/agent/user_sync.go:18-129`
- `internal/agent/xray_grpc_adapter.go:21-131`
- `internal/agent/runner.go:25-627`
- `internal/agent/geodata_updater.go:94-152`
- `internal/agent/spool.go:17-144`, `internal/agent/quota_store.go:25-201`
