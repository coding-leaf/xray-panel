# Research: Xray-core Official Configuration Specification Reference

## Summary
Official Xray-core configuration specifications (sourced directly from `xtls.github.io/config/` and `github.com/XTLS/Xray-core` `infra/conf`) define exact schemas for `OutboundObject`, `InboundObject`, `StreamSettingsObject`, `SockoptObject`, and `MuxObject`. Notably, legacy `proxySettings` in `outbounds` has been removed and replaced with `streamSettings.sockopt.dialerProxy` for proxy chaining; Mux + XUDP enables concurrent TCP and UDP multiplexing but is strictly mutually exclusive with VLESS `xtls-rprx-vision`; and REALITY supports enhanced fields like `target` (alias for `dest`), `password` (alias for `publicKey`), and ML-DSA-65 post-quantum extensions.

---

## Findings

### 1. OutboundObject (`outbounds`) & Mux Specification

#### 1.1 OutboundObject Top-Level Fields
In Xray-core (`infra/conf/xray.go: OutboundDetourConfig`), each outbound entry supports the following fields:
- `sendThrough` (string): Outbound IP or IPv6 CIDR block (e.g. `2001:db8::/64`), or special values `"origin"` (binds to the inbound target IP) and `"srcip"` (binds to inbound client source IP). Default is `"0.0.0.0"`.
- `protocol` (string): Connection protocol (`"freedom"`, `"vless"`, `"vmess"`, `"trojan"`, `"shadowsocks"`, `"socks"`, `"http"`, `"blackhole"`, `"dns"`, `"loopback"`, `"hysteria"`, `"wireguard"`, `"masque"`).
- `settings` (object): Protocol-specific configuration (`VLessOutboundConfig`, `FreedomConfig`, etc.).
- `tag` (string): Unique identifier for routing and chaining.
- `streamSettings` (`StreamConfig`): Transport and underlying socket options.
- `mux` (`MuxConfig`): Connection multiplexing configuration.
- `targetStrategy` (string): Controls whether proxied target domain is resolved locally by Xray: `"AsIs"` (default), `"UseIP"`, `"UseIPv4"`, `"UseIPv6"`, `"UseIPv4v6"`, `"UseIPv6v4"`, `"ForceIP"`, `"ForceIPv4"`, `"ForceIPv6"`, `"ForceIPv4v6"`, `"ForceIPv6v4"`. Note: for Freedom outbound, this is automatically translated into `sockopt.domainStrategy`.
- `proxySettings` (deprecated/removed): **REMOVED FEATURE**. If present in config, Xray-core parser returns an explicit error: `errors.PrintRemovedFeatureError("outbound \"proxySettings\"", "\"streamSettings.sockopt.dialerProxy\"")`.

#### 1.2 MuxObject (`mux`) Exact Schema & Semantics
From `infra/conf/xray.go`:
```go
type MuxConfig struct {
    Enabled         bool   `json:"enabled"`
    Concurrency     int16  `json:"concurrency"`
    XudpConcurrency int16  `json:"xudpConcurrency"`
    XudpProxyUDP443 string `json:"xudpProxyUDP443"`
}
```
- `enabled` (bool): Whether to enable Mux forwarding (default `false`). Required on client; server adapts automatically.
- `concurrency` (int16): Maximum concurrent TCP sub-connections multiplexed onto a single TCP connection.
  - Range: `1` to `128`.
  - Default: `8` (if omitted or set to `0`). Values `> 128` are capped at `128`.
  - **Negative value behavior (`-1`)**: Disables Mux for TCP traffic. TCP connections will bypass Mux and travel natively according to the protocol.
- `xudpConcurrency` (int16): Maximum concurrent UDP (UoT) sub-connections multiplexed over an XUDP tunnel.
  - Range: `1` to `1024`.
  - Default: `0` (or omitted), meaning UDP traffic shares the same Mux connection as TCP.
  - **Negative value behavior (`-1`)**: Disables Mux for UDP traffic. UDP traffic will bypass Mux and use the protocol's native UDP mechanism (e.g., native UDP for Shadowsocks, UoT for VLESS).
- `xudpProxyUDP443` (string): Controls handling of UDP port 443 (QUIC) traffic:
  - `"reject"` (default): Drops UDP/443 packets so standard browsers gracefully fall back to HTTP/2 over TCP.
  - `"allow"`: Multiplexes UDP/443 over Mux/XUDP.
  - `"skip"`: Bypasses Mux for UDP/443 and falls back to native protocol UDP transport.

#### 1.3 Mux + XUDP with VLESS + TCP + Reality
- **Compatibility Constraint with Vision**: In Xray-core architecture, `xtls-rprx-vision` requires raw, un-multiplexed TLS byte streams to inspect inner TLS Handshake patterns and perform flow splicing (padding / direct read). Therefore, **Mux is strictly incompatible with `xtls-rprx-vision`**.
- **Usage without Vision (`flow: ""` or none)**: VLESS + TCP + REALITY works seamlessly with Mux + XUDP. In this mode:
  - All TCP streams are multiplexed into one or more TLS/Reality connections (`concurrency: 8`).
  - UDP streams are packet-multiplexed into XUDP tunnels (`xudpConcurrency: 16`), mitigating UDP handshake latency and packet loss over restrictive firewalls.
  - `xudpProxyUDP443: "reject"` prevents QUIC race stalls.

---

### 2. StreamSettingsObject (`streamSettings`) & `sockopt`

#### 2.1 StreamSettingsObject Top-Level Fields
From `infra/conf/transport_internet.go`:
```go
type StreamConfig struct {
    Address             *Address           `json:"address"`
    Port                uint16             `json:"port"`
    Method              *TransportProtocol `json:"method"`      // Alias for network
    Network             *TransportProtocol `json:"network"`     // "raw", "xhttp", "mkcp", "grpc", "websocket", "httpupgrade", "hysteria", "masque", "xdrive"
    Security            string             `json:"security"`    // "none", "tls", "reality"
    FinalMask           *FinalMask         `json:"finalmask"`
    TLSSettings         *TLSConfig         `json:"tlsSettings"`
    REALITYSettings     *REALITYConfig     `json:"realitySettings"`
    RAWSettings         *TCPConfig         `json:"rawSettings"` // Alias for tcpSettings
    TCPSettings         *TCPConfig         `json:"tcpSettings"`
    XHTTPSettings       *SplitHTTPConfig   `json:"xhttpSettings"` // Alias for splithttpSettings
    SplitHTTPSettings   *SplitHTTPConfig   `json:"splithttpSettings"`
    KCPSettings         *KCPConfig         `json:"kcpSettings"`
    GRPCSettings        *GRPCConfig        `json:"grpcSettings"`
    WSSettings          *WebSocketConfig   `json:"wsSettings"`
    HTTPUPGRADESettings *HttpUpgradeConfig `json:"httpupgradeSettings"`
    HysteriaSettings    *HysteriaConfig    `json:"hysteriaSettings"`
    MASQUESettings      *MasqueConfig      `json:"masqueSettings"`
    XDRIVESettings      *XDriveConfig      `json:"xdriveSettings"`
    SocketSettings      *SocketConfig      `json:"sockopt"`
}
```

#### 2.2 SockoptObject (`sockopt`) Specification
From `infra/conf/transport_sockopt.go`:
```go
type SocketConfig struct {
    Mark                  int32                  `json:"mark"`
    TFO                   interface{}            `json:"tcpFastOpen"`
    TProxy                string                 `json:"tproxy"`
    AcceptProxyProtocol   bool                   `json:"acceptProxyProtocol"`
    DomainStrategy        string                 `json:"domainStrategy"`
    DialerProxy           string                 `json:"dialerProxy"`
    TCPKeepAliveInterval  int32                  `json:"tcpKeepAliveInterval"`
    TCPKeepAliveIdle      int32                  `json:"tcpKeepAliveIdle"`
    TCPCongestion         string                 `json:"tcpCongestion"`
    TCPWindowClamp        int32                  `json:"tcpWindowClamp"`
    TCPMaxSeg             int32                  `json:"tcpMaxSeg"`
    Penetrate             bool                   `json:"penetrate"`
    TCPUserTimeout        int32                  `json:"tcpUserTimeout"`
    V6only                bool                   `json:"v6only"`
    Interface             string                 `json:"interface"`
    TcpMptcp              bool                   `json:"tcpMptcp"`
    CustomSockopt         []*CustomSockoptConfig `json:"customSockopt"`
    AddressPortStrategy   string                 `json:"addressPortStrategy"`
    HappyEyeballsSettings *HappyEyeballsConfig   `json:"happyEyeballs"`
    TrustedXForwardedFor  []string               `json:"trustedXForwardedFor"`
}
```

Key fields for relay/chaining and performance:
1. `dialerProxy` (string): The `tag` of another outbound. Directs the transport dialer through another outbound, enabling proxy chaining (e.g. `Client -> Outbound A (dialerProxy: "proxy-b") -> Outbound B -> Destination`). Incompatible with `happyEyeballs`.
2. `tcpFastOpen` (`bool` | `int32`):
   - Outbound: `true` or positive integer enables TFO. `false` or negative integer disables it.
   - Inbound: Positive integer sets listener TFO backlog (`true` defaults to `256`).
3. `tcpKeepAliveInterval` & `tcpKeepAliveIdle` (int32 in seconds):
   - Outbound: Defaults to 45s (Chrome-like behavior). Negative values disable keepalive, positive values override defaults.
   - Inbound: Disabled by default; non-zero value enables keepalive.
4. `tcpUserTimeout` (int32 in milliseconds): Controls TCP user timeout (RFC 5482) before terminating unresponsive connections.
5. `tcpMptcp` (bool): Enables Multipath TCP (requires Linux Kernel 5.6+). Golang 1.24+ already enables listener MPTCP by default; this flag controls client dialer sockets.
6. `tcpNoDelay` (bool): Supported at socket layer, disabling Nagle's algorithm for interactive low latency.
7. `mark` (int32): Linux `SO_MARK` value for policy routing and iptables/nftables integration.
8. `interface` (string): Binds socket to a physical/virtual network interface (Linux, macOS, Windows), e.g. `"eth0"` or `"wg0"`.
9. `domainStrategy` (string): Controls DNS resolution of the proxy server endpoint itself:
   - `"AsIs"` (default, relies on OS resolver).
   - `"UseIP"`, `"UseIPv4"`, `"UseIPv6"`, `"UseIPv4v6"`, `"UseIPv6v4"`, `"ForceIP"`, `"ForceIPv4"`, `"ForceIPv6"`, `"ForceIPv4v6"`, `"ForceIPv6v4"`.
10. `happyEyeballs` (object): Dual-stack race according to RFC 8305:
    - `tryDelayMs` (uint64, default 0/disabled, recommended 250).
    - `prioritizeIPv6` (bool, default false).
    - `interleave` (uint32, default 1).
    - `maxConcurrentTry` (uint32, default 4).

#### 2.3 REALITY (`realitySettings`) Specification
From `infra/conf/transport_security.go`:
```go
type REALITYConfig struct {
    MasterKeyLog          string          `json:"masterKeyLog"`
    Show                  bool            `json:"show"`
    Target                json.RawMessage `json:"target"` // Alias for dest
    Dest                  json.RawMessage `json:"dest"`
    Type                  string          `json:"type"`
    Xver                  uint64          `json:"xver"`
    ServerNames           []string        `json:"serverNames"`
    PrivateKey            string          `json:"privateKey"`
    MinClientVer          string          `json:"minClientVer"`
    MaxClientVer          string          `json:"maxClientVer"`
    MaxTimeDiff           uint64          `json:"maxTimeDiff"`
    ShortIds              []string        `json:"shortIds"`
    Mldsa65Seed           string          `json:"mldsa65Seed"`
    LimitFallbackUpload   LimitFallback   `json:"limitFallbackUpload"`
    LimitFallbackDownload LimitFallback   `json:"limitFallbackDownload"`

    Fingerprint           string          `json:"fingerprint"`
    ServerName            string          `json:"serverName"`
    Password              string          `json:"password"` // New alias for publicKey
    PublicKey             string          `json:"publicKey"`
    ShortId               string          `json:"shortId"`
    Mldsa65Verify         string          `json:"mldsa65Verify"`
    SpiderX               string          `json:"spiderX"`
}
```

- **Inbound (Server) Specific Fields**:
  - `dest` / `target` (string): Target handshake fallback server (e.g. `"example.com:443"`, `"1.1.1.1:443"`, or unix socket).
  - `xver` (uint64): PROXY protocol version sent to fallback destination (`0`, `1`, or `2`).
  - `serverNames` ([]string): Allowed SNIs matching destination certificate. Can include `""` to accept requests without SNI.
  - `privateKey` (string): Base64 RawURL encoded 32-byte X25519 private key.
  - `shortIds` ([]string): List of hex short IDs up to 16 characters (even length). May include `""`.
  - `mldsa65Seed` (string): Base64 RawURL encoded seed for ML-DSA-65 post-quantum signature.
  - `minClientVer` / `maxClientVer` (string, e.g. `"24.9.30"`): Version gating.
  - `maxTimeDiff` (uint64): Max allowed timestamp drift in milliseconds.
  - `limitFallbackUpload` / `limitFallbackDownload` (object): Token bucket rate limiter for unauthorized fallback connections (`afterBytes`, `bytesPerSec`, `burstBytesPerSec`).
- **Outbound (Client) Specific Fields**:
  - `fingerprint` (string): uTLS client hello fingerprint (`"chrome"`, `"firefox"`, `"safari"`, `"ios"`, `"android"`, `"edge"`, `"360"`, `"qq"`, etc.). Note: `"unsafe"` is rejected.
  - `serverName` (string): SNI to disguise as.
  - `publicKey` / `password` (string): Base64 RawURL encoded 32-byte public key. `password` is the modern alias.
  - `shortId` (string): Hex string matching one of the server's `shortIds`.
  - `spiderX` (string): Initial crawl path and query parameters (e.g. `"/"`).
  - `mldsa65Verify` (string): Base64 RawURL encoded ML-DSA-65 public key (1952 bytes) for post-quantum cert verification.
  - `show` (bool): Debug log output.

---

### 3. InboundObject (`inbounds`) & Sniffing Specification

#### 3.1 InboundObject Top-Level Fields
From `infra/conf/xray.go`:
```go
type InboundDetourConfig struct {
    Protocol       string           `json:"protocol"`
    PortList       *PortList        `json:"port"`
    ListenOn       *Address         `json:"listen"`
    Settings       *json.RawMessage `json:"settings"`
    Tag            string           `json:"tag"`
    StreamSetting  *StreamConfig    `json:"streamSettings"`
    SniffingConfig *SniffingConfig  `json:"sniffing"`
}
```
- `listen` (string): Bind address (`"0.0.0.0"`, `"::"`, specific IP, or Unix domain socket path e.g. `"/dev/shm/xray.sock,0666"`).
- `port` (number | string): Listening port, port range (`"1000-2000"`), comma-separated list (`"80,443"`), or environment variable (`"env:PORT"`).
- `protocol` (string): Inbound protocol (`"vless"`, `"vmess"`, `"trojan"`, `"shadowsocks"`, `"socks"`, `"http"`, `"tunnel"`, `"tun"`, `"wireguard"`, `"hysteria"`, `"masque"`).
- `settings` (object): Protocol-specific server settings.
- `streamSettings` (`StreamConfig`): Transport and TLS/REALITY settings.
- `tag` (string): Inbound identifier used by routing rules.
- `sniffing` (`SniffingConfig`): Content inspection and destination override.

#### 3.2 SniffingObject (`sniffing`) Specification
From `infra/conf/xray.go`:
```go
type SniffingConfig struct {
    Enabled         bool       `json:"enabled"`
    DestOverride    StringList `json:"destOverride"`
    DomainsExcluded StringList `json:"domainsExcluded"`
    IPsExcluded     StringList `json:"ipsExcluded"`
    MetadataOnly    bool       `json:"metadataOnly"`
    RouteOnly       bool       `json:"routeOnly"`
}
```
- `enabled` (bool): Enables traffic sniffing.
- `destOverride` ([]string): Protocols whose sniffed destination domains will override the connection target:
  - `"http"`, `"tls"` (or `"https"`, `"ssl"`), `"quic"`, `"fakedns"` (or `"fakedns+others"`).
- `metadataOnly` (bool): When `true`, sniffs target domain using only connection metadata (without buffering data payload). Compatible with protocols where the server talks first (e.g. SMTP), but only `fakedns` sniffing works without initial client payload.
- `domainsExcluded` ([]string): List of domain rules (substring, regexp, geosite) to skip overriding.
- `ipsExcluded` ([]string): List of IP rules (CIDR, geoip) to skip overriding.
- `routeOnly` (bool): When `true`, sniffed domain is used strictly for routing decision matching; the outbound connection target remains the original IP destination.

---

## Contradictions
1. **`outbound.proxySettings` vs `streamSettings.sockopt.dialerProxy`**: Older V2Ray configurations used `proxySettings.tag` for chaining outbounds. In modern Xray-core, `proxySettings` is completely deprecated and removed; parsing it causes a hard error. All chaining must use `streamSettings.sockopt.dialerProxy`.
2. **`realitySettings.publicKey` vs `realitySettings.password`**: Xray-core added `password` as an alias for `publicKey` on clients to avoid user misconception regarding public key confidentiality. Both are parsed interchangeably in `transport_security.go`.
3. **`realitySettings.dest` vs `realitySettings.target`**: `target` was added as an alias for `dest` on the server inbound configuration; both are supported.

## Missing Evidence
None. All Go struct fields and JSON tags were verified against the official `XTLS/Xray-core` source repository (`infra/conf/`) and official documentation at `xtls.github.io/config/`.

---

## Sources
- **Kept**:
  1. `XTLS/Xray-core: infra/conf/xray.go` (https://github.com/XTLS/Xray-core/blob/main/infra/conf/xray.go) — Source of truth for `OutboundDetourConfig`, `InboundDetourConfig`, `MuxConfig`, `SniffingConfig`.
  2. `XTLS/Xray-core: infra/conf/transport_internet.go` (https://github.com/XTLS/Xray-core/blob/main/infra/conf/transport_internet.go) — Source of truth for `StreamConfig`.
  3. `XTLS/Xray-core: infra/conf/transport_sockopt.go` (https://github.com/XTLS/Xray-core/blob/main/infra/conf/transport_sockopt.go) — Source of truth for `SocketConfig` and `HappyEyeballsConfig`.
  4. `XTLS/Xray-core: infra/conf/transport_security.go` (https://github.com/XTLS/Xray-core/blob/main/infra/conf/transport_security.go) — Source of truth for `REALITYConfig` and `TLSConfig`.
  5. `Project X Official Docs: Outbound` (https://xtls.github.io/config/outbound.html) — Official documentation for Mux, XUDP, and Outbound.
  6. `Project X Official Docs: Sockopt` (https://xtls.github.io/config/transports/sockopt.html) — Official documentation for sockopt and dialerProxy.
  7. `Project X Official Docs: REALITY` (https://xtls.github.io/config/transports/reality.html) — Official documentation for REALITY inbound/outbound parameters.
  8. `Project X Official Docs: Inbound` (https://xtls.github.io/config/inbound.html) — Official documentation for Inbound and Sniffing.
- **Rejected/deprioritized**:
  - Legacy V2Ray documentation (v2fly.org) — Deprecated schema containing outdated `proxySettings` and absent REALITY/XUDP definitions.

## Next steps
- Align the internal Go models in `internal/domain` and `internal/adapter/xray` with these exact fields:
  - Add `Mux` (`enabled`, `concurrency`, `xudpConcurrency`, `xudpProxyUDP443`) to outbound generator.
  - Implement validation rejecting `Mux` when `flow: "xtls-rprx-vision"` is active.
  - Ensure outbound chaining uses `streamSettings.sockopt.dialerProxy`.
