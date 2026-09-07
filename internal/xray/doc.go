// Package xray provides a production-grade gRPC client and runtime coordinator
// for Xray-core / v2ray-core, separating dynamic runtime state changes from persistent disk configuration.
//
// # Architecture Overview
//
// Traditional control panels modify config.json directly and execute 'xray run' or 'systemctl restart xray'
// on every user addition/removal. This forcibly terminates active long-lived TCP/TLS connections (e.g. WebSocket,
// gRPC, REALITY tunnels) and introduces significant process restart overhead.
//
// This package decouples runtime hot-reloading from disk persistence:
//  1. Runtime Hot Changes: User additions and deletions are dispatched immediately to the running Xray process
//     via Xray's official gRPC HandlerService (dokodemo-door inbound, e.g. 127.0.0.1:10085).
//  2. Local State Coordination: Successfully applied changes are recorded into an embedded ACID BoltDB (bbolt) store.
//     If gRPC fails, the DB transaction is not committed. If DB persistence fails after gRPC success, a compensation
//     rollback is attempted to prevent phantom/dirty state.
//  3. Cold-Boot Disk Persistence: A fallback SyncToDiskConfig mechanism writes all current users from BoltDB
//     back into config.json only upon system shutdown or manual trigger, ensuring zero data loss on reboot.
//  4. Traffic Collection: Real-time traffic statistics (uplink and downlink) are collected on demand via StatsService.
//
// # Official Protobuf Dependencies (github.com/xtls/xray-core)
//
// Add the following module dependency to your go.mod:
//
//	require github.com/xtls/xray-core v1.260327.0 // or newer
//
// The following protobuf packages from github.com/xtls/xray-core are required and utilized:
//
//  1. "github.com/xtls/xray-core/app/proxyman/command":
//     - HandlerServiceClient: gRPC client interface generated from command.proto.
//     - AlterInboundRequest: Request message containing Inbound Tag and a serialized Operation.
//     - AddUserOperation: Operation message wrapping a protocol.User.
//     - RemoveUserOperation: Operation message specifying the Email identifier to remove.
//
//  2. "github.com/xtls/xray-core/app/stats/command":
//     - StatsServiceClient: gRPC client interface for Xray's internal stats engine.
//     - QueryStatsRequest: Request message with regex/prefix Pattern and Reset_ flag.
//     - QueryStatsResponse & StatEntry: Response containing counter Name and Value (int64 bytes).
//     - SysStatsRequest & SysStatsResponse: System diagnostics used for health checking.
//
//  3. "github.com/xtls/xray-core/common/protocol":
//     - User: Represents a user identity (Email, Level, and serialized Account).
//     - SecurityConfig & SecurityType: Used for protocol encryption settings.
//
//  4. "github.com/xtls/xray-core/common/serial":
//     - ToTypedMessage: Converts proto.Message structs into typed protobuf wrappers
//     (type.googleapis.com/...) required by Xray's internal polymorphic dispatch system.
//
//  5. Protocol Account Definitions:
//     - "github.com/xtls/xray-core/proxy/vless": Account (Id, Flow e.g. "xtls-rprx-vision").
//     - "github.com/xtls/xray-core/proxy/vmess": Account (Id, SecuritySettings).
//     - "github.com/xtls/xray-core/proxy/trojan": Account (Password).
//     - "github.com/xtls/xray-core/proxy/shadowsocks": Account (Password, CipherType).
//
// # Example Usage
//
//	client, err := xray.NewXrayClient("127.0.0.1:10085",
//	    xray.WithDBPath("/var/lib/panel/xray.db"),
//	    xray.WithDialTimeout(3*time.Second),
//	)
//	if err != nil {
//	    log.Fatalf("failed to create client: %v", err)
//	}
//	defer client.Close()
//
//	// Add VLESS Reality user
//	err = client.AddInboundUser("vless-in", "alice@example.com", "uuid-here", "xtls-rprx-vision")
//
//	// Query traffic
//	stats, err := client.QueryTraffic("user>>>", false)
//
//	// Cold-boot sync to disk on graceful shutdown
//	err = client.SyncToDiskConfig("/etc/xray/config.template.json", "/etc/xray/config.json")
package xray
