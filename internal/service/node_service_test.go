package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"panel/internal/adapter/repository"
	"panel/internal/adapter/xray"
	"panel/internal/domain"
	"panel/internal/service"
)

func setupNodeServiceTestEnv(t *testing.T) (
	*service.NodeService,
	*service.ConfigService,
	*service.UserService,
	*repository.GORMNodeRepository,
	*repository.GORMInboundRepository,
	*repository.GORMUserRepository,
) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "node_service_test.db")
	db, err := repository.InitSQLite(dbPath)
	if err != nil {
		t.Fatalf("InitSQLite failed: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	nodeRepo := repository.NewNodeRepository(db)
	userRepo := repository.NewUserRepository(db)
	inboundRepo := repository.NewInboundRepository(db)
	trafficLogRepo := repository.NewGormTrafficLogRepository(db)
	snapshotRepo := repository.NewGormConfigSnapshotRepository(db)

	compiler := xray.NewXrayCompiler(8080)
	nodeSvc := service.NewNodeService(nodeRepo, nodeRepo, inboundRepo, userRepo, compiler)

	configPath := filepath.Join(t.TempDir(), "xray_config.json")
	configMgr := xray.NewConfigManager(configPath, "")
	configSvc := service.NewConfigService(configMgr, nil, inboundRepo, userRepo, snapshotRepo, compiler).
		WithNodeCacheInvalidator(nodeSvc)
	userSvc := service.NewUserService(userRepo, inboundRepo, trafficLogRepo, nil, configSvc).
		WithNodeCacheInvalidator(nodeSvc)

	return nodeSvc, configSvc, userSvc, nodeRepo, inboundRepo, userRepo
}

func TestNodeService_CRUD_TokenSecurity_Rotate_DeleteConflict(t *testing.T) {
	nodeSvc, _, _, nodeRepo, inboundRepo, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:   "SG Node",
		Host:   "127.0.0.1",
		Region: "SG",
		Remark: "edge",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if len(created.Token) != 64 {
		t.Fatalf("expected 64-hex token (32 bytes), got len=%d", len(created.Token))
	}
	if created.TokenPrefix != created.Token[:8] {
		t.Fatalf("expected tokenPrefix %q, got %q", created.Token[:8], created.TokenPrefix)
	}

	// Verify DB stores SHA-256 hash, not plaintext
	stored, err := nodeRepo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	sum := sha256.Sum256([]byte(created.Token))
	expectedHash := hex.EncodeToString(sum[:])
	if stored.TokenHash != expectedHash {
		t.Fatalf("expected SHA-256 tokenHash %q, got %q", expectedHash, stored.TokenHash)
	}

	// Verify JSON serialization of Node never exposes tokenHash
	nodeJSON, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal node failed: %v", err)
	}
	if strings.Contains(string(nodeJSON), expectedHash) || strings.Contains(string(nodeJSON), "tokenHash") {
		t.Fatalf("node JSON leaked tokenHash: %s", string(nodeJSON))
	}

	// Authenticate valid / invalid / disabled
	authed, err := nodeSvc.AuthenticateAgent(ctx, created.Token)
	if err != nil || authed.ID != created.ID {
		t.Fatalf("expected AuthenticateAgent success, got %+v err=%v", authed, err)
	}
	if _, err := nodeSvc.AuthenticateAgent(ctx, "wrong-token"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for bad token, got %v", err)
	}

	disabled := false
	if _, err := nodeSvc.UpdateNode(ctx, created.ID, domain.UpdateNodeDTO{Enabled: &disabled}); err != nil {
		t.Fatalf("UpdateNode disable failed: %v", err)
	}
	if _, err := nodeSvc.AuthenticateAgent(ctx, created.Token); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for disabled node, got %v", err)
	}

	enabled := true
	if _, err := nodeSvc.UpdateNode(ctx, created.ID, domain.UpdateNodeDTO{Enabled: &enabled}); err != nil {
		t.Fatalf("UpdateNode enable failed: %v", err)
	}

	// RotateToken revokes old token and issues new 64-hex token
	rotated, err := nodeSvc.RotateToken(ctx, created.ID)
	if err != nil {
		t.Fatalf("RotateToken failed: %v", err)
	}
	if rotated.Token == created.Token || len(rotated.Token) != 64 {
		t.Fatalf("expected new distinct 64-hex token, got %q", rotated.Token)
	}
	if _, err := nodeSvc.AuthenticateAgent(ctx, created.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected old token to be revoked (ErrUnauthorized), got %v", err)
	}
	if _, err := nodeSvc.AuthenticateAgent(ctx, rotated.Token); err != nil {
		t.Fatalf("expected new token to authenticate, got %v", err)
	}

	// Delete conflict when inbound is bound
	ib := &domain.Inbound{
		NodeID:   created.ID,
		Tag:      "sg-vless",
		Port:     443,
		Protocol: "vless",
		Enabled:  true,
	}
	if err := inboundRepo.Create(ctx, ib); err != nil {
		t.Fatalf("create bound inbound failed: %v", err)
	}
	if err := nodeSvc.DeleteNode(ctx, created.ID); !errors.Is(err, domain.ErrNodeHasInbounds) {
		t.Fatalf("expected ErrNodeHasInbounds on delete with bound inbound, got %v", err)
	}

	// Install script contains master URL and placeholder token (no real token leak)
	script, err := nodeSvc.RenderInstallScript(ctx, created.ID, "https://panel.example.test")
	if err != nil {
		t.Fatalf("RenderInstallScript failed: %v", err)
	}
	if !strings.Contains(script, "https://panel.example.test") || strings.Contains(script, rotated.Token) {
		t.Fatalf("unexpected install script content: %s", script)
	}

	if err := inboundRepo.Delete(ctx, ib.ID); err != nil {
		t.Fatalf("delete inbound failed: %v", err)
	}
	if err := nodeSvc.DeleteNode(ctx, created.ID); err != nil {
		t.Fatalf("DeleteNode after unbinding inbound failed: %v", err)
	}
}

func TestNodeService_HandleAgentSync_ShortCircuit_CacheInvalidation_Idempotency(t *testing.T) {
	nodeSvc, configSvc, userSvc, nodeRepo, inboundRepo, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	createdNode, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "JP Direct",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Local inbound (NodeID = 0) and remote inbound (NodeID = createdNode.ID)
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:   0,
		Tag:      "local-vless",
		Port:     8443,
		Protocol: "vless",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("create local inbound failed: %v", err)
	}
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:   createdNode.ID,
		Tag:      "jp-vless",
		Port:     443,
		Protocol: "vless",
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"apple.com:443","serverNames":["apple.com"],"privateKey":"OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc"}}`,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("create remote inbound failed: %v", err)
	}

	// Verify local config.json excludes remote inbound jp-vless
	localRaw, err := configSvc.GetRawConfig(ctx)
	if err != nil {
		t.Fatalf("GetRawConfig failed: %v", err)
	}
	if strings.Contains(string(localRaw), "jp-vless") {
		t.Fatalf("local config.json must NOT contain remote inbound jp-vless: %s", string(localRaw))
	}
	if !strings.Contains(string(localRaw), "local-vless") {
		t.Fatalf("local config.json must contain local-vless: %s", string(localRaw))
	}

	// Create user with finite quota authorized on jp-vless
	user, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "alice@example.com",
		InboundTags: []string{"jp-vless"},
		TotalBytes:  1000,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// 1. First sync: full config + users returned
	resp1, err := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
		AgentVersion: "0.1.0",
		XrayVersion:  "25.1.1",
	})
	if err != nil {
		t.Fatalf("first HandleAgentSync failed: %v", err)
	}
	if resp1.ConfigUnchanged || resp1.UsersUnchanged {
		t.Fatalf("expected initial sync unchanged=false/false, got %+v", resp1)
	}
	if !strings.Contains(resp1.ConfigJSON, "jp-vless") || strings.Contains(resp1.ConfigJSON, "local-vless") {
		t.Fatalf("remote configJson must contain jp-vless and exclude local-vless: %s", resp1.ConfigJSON)
	}
	if !strings.Contains(resp1.ConfigJSON, `"freedom"`) || !strings.Contains(resp1.ConfigJSON, `"127.0.0.1"`) {
		t.Fatalf("remote configJson missing direct outbound or api inbound: %s", resp1.ConfigJSON)
	}
	if len(resp1.Users) != 1 || resp1.Users[0].Email != user.Email || resp1.Users[0].Flow != "xtls-rprx-vision" {
		t.Fatalf("unexpected sync users: %+v", resp1.Users)
	}

	// 2. Second sync with matching hashes: short-circuit
	resp2, err := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
		ConfigHash: resp1.ConfigHash,
		UsersHash:  resp1.UsersHash,
	})
	if err != nil {
		t.Fatalf("second HandleAgentSync failed: %v", err)
	}
	if !resp2.ConfigUnchanged || !resp2.UsersUnchanged {
		t.Fatalf("expected short-circuit unchanged=true/true, got %+v", resp2)
	}
	if resp2.ConfigJSON != "" || len(resp2.Users) != 0 {
		t.Fatalf("expected empty configJson and users on short-circuit, got configLen=%d users=%d", len(resp2.ConfigJSON), len(resp2.Users))
	}

	// 3. Create another user -> ConfigHash stays unchanged (no Xray restart), UsersHash updates!
	if _, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "bob@example.com",
		InboundTags: []string{"jp-vless"},
	}); err != nil {
		t.Fatalf("CreateUser bob failed: %v", err)
	}
	resp3, err := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
		ConfigHash: resp1.ConfigHash,
		UsersHash:  resp1.UsersHash,
	})
	if err != nil {
		t.Fatalf("third HandleAgentSync failed: %v", err)
	}
	if !resp3.ConfigUnchanged || resp3.UsersUnchanged {
		t.Fatalf("expected configUnchanged=true and usersUnchanged=false after user change, got config=%v users=%v", resp3.ConfigUnchanged, resp3.UsersUnchanged)
	}
	if len(resp3.Users) != 2 {
		t.Fatalf("expected 2 users, got %+v", resp3.Users)
	}

	// 4. Duplicate traffic batch is idempotent; exceeding quota drops alice from users immediately
	batch := &domain.AgentTrafficBatch{
		BatchID: "b-quota-1",
		Users: []domain.AgentUserTrafficDelta{
			{Email: "alice@example.com", UpBytes: 600, DownBytes: 500},
		},
		Inbounds: []domain.AgentInboundTrafficDelta{
			{Tag: "jp-vless", UpBytes: 600, DownBytes: 500},
		},
	}
	resp4, err := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
		ConfigHash:   resp3.ConfigHash,
		UsersHash:    resp3.UsersHash,
		TrafficBatch: batch,
	})
	if err != nil {
		t.Fatalf("fourth HandleAgentSync failed: %v", err)
	}
	if resp4.UsersUnchanged || len(resp4.Users) != 1 || resp4.Users[0].Email != "bob@example.com" {
		t.Fatalf("expected alice evicted after exceeding quota, got users=%+v", resp4.Users)
	}

	// Repeat exact same batchId -> must not double count
	if _, err := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
		ConfigHash:   resp4.ConfigHash,
		UsersHash:    resp4.UsersHash,
		TrafficBatch: batch,
	}); err != nil {
		t.Fatalf("fifth HandleAgentSync failed: %v", err)
	}
	nAfter, _ := nodeRepo.GetByID(ctx, createdNode.ID)
	if nAfter.XrayUpBytes != 600 || nAfter.XrayDownBytes != 500 {
		t.Fatalf("expected node xray traffic 600/500, got %d/%d", nAfter.XrayUpBytes, nAfter.XrayDownBytes)
	}
	ibAfter, _ := inboundRepo.GetByTag(ctx, "jp-vless")
	if ibAfter.UpBytes != 600 || ibAfter.DownBytes != 500 {
		t.Fatalf("expected jp-vless traffic 600/500, got %d/%d", ibAfter.UpBytes, ibAfter.DownBytes)
	}

	// 5. Concurrent sync + invalidation race safety
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%3 == 0 {
				nodeSvc.InvalidateNode(createdNode.ID)
			}
			_, syncErr := nodeSvc.HandleAgentSync(ctx, createdNode.ID, &domain.AgentSyncRequest{
				ConfigHash: resp4.ConfigHash,
				UsersHash:  resp4.UsersHash,
			})
			if syncErr != nil {
				t.Errorf("concurrent HandleAgentSync failed: %v", syncErr)
			}
		}(i)
	}
	wg.Wait()
}

func TestNode_CalcCycleUsedBytes_DualAccounting(t *testing.T) {
	// NIC preferred over Xray
	n1 := &domain.Node{
		TrafficCountMode:   domain.TrafficCountModeBoth,
		TrafficMultiplier:  1.05,
		CycleNicUpBytes:    1000,
		CycleNicDownBytes:  2000,
		CycleXrayUpBytes:   500,
		CycleXrayDownBytes: 500,
	}
	if got := n1.CalcCycleUsedBytes(); got != 3150 {
		t.Fatalf("expected NIC both*1.05 = 3150, got %d", got)
	}

	// Fallback to Xray when NIC is zero
	n2 := &domain.Node{
		TrafficCountMode:   domain.TrafficCountModeBoth,
		TrafficMultiplier:  1.0,
		CycleXrayUpBytes:   400,
		CycleXrayDownBytes: 600,
	}
	if got := n2.CalcCycleUsedBytes(); got != 1000 {
		t.Fatalf("expected Xray fallback both = 1000, got %d", got)
	}

	// Outbound mode uses up only
	n3 := &domain.Node{
		TrafficCountMode:  domain.TrafficCountModeOutbound,
		TrafficMultiplier: 1.0,
		CycleNicUpBytes:   700,
		CycleNicDownBytes: 3000,
	}
	if got := n3.CalcCycleUsedBytes(); got != 700 {
		t.Fatalf("expected outbound = 700, got %d", got)
	}

	// Max mode uses max(up, down)
	n4 := &domain.Node{
		TrafficCountMode:  domain.TrafficCountModeMax,
		TrafficMultiplier: 1.0,
		CycleNicUpBytes:   700,
		CycleNicDownBytes: 3000,
	}
	if got := n4.CalcCycleUsedBytes(); got != 3000 {
		t.Fatalf("expected max = 3000, got %d", got)
	}
}

func TestNodeService_QuotaValidation(t *testing.T) {
	nodeSvc, _, _, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	badWarn := 95
	badDrain := 90
	if _, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:              "Bad Threshold",
		Host:              "127.0.0.1",
		WarnThresholdPct:  &badWarn,
		DrainThresholdPct: &badDrain,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for warn > drain, got %v", err)
	}

	zeroMult := 0.0
	if _, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:              "Bad Mult",
		Host:              "127.0.0.1",
		TrafficMultiplier: &zeroMult,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for multiplier <= 0, got %v", err)
	}

	badReset := 32
	if _, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:            "Bad ResetDay",
		Host:            "127.0.0.1",
		BillingResetDay: &badReset,
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for billingResetDay > 31, got %v", err)
	}

	if _, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:             "Bad Mode",
		Host:             "127.0.0.1",
		TrafficCountMode: "inbound_only",
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unsupported trafficCountMode, got %v", err)
	}
}

func TestNodeService_HandleAgentSync_AutoDrainBreakAndReset(t *testing.T) {
	nodeSvc, _, _, nodeRepo, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	var transitions []string
	nodeSvc.WithStateChangeNotifier(func(ctx context.Context, node *domain.Node, oldState, newState, reason string) {
		transitions = append(transitions, oldState+"->"+newState+":"+reason)
	})

	limit := int64(1000)
	resetDay := 31
	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name:              "Quota Edge",
		Host:              "127.0.0.1",
		TrafficLimitBytes: &limit,
		BillingResetDay:   &resetDay,
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// 1. 85% -> remains active, QuotaPolicy populated
	resp1, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 400, NicDownDelta: 450},
	})
	if err != nil {
		t.Fatalf("sync 85%% failed: %v", err)
	}
	if resp1.DesiredState != domain.NodeStateActive {
		t.Fatalf("expected active at 85%%, got %q", resp1.DesiredState)
	}
	if resp1.QuotaPolicy == nil || resp1.QuotaPolicy.CycleUsedBytes != 850 || resp1.QuotaPolicy.TrafficLimitBytes != 1000 {
		t.Fatalf("unexpected QuotaPolicy at 85%%: %+v", resp1.QuotaPolicy)
	}

	// 2. +50 -> 90% => auto draining
	resp2, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 50},
	})
	if err != nil {
		t.Fatalf("sync 90%% failed: %v", err)
	}
	if resp2.DesiredState != domain.NodeStateDraining {
		t.Fatalf("expected draining at 90%%, got %q", resp2.DesiredState)
	}

	// 3. +50 -> 95% => auto circuit_broken
	resp3, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicDownDelta: 50},
	})
	if err != nil {
		t.Fatalf("sync 95%% failed: %v", err)
	}
	if resp3.DesiredState != domain.NodeStateCircuitBroken {
		t.Fatalf("expected circuit_broken at 95%%, got %q", resp3.DesiredState)
	}

	if len(transitions) != 2 ||
		transitions[0] != "active->draining:auto_drain_quota" ||
		transitions[1] != "draining->circuit_broken:auto_break_quota" {
		t.Fatalf("unexpected transitions: %v", transitions)
	}

	// 4. Short-month billing reset (BillingResetDay=31 in Feb 2024 leap year: Feb 28 does NOT reset, Feb 29 DOES reset)
	feb28 := time.Date(2024, 2, 28, 12, 0, 0, 0, time.UTC)
	if err := nodeSvc.CheckAndResetMonthlyTrafficAt(ctx, feb28); err != nil {
		t.Fatalf("CheckAndResetMonthlyTrafficAt feb28 failed: %v", err)
	}
	stillBroken, _ := nodeRepo.GetByID(ctx, created.ID)
	if stillBroken.DesiredState != domain.NodeStateCircuitBroken || stillBroken.CycleUsedBytes != 0 && stillBroken.CalcCycleUsedBytes() != 950 {
		t.Fatalf("expected no reset on leap year feb 28, got state=%q used=%d", stillBroken.DesiredState, stillBroken.CalcCycleUsedBytes())
	}

	feb29 := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
	if err := nodeSvc.CheckAndResetMonthlyTrafficAt(ctx, feb29); err != nil {
		t.Fatalf("CheckAndResetMonthlyTrafficAt feb29 failed: %v", err)
	}
	recovered, _ := nodeRepo.GetByID(ctx, created.ID)
	if recovered.DesiredState != domain.NodeStateActive || recovered.CalcCycleUsedBytes() != 0 || recovered.LastResetMonth != 202402 {
		t.Fatalf("expected reset & recovery on feb 29, got state=%q used=%d month=%d", recovered.DesiredState, recovered.CalcCycleUsedBytes(), recovered.LastResetMonth)
	}
	if len(transitions) != 3 || transitions[2] != "circuit_broken->active:monthly_reset_recovered" {
		t.Fatalf("expected recovery notification, got %v", transitions)
	}

	// 5. Repeat on same month must not double-reset; manual drain must not be overridden by auto-break
	if _, err := nodeSvc.SetNodeDesiredState(ctx, created.ID, domain.NodeStateDraining); err != nil {
		t.Fatalf("SetNodeDesiredState manual failed: %v", err)
	}
	respManual, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		HostMetrics: &domain.AgentHostMetrics{NicUpDelta: 990},
	})
	if err != nil {
		t.Fatalf("sync under manual state failed: %v", err)
	}
	if respManual.DesiredState != domain.NodeStateDraining {
		t.Fatalf("manual draining state must not be overwritten by auto breaker, got %q", respManual.DesiredState)
	}
}

func TestNodeService_AutoStaticRelayInjection_PreservesFlowAndConfigHash(t *testing.T) {
	nodeSvc, configSvc, userSvc, _, _, _ := setupNodeServiceTestEnv(t)
	configSvc.WithNodeRepo(nil) // not required for SaveOutbound(0)
	nodeSvc.WithLocalOutboundsProvider(configSvc.ListOutbounds)
	ctx := context.Background()

	// Initialize local config.json
	if err := configSvc.SaveConfigQuietly(ctx, "init"); err != nil {
		t.Fatalf("SaveConfigQuietly failed: %v", err)
	}

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "US Relay Target",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Remote VLESS TCP+REALITY inbound (default flow=xtls-rprx-vision, port=443, externalPort=8443)
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:         created.ID,
		Tag:            "us-vless",
		Port:           443,
		ExternalPort:   8443,
		Protocol:       "vless",
		SettingsJSON:   `{"decryption":"none","flow":"xtls-rprx-vision"}`,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"apple.com:443","serverNames":["apple.com"],"privateKey":"SL552CAJYk9fBNX_MvMJCaFmDQ-LHhugYSHYZ4bZXU8","shortIds":["0123456789abcdef"]}}`,
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create remote vless inbound failed: %v", err)
	}
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:   created.ID,
		Tag:      "us-trojan",
		Port:     9443,
		Protocol: "trojan",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("create remote trojan inbound failed: %v", err)
	}

	if _, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "normal-user@example.com",
		InboundTags: []string{"us-vless"},
	}); err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Baseline sync without local relay outbounds
	baseResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("baseline HandleAgentSync failed: %v", err)
	}

	// Save Master local outbounds:
	// 1. VLESS matching ExternalPort 8443 with EMPTY flow (must remain empty!)
	// 2. VLESS matching Port 443 with explicit xtls-rprx-vision
	// 3. Trojan matching Port 9443
	// 4. Unrelated host (must NOT inject)
	if err := configSvc.SaveOutbound(ctx, domain.Outbound{
		Tag:          "relay-us-noflow",
		Protocol:     "vless",
		SettingsJSON: `{"vnext":[{"address":"127.0.0.1","port":8443,"users":[{"id":"22222222-2222-2222-2222-222222222222","encryption":"none"}]}]}`,
	}); err != nil {
		t.Fatalf("SaveOutbound relay-us-noflow failed: %v", err)
	}
	if err := configSvc.SaveOutbound(ctx, domain.Outbound{
		Tag:          "relay-us-vision",
		Protocol:     "vless",
		SettingsJSON: `{"vnext":[{"address":"127.0.0.1","port":443,"users":[{"id":"33333333-3333-3333-3333-333333333333","encryption":"none","flow":"xtls-rprx-vision"}]}]}`,
	}); err != nil {
		t.Fatalf("SaveOutbound relay-us-vision failed: %v", err)
	}
	if err := configSvc.SaveOutbound(ctx, domain.Outbound{
		Tag:          "relay-us-trojan",
		Protocol:     "trojan",
		SettingsJSON: `{"servers":[{"address":"127.0.0.1","port":9443,"password":"trojan-relay-pw"}]}`,
	}); err != nil {
		t.Fatalf("SaveOutbound relay-us-trojan failed: %v", err)
	}
	if err := configSvc.SaveOutbound(ctx, domain.Outbound{
		Tag:          "other-host",
		Protocol:     "vless",
		SettingsJSON: `{"vnext":[{"address":"10.9.9.9","port":443,"users":[{"id":"44444444-4444-4444-4444-444444444444","encryption":"none"}]}]}`,
	}); err != nil {
		t.Fatalf("SaveOutbound other-host failed: %v", err)
	}

	syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: baseResp.ConfigHash,
		UsersHash:  baseResp.UsersHash,
	})
	if err != nil {
		t.Fatalf("HandleAgentSync after local relay add failed: %v", err)
	}
	if syncResp.ConfigUnchanged {
		t.Fatalf("expected ConfigHash to change when matching local relay outbounds are added")
	}

	var cfgMap map[string]interface{}
	if err := json.Unmarshal([]byte(syncResp.ConfigJSON), &cfgMap); err != nil {
		t.Fatalf("unmarshal ConfigJSON failed: %v", err)
	}
	var vlessClients, trojanClients []interface{}
	for _, item := range cfgMap["inbounds"].([]interface{}) {
		inb := item.(map[string]interface{})
		switch inb["tag"] {
		case "us-vless":
			vlessClients = inb["settings"].(map[string]interface{})["clients"].([]interface{})
		case "us-trojan":
			trojanClients = inb["settings"].(map[string]interface{})["clients"].([]interface{})
		}
	}

	if len(vlessClients) != 3 {
		t.Fatalf("expected 3 vless clients (1 db + 2 static), got %d: %+v", len(vlessClients), vlessClients)
	}
	byEmail := make(map[string]map[string]interface{})
	for _, cl := range vlessClients {
		cm := cl.(map[string]interface{})
		byEmail[cm["email"].(string)] = cm
	}
	if _, hasUnrelated := byEmail["relay-auto:other-host@internal"]; hasUnrelated {
		t.Fatalf("unrelated host outbound must not be injected")
	}
	if noFlowCl := byEmail["relay-auto:relay-us-noflow@internal"]; noFlowCl == nil {
		t.Fatalf("missing relay-auto:relay-us-noflow@internal")
	} else if _, hasFlow := noFlowCl["flow"]; hasFlow {
		t.Fatalf("CRITICAL: empty outbound flow was erroneously filled with %v", noFlowCl["flow"])
	}
	if visionCl := byEmail["relay-auto:relay-us-vision@internal"]; visionCl == nil || visionCl["flow"] != "xtls-rprx-vision" {
		t.Fatalf("expected vision flow preserved, got %+v", visionCl)
	}
	if len(trojanClients) != 1 || trojanClients[0].(map[string]interface{})["email"] != "relay-auto:relay-us-trojan@internal" {
		t.Fatalf("unexpected trojan static clients: %+v", trojanClients)
	}

	// BuildRemoteNodeUsers (UsersHash & fresh sync Users) must ONLY contain normal DB users
	if !syncResp.UsersUnchanged {
		t.Fatalf("expected UsersUnchanged=true when only static relay credentials changed")
	}
	freshSync, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("fresh HandleAgentSync failed: %v", err)
	}
	if len(freshSync.Users) != 1 || freshSync.Users[0].Email != "normal-user@example.com" {
		t.Fatalf("static relay credentials leaked into freshSync.Users: %+v", freshSync.Users)
	}
}

func TestNodeService_RemoteLogs_CacheWaiterWakeupTimeoutAndWhitelist(t *testing.T) {
	nodeSvc, _, _, nodeRepo, _, _ := setupNodeServiceTestEnv(t)
	nodeSvc.WithLogWaitTimeout(120 * time.Millisecond)
	ctx := context.Background()

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Log Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Seed baseline heartbeat metrics to verify lightweight log ack preserves them
	if err := nodeRepo.UpdateHeartbeat(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: "c1",
		UsersHash:  "u1",
		HostMetrics: &domain.AgentHostMetrics{
			CPUPercent:   25.5,
			MemPercent:   55.0,
			NicUpDelta:   500,
			NicDownDelta: 1500,
		},
	}, time.Now()); err != nil {
		t.Fatalf("seed heartbeat failed: %v", err)
	}

	// 1. Whitelist & input checks
	if _, err := nodeSvc.RequestNodeLogs(ctx, 0, "xray", 100, false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for nodeId=0, got %v", err)
	}
	if _, err := nodeSvc.RequestNodeLogs(ctx, created.ID, "bash;rm", 100, false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for disallowed service, got %v", err)
	}
	if _, err := nodeSvc.RequestNodeLogs(ctx, 99999, "xray", 100, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing node, got %v", err)
	}

	// 2. Timeout when agent doesn't sync returns Pending=true
	pendingResp, err := nodeSvc.RequestNodeLogs(ctx, created.ID, "xray", 50, false)
	if err != nil {
		t.Fatalf("timeout RequestNodeLogs should not return error, got %v", err)
	}
	if !pendingResp.Pending || pendingResp.Cached {
		t.Fatalf("expected Pending=true, Cached=false on timeout, got %+v", pendingResp)
	}

	// Normal heartbeat after timeout must NOT receive stale cancelled logRequest
	idleSync, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("idle HandleAgentSync failed: %v", err)
	}
	if idleSync.LogRequest != nil {
		t.Fatalf("expected nil LogRequest after timed-out request cleanup, got %+v", idleSync.LogRequest)
	}

	// 3. Live waiter wakeup via HandleAgentSync + lightweight logResult ack
	nodeSvc.WithLogWaitTimeout(2 * time.Second)
	type logCallResult struct {
		resp *service.RemoteNodeLogsResponse
		err  error
	}
	doneCh := make(chan logCallResult, 1)
	go func() {
		r, e := nodeSvc.RequestNodeLogs(ctx, created.ID, "xray-agent", 80, false)
		doneCh <- logCallResult{resp: r, err: e}
	}()

	// Poll HandleAgentSync until pending LogRequest is delivered
	var deliveredReq *domain.AgentLogRequest
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
		if err != nil {
			t.Fatalf("HandleAgentSync failed: %v", err)
		}
		if syncResp.LogRequest != nil {
			deliveredReq = syncResp.LogRequest
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if deliveredReq == nil {
		t.Fatalf("expected pending LogRequest delivered via HandleAgentSync")
	}
	if deliveredReq.Service != "xray-agent" || deliveredReq.Lines != 80 {
		t.Fatalf("unexpected LogRequest: %+v", deliveredReq)
	}

	// Agent sends lightweight ack with nil HostMetrics and nil TrafficBatch
	if _, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		LogResult: &domain.AgentLogResult{
			RequestID:       deliveredReq.RequestID,
			Service:         "xray-agent",
			Lines:           []string{"line-1", "line-2", "line-3"},
			CollectedAtUnix: 1732018802,
		},
	}); err != nil {
		t.Fatalf("logResult HandleAgentSync failed: %v", err)
	}

	res := <-doneCh
	if res.err != nil {
		t.Fatalf("RequestNodeLogs failed: %v", res.err)
	}
	if res.resp.Pending || res.resp.Cached || len(res.resp.Lines) != 3 || res.resp.CollectedAt != 1732018802 {
		t.Fatalf("unexpected RequestNodeLogs response: %+v", res.resp)
	}

	// Verify baseline CPU/NIC counters were NOT zeroed out by lightweight log ack
	nodeAfter, _ := nodeRepo.GetByID(ctx, created.ID)
	if nodeAfter.CPUPercent != 25.5 || nodeAfter.NicUpBytes != 500 {
		t.Fatalf("lightweight log ack overwrote metrics: %+v", nodeAfter)
	}

	// 4. Subsequent request within 30s hits cache without queuing another LogRequest
	cachedResp, err := nodeSvc.RequestNodeLogs(ctx, created.ID, "xray-agent", 2, false)
	if err != nil {
		t.Fatalf("cached RequestNodeLogs failed: %v", err)
	}
	if !cachedResp.Cached || cachedResp.Pending || len(cachedResp.Lines) != 2 || cachedResp.Lines[0] != "line-2" {
		t.Fatalf("expected cached last 2 lines [line-2 line-3], got %+v", cachedResp)
	}
	afterCacheSync, _ := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if afterCacheSync.LogRequest != nil {
		t.Fatalf("cache hit must not queue a LogRequest")
	}
}
