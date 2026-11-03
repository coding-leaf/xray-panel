# Codebase and Xray Binary Comprehensive Audit Report

**Date**: 2026-11-03  
**Auditor**: `trellis-research` Sub-agent  
**Scope**: Codebase audit (`D:\code\Workspace\xray-panel`) & Xray-core binary inspection for Outbound Mux support and config schema alignment.  
**Strict Constraint**: Read-only on source code; output persisted exclusively to research documentation.

---

## 1. Executive Summary & Binary Audit

### 1.1 Local Binary Audit Results
- **Location**: `./temp/xray.exe` (Discovered in repository `temp/` folder, ignored by `.gitignore:42`).
- **Core Version**: `Xray 26.10.31 (Xray, Penetrates Everything.) a4a3dff (go1.27.3 windows/amd64)`
- **CLI Test Validation (`xray run -test -c <tmpPath>`)**:
  - Outbound with `mux` (`enabled: true`, `concurrency: 8`, `xudpConcurrency: 16`, `xudpProxyUDP443: "reject"`) passes syntax validation cleanly (`Configuration OK.`).
  - Outbound with `mux.concurrency: -1` (disables TCP mux) and `mux.xudpProxyUDP443: "skip"` passes syntax validation cleanly.
  - Proxy chaining with `streamSettings.sockopt.dialerProxy` passes validation cleanly.
  - **FATAL REMOVED FEATURE DETECTED**: Legacy `outbound.proxySettings` fails immediately with exit code 23:
    ```
    Failed to start: ... > common/errors: The feature outbound "proxySettings" has been removed and migrated to "streamSettings.sockopt.dialerProxy". Please update your config(s) according to release note and documentation.
    ```
  - `outbound.targetStrategy` on non-freedom outbounds is accepted; on `freedom` outbounds, Xray logs an auto-migration warning:
    ```
    [Warning] infra/conf: The "outbound.targetStrategy" setting is not supported directly by freedom and has been automatically migrated to "sockopt.domainStrategy" with no behavior change.
    ```
  - Inbound and Outbound `streamSettings.sockopt.acceptProxyProtocol: true` passes validation cleanly.
  - Inbound `sniffing` with `metadataOnly: true`, `domainsExcluded: [...]`, `ipsExcluded: [...]` passes validation cleanly.
  - Top-level `fakedns`, `observatory`, `burstObservatory`, and `routing.balancers` pass validation cleanly.

---

## 2. Category A: Silent Data Loss / Struct Stripping during Recompile Pipeline

### 2.1 The Destruction Mechanism
When a user edits `config.json` via the Raw JSON Editor (`web/src/views/ConfigView.vue`) or imports an external configuration, the file initially contains advanced Xray configurations (e.g. `mux`, `dialerProxy`, `sendThrough`, `fakedns`, `observatory`).
However, **as soon as ANY normal panel operation runs** (such as toggling a user, adding/updating an inbound, saving an outbound, updating routing, or changing DNS):
1. `ConfigService.SaveConfigQuietly` (`internal/service/config_service.go:126`) or `ConfigService.RecompileAndApply` (`internal/service/config_service.go:198`) or `saveOutboundsListAndRecompile` (`internal/service/config_service.go:491`) is invoked.
2. `ConfigService.ListOutbounds` (`internal/service/config_service.go:421-450`) reads the raw JSON from disk using a hardcoded, incomplete inline Go struct:
   ```go
   // internal/service/config_service.go:428-435
   var root struct {
       Outbounds []struct {
           Tag            string          `json:"tag"`
           Protocol       string          `json:"protocol"`
           Settings       json.RawMessage `json:"settings"`
           StreamSettings json.RawMessage `json:"streamSettings"`
       } `json:"outbounds"`
   }
   ```
   **Immediate Loss**: `mux`, `sendThrough`, `targetStrategy` are ignored and discarded at JSON parse time.
3. The parsed outbounds are converted into `domain.Outbound` (`internal/domain/outbound.go:3-8`):
   ```go
   // internal/domain/outbound.go:3-8
   type Outbound struct {
       Tag            string `json:"tag"`
       Protocol       string `json:"protocol"`
       SettingsJSON   string `json:"settingsJson"`
       StreamSettings string `json:"streamSettings,omitempty"`
   }
   ```
   `domain.Outbound` possesses no `Mux`, `SendThrough`, or `TargetStrategy` fields.
4. `ConfigService` calls `compiler.CompileToJSON(inbounds, outbounds, routing, dns, users)`.
5. Inside `XrayCompiler.Compile` (`internal/adapter/xray/compiler.go:88-272`):
   - A fresh root config object `&XrayConfigFile{...}` is synthesized from scratch (`compiler.go:95-121`). Any top-level section not defined on `XrayConfigFile` (`fakedns`, `observatory`, `burstObservatory`, `transport`, `metrics`, `reverse`) is **silently deleted**.
   - `compileOutbound` (`compiler.go:392-429`) unmarshals `ob.StreamSettings` into `*XrayStreamSettings` (`internal/adapter/xray/schema.go:74-86`):
     - `XraySockopt` (`schema.go:127-131`) **lacks `dialerProxy`**! Any proxy chaining tag set by the user is **silently deleted**.
     - `XrayStreamSettings` **lacks `tcpSettings` (and `rawSettings`)**, `splithttpSettings`, `hysteriaSettings`, `masqueSettings`. Custom TCP HTTP obfuscation headers or SplitHTTP configurations are **silently deleted**.
   - `compileOutbound` builds `&XrayOutbound{Tag, Protocol, Settings, StreamSettings}`. `XrayOutbound` (`schema.go:142-147`) has no `mux`, `sendThrough`, or `targetStrategy`.
   - `compileInbound` (`compiler.go:287-390`) unmarshals `inb.SniffingJSON` into `*XraySniffingConfig` (`schema.go:68-72`). Fields `metadataOnly`, `domainsExcluded`, and `ipsExcluded` are not in the struct and are **silently deleted**.
6. The compiled JSON is serialized and written to disk via `configMgr.WriteConfig(ctx, jsonBytes)` (`config_service.go:189`, `518`), permanently erasing the dropped fields.

---

### 2.2 Exhaustive Codebase Gap Inventory (Category A)

#### Gap A.1: Outbound Mux Configuration (`mux`)
- **Missing In**:
  - `internal/domain/outbound.go:3-8` (`domain.Outbound` struct)
  - `internal/adapter/xray/schema.go:142-147` (`XrayOutbound` struct)
  - `internal/adapter/xray/schema.go` (No `XrayMuxConfig` struct exists)
  - `internal/service/config_service.go:428-435` (`ListOutbounds` inline parser)
  - `internal/adapter/xray/compiler.go:392-429` (`compileOutbound`)
- **Impact**: Any `mux` block configured on outbounds is 100% wiped upon any recompile, breaking multiplexing for VLESS/VMess/Trojan/Shadowsocks.

#### Gap A.2: Outbound Chaining via `sockopt.dialerProxy`
- **Missing In**:
  - `internal/adapter/xray/schema.go:127-131` (`XraySockopt` struct only defines `Mark`, `Tproxy`, `TCPFastOpen`).
- **Impact**: Modern Xray-core removed `proxySettings` and requires `streamSettings.sockopt.dialerProxy` for proxy chaining (`Client -> Outbound A -> Outbound B -> Dest`). Because `dialerProxy` is missing from `XraySockopt`, any chained outbound has its `dialerProxy` stripped on recompile, breaking upstream relay tunnels.

#### Gap A.3: Outbound `sendThrough` & `targetStrategy`
- **Missing In**:
  - `internal/domain/outbound.go:3-8`
  - `internal/adapter/xray/schema.go:142-147`
  - `internal/service/config_service.go:428-435`
  - `internal/adapter/xray/compiler.go:392-429`
- **Impact**: Multi-homed server IP binding (`sendThrough: "1.2.3.4"` or `"origin"`) and DNS resolution strategy on outbounds (`targetStrategy: "UseIPv4"`) are dropped.

#### Gap A.4: Inbound PROXY Protocol (`sockopt.acceptProxyProtocol`)
- **Missing In**:
  - `internal/adapter/xray/schema.go:127-131` (`XraySockopt`)
- **Impact**: Inbounds running behind HAProxy, Nginx stream reverse proxy, or Cloudflare Spectrum requiring `acceptProxyProtocol: true` lose this setting whenever `compileInbound` unmarshals `inb.StreamSettings` into `XrayStreamSettings`.

#### Gap A.5: Inbound & Outbound `streamSettings.tcpSettings` / `rawSettings`
- **Missing In**:
  - `internal/adapter/xray/schema.go:74-86` (`XrayStreamSettings`)
- **Impact**: TCP stream configurations with fake HTTP header obfuscation (`header: { type: "http", request: ... }`) are discarded because `XrayStreamSettings` does not declare `TCPSettings *XrayTCPConfig`.

#### Gap A.6: Inbound Sniffing Extensions (`metadataOnly`, `domainsExcluded`, `ipsExcluded`)
- **Missing In**:
  - `internal/adapter/xray/schema.go:68-72` (`XraySniffingConfig`)
- **Current State**:
  ```go
  type XraySniffingConfig struct {
      Enabled      bool     `json:"enabled"`
      DestOverride []string `json:"destOverride,omitempty"`
      RouteOnly    bool     `json:"routeOnly,omitempty"`
  }
  ```
- **Impact**: Sniffing exclusion rules and metadata-only sniffing (essential when sniffing protocols with delayed client payload or fakeDNS) are stripped.

#### Gap A.7: Post-Quantum REALITY & Client Aliases (`password`, `mldsa65Seed`, `mldsa65Verify`)
- **Missing In**:
  - `internal/adapter/xray/schema.go:102-117` (`XrayRealitySettings`)
- **Impact**: Modern clients use `password` as alias for `publicKey`. Modern Xray post-quantum extensions (`mldsa65Seed`, `mldsa65Verify`) are stripped if present.

#### Gap A.8: Routing Rule Source Matching & Balancer Tags (`source`, `sourcePort`, `balancerTag`, `domainMatcher`)
- **Missing In**:
  - `internal/domain/outbound.go:10-23` (`domain.RoutingRule`)
  - `internal/adapter/xray/schema.go:149-163` (`XrayRoutingRule`)
- **Impact**: Rules filtering by client LAN subnet (`source: ["192.168.1.0/24"]`), source port, or routing to a balancer group (`balancerTag`) are stripped.

#### Gap A.9: Top-Level Modules in `XrayConfigFile` (`fakedns`, `observatory`, `burstObservatory`, `transport`, `metrics`)
- **Missing In**:
  - `internal/adapter/xray/schema.go:6-15` (`XrayConfigFile`)
- **Impact**: Panel completely discards FakeDNS and health-check observatory pools on recompile.

---

## 3. Category B: Missing Frontend UI Display / Editing

### 3.1 Outbound Form & Types (`web/src/views/outbounds/`)

#### 1. Mux Multiplexing Configuration Missing
- **Files**:
  - `web/src/views/outbounds/types.ts:8-43` (`OutboundFormState`)
  - `web/src/views/outbounds/types.ts:52-94` (`createDefaultOutboundForm`)
  - `web/src/views/outbounds/types.ts:96-200` (`populateOutboundForm`)
  - `web/src/views/outbounds/types.ts:373-379` (`sanitizeOutboundPayload`)
  - `web/src/views/outbounds/components/OutboundFormDrawer.vue:1-460`
- **Findings**:
  - No Mux fields exist in `OutboundFormState`.
  - No form section or controls exist for:
    - `muxEnabled`: Toggle switch (boolean)
    - `muxConcurrency`: Number input (`1` ~ `128`, default `8`, `-1` to disable TCP mux)
    - `muxXudpConcurrency`: Number input (`1` ~ `1024`, default `16`, `-1` to disable UDP mux)
    - `muxXudpProxyUDP443`: Dropdown (`"reject"` | `"allow"` | `"skip"`, default `"reject"`)
  - **Incompatibility with Vision**: `xtls-rprx-vision` strictly conflicts with Mux. Currently, `OutboundFormDrawer.vue` does not warn or prevent enabling both simultaneously.

#### 2. Outbound Proxy Chaining (`dialerProxy`) Missing
- **Files**: `web/src/views/outbounds/components/OutboundFormDrawer.vue`, `types.ts`
- **Findings**: No dropdown selector to choose an existing outbound tag as `dialerProxy` (for example, chaining an outbound through `warp-out` or an upstream SOCKS proxy).

#### 3. Outbound `sendThrough` Missing
- **Files**: `web/src/views/outbounds/components/OutboundFormDrawer.vue`, `types.ts`
- **Findings**: No input field for binding the local exit IP or interface.

### 3.2 Outbound Display & Inspection (`OutboundTable.vue` & `OutboundDetailDrawer.vue`)
- **Files**:
  - `web/src/views/outbounds/components/OutboundTable.vue:104-120` (Network & Security column)
  - `web/src/views/outbounds/components/OutboundTable.vue:128-132` (Settings summary)
  - `web/src/views/outbounds/components/OutboundDetailDrawer.vue:55-87` (StreamSettings card)
  - `web/src/views/outbounds/types.ts:452-466` (`formatSettingsSummary`)
- **Findings**:
  - `OutboundTable.vue` displays network and security badges, but has no indicator for whether Mux is enabled (e.g. `MUX: 8` or `XUDP: 16`).
  - `OutboundDetailDrawer.vue` lacks a dedicated card/section displaying Mux parameters and dialerProxy chaining target.
  - `formatSettingsSummary` omits Mux information.

### 3.3 Inbound Form & Types (`web/src/views/inbounds/`)
- **Files**:
  - `web/src/views/inbounds/types.ts:48-115` (`InboundFormData`)
  - `web/src/views/inbounds/components/InboundFormDrawer.vue`
- **Findings**:
  - **Missing PROXY Protocol Support**: No checkbox for `acceptProxyProtocol` under StreamSettings/Sockopt.
  - **Missing Sniffing Advanced Controls**: Sniffing has `routeOnly` and `destOverride`, but lacks `metadataOnly` and exclusion inputs (`domainsExcluded`, `ipsExcluded`).

### 3.4 Topology Quick Publish Wizard (`web/src/views/topology/`)
- **Files**:
  - `web/src/views/topology/components/QuickPublishWizardModal.vue:250-375` (`parseNodeLink`)
- **Findings**:
  - When importing a node link (`vless://`, `trojan://`, `ss://`), `parsedOutboundPayload` constructs `{ tag, protocol, settingsJson, streamSettings }` without Mux configuration options.

### 3.5 Routing View (`web/src/views/RoutingView.vue`)
- **Files**:
  - `web/src/views/RoutingView.vue:550-650` (Rule form drawer)
  - `web/src/views/RoutingView.vue:860-910` (Rule form state)
- **Findings**:
  - Routing rule drawer has inputs for `tag`, `inboundTag`, `outboundTag`, `vlessRoute`, `user`, `domain`, `ip`, `port`, `network`, `protocol`.
  - Missing inputs for `source` (Client IP CIDR), `sourcePort`, `balancerTag`, and `domainMatcher`.

---

## 4. Exact Schema Alignment Matrix

| Object / Scope | Field Name | Type | In Xray-core | In `schema.go` | In `domain` | In Frontend UI | Status / Risk |
|---|---|---|---|---|---|---|---|
| **Outbound** | `mux.enabled` | `bool` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `mux.concurrency` | `int16` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `mux.xudpConcurrency` | `int16` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `mux.xudpProxyUDP443` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `sendThrough` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `targetStrategy` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss** |
| **Outbound** | `proxySettings` | `object` | ❌ **REMOVED** | ❌ (Omitted) | ❌ (Omitted) | ❌ (Omitted) | **Correctly Avoided** |
| **Sockopt** | `dialerProxy` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss (Proxy Chaining)** |
| **Sockopt** | `acceptProxyProtocol` | `bool` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss (PROXY Protocol)** |
| **Sockopt** | `domainStrategy` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `tcpKeepAliveInterval`| `int32` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `tcpKeepAliveIdle` | `int32` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `tcpUserTimeout` | `int32` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `tcpCongestion` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `interface` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sockopt** | `tcpMptcp` | `bool` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **StreamSettings** | `tcpSettings` (`rawSettings`) | `object` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | **Silent Data Loss (HTTP Obfuscation)** |
| **StreamSettings** | `splithttpSettings` | `object` | Yes (Alias) | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sniffing** | `metadataOnly` | `bool` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sniffing** | `domainsExcluded` | `[]string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **Sniffing** | `ipsExcluded` | `[]string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Silent Loss |
| **REALITY** | `password` | `string` | Yes (Alias) | ❌ Missing | ❌ Missing | ❌ Missing | Modern Client Alias |
| **REALITY** | `mldsa65Seed` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Post-Quantum Seed |
| **REALITY** | `mldsa65Verify` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Post-Quantum Verify |
| **RoutingRule** | `source` | `[]string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Rule Match Gap |
| **RoutingRule** | `sourcePort` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Rule Match Gap |
| **RoutingRule** | `balancerTag` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Load Balancing Gap |
| **RoutingRule** | `domainMatcher` | `string` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Matcher Strategy Gap |
| **Root Config** | `fakedns` | `array` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Top-Level Loss |
| **Root Config** | `observatory` | `object` | Yes | ❌ Missing | ❌ Missing | ❌ Missing | Top-Level Loss |

---

## 5. Architectural Recommendations for Implementation

### Phase 1: Backend Domain & Schema Foundation
1. **Define `XrayMuxConfig` & `domain.MuxConfig`**:
   ```go
   type XrayMuxConfig struct {
       Enabled         bool   `json:"enabled"`
       Concurrency     int16  `json:"concurrency,omitempty"`
       XudpConcurrency int16  `json:"xudpConcurrency,omitempty"`
       XudpProxyUDP443 string `json:"xudpProxyUDP443,omitempty"`
   }
   ```
2. **Update `domain.Outbound`** (`internal/domain/outbound.go`):
   - Add `SendThrough string `json:"sendThrough,omitempty"``
   - Add `Mux string `json:"mux,omitempty"`` (or structured JSON string representation, matching `SettingsJSON` / `StreamSettings` style).
3. **Update `XrayOutbound`** (`internal/adapter/xray/schema.go`):
   - Add `SendThrough string `json:"sendThrough,omitempty"``
   - Add `TargetStrategy string `json:"targetStrategy,omitempty"``
   - Add `Mux *XrayMuxConfig `json:"mux,omitempty"``
4. **Update `XraySockopt`** (`internal/adapter/xray/schema.go`):
   - Add `DialerProxy string `json:"dialerProxy,omitempty"``
   - Add `AcceptProxyProtocol bool `json:"acceptProxyProtocol,omitempty"``
   - Add `DomainStrategy string `json:"domainStrategy,omitempty"``
5. **Update `ConfigService.ListOutbounds`** (`internal/service/config_service.go:428-445`):
   - Expand the inline unmarshal struct to parse `mux`, `sendThrough`, and `targetStrategy`.
   - Populate these fields into the returned `domain.Outbound` slice.
6. **Update `XrayCompiler.compileOutbound`** (`internal/adapter/xray/compiler.go:392-429`):
   - Unmarshal `ob.Mux` into `XrayMuxConfig`.
   - Enforce invariant: if `flow: "xtls-rprx-vision"`, strip or reject `Mux` to avoid Xray runtime stalling.
   - Attach `SendThrough` and `Mux` to `XrayOutbound`.
7. **Add Unit & Integration Tests** (`internal/adapter/xray/compiler_test.go`):
   - Test round-trip compile with Mux and XUDP settings.
   - Run `validateWithXrayCore(t, jsonBytes)` against `./temp/xray.exe`.

### Phase 2: Frontend UI Alignment
1. **Extend `OutboundFormState` & Serialization** (`web/src/views/outbounds/types.ts`):
   - Add Mux fields: `muxEnabled`, `muxConcurrency`, `muxXudpConcurrency`, `muxXudpProxyUDP443`.
   - Add `dialerProxy` and `sendThrough`.
   - Update `createDefaultOutboundForm`, `populateOutboundForm`, `buildStreamSettingsJSON`, and `sanitizeOutboundPayload`.
2. **Add Mux & Advanced Drawer Section** (`web/src/views/outbounds/components/OutboundFormDrawer.vue`):
   - Provide an expandable "连接复用 (Mux & XUDP)" SectionCard for `vless`, `vmess`, `trojan`, `shadowsocks`.
   - Include concurrency number inputs and `xudpProxyUDP443` dropdown.
   - Add mutual exclusivity validation: when `vlessFlow === 'xtls-rprx-vision'`, show an alert or automatically set `vlessFlow = 'none'` when enabling Mux.
3. **Enhance Detail & Table Display** (`OutboundTable.vue` & `OutboundDetailDrawer.vue`):
   - Display `MUX` badge on table rows when enabled.
   - Show Mux parameters and `dialerProxy` in detail drawer.

---

## 6. Verification Plan Against Real Binary
- Run `mise x -- go test -v ./internal/adapter/xray/...` with `./temp/xray.exe` present.
- Verify `ListOutbounds -> SaveOutbound -> CompileToJSON` retains `mux` and `dialerProxy` without data degradation.
- Verify frontend production build: `cd web && mise x -- npm run build`.
