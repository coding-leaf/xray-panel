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
	nodeSvc := service.NewNodeService(nodeRepo, inboundRepo, userRepo, compiler)

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

func TestNodeService_RemoteLogs_AsyncCommandDedupCancelSafeCacheTTLAndPurge(t *testing.T) {
	nodeSvc, _, _, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	nodeSvc.WithNowFunc(func() time.Time { return now })

	node1, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{Name: "Log Node 1", Host: "127.0.0.1"})
	if err != nil {
		t.Fatalf("CreateNode node1 failed: %v", err)
	}
	node2, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{Name: "Log Node 2", Host: "127.0.0.2"})
	if err != nil {
		t.Fatalf("CreateNode node2 failed: %v", err)
	}

	// 1. Whitelist & input checks
	if _, err := nodeSvc.RequestNodeLogs(ctx, 0, "xray", 100, false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for nodeId=0, got %v", err)
	}
	if _, err := nodeSvc.RequestNodeLogs(ctx, node1.ID, "bash;rm", 100, false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for disallowed service, got %v", err)
	}
	if _, err := nodeSvc.RequestNodeLogs(ctx, 99999, "xray", 100, false); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing node, got %v", err)
	}

	// 2. Immediate async enqueue returns pending=true + commandId + status="queued"
	reqCtx, cancel := context.WithCancel(ctx)
	resp1, err := nodeSvc.RequestNodeLogs(reqCtx, node1.ID, "xray", 50, false)
	if err != nil {
		t.Fatalf("RequestNodeLogs #1 failed: %v", err)
	}
	if !resp1.Pending || resp1.Cached || resp1.CommandID == "" || resp1.Status != domain.NodeCommandStatusQueued {
		t.Fatalf("unexpected async log enqueue response: %+v", resp1)
	}
	// Cancelling the HTTP caller context must NEVER delete the queued command!
	cancel()

	// 3. Duplicate trigger for same (nodeID, service) reuses commandId and bumps lines if larger while queued
	resp2, err := nodeSvc.RequestNodeLogs(ctx, node1.ID, "xray", 120, false)
	if err != nil {
		t.Fatalf("RequestNodeLogs #2 failed: %v", err)
	}
	if resp2.CommandID != resp1.CommandID || resp2.Status != domain.NodeCommandStatusQueued {
		t.Fatalf("expected deduplicated commandId %q, got %+v", resp1.CommandID, resp2)
	}

	// Different service on same node gets its own command (no overwrite!)
	respAgentSvc, err := nodeSvc.RequestNodeLogs(ctx, node1.ID, "xray-agent", 60, false)
	if err != nil {
		t.Fatalf("RequestNodeLogs xray-agent failed: %v", err)
	}
	if respAgentSvc.CommandID == "" || respAgentSvc.CommandID == resp1.CommandID {
		t.Fatalf("expected independent command for xray-agent, got %+v", respAgentSvc)
	}

	// 4. HandleAgentSync pops both queued commands in order and transitions them to dispatched
	syncResp, err := nodeSvc.HandleAgentSync(ctx, node1.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("HandleAgentSync failed: %v", err)
	}
	if len(syncResp.Commands) != 2 {
		t.Fatalf("expected 2 dispatched commands, got %+v", syncResp.Commands)
	}
	if syncResp.Commands[0].ID != resp1.CommandID || syncResp.Commands[0].Lines != 120 || syncResp.Commands[0].Service != "xray" {
		t.Fatalf("expected bumped lines=120 for first command, got %+v", syncResp.Commands[0])
	}
	if syncResp.Commands[1].ID != respAgentSvc.CommandID || syncResp.Commands[1].Service != "xray-agent" {
		t.Fatalf("unexpected second command: %+v", syncResp.Commands[1])
	}

	// Next sync does NOT re-deliver already dispatched commands
	syncResp2, err := nodeSvc.HandleAgentSync(ctx, node1.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("HandleAgentSync #2 failed: %v", err)
	}
	if len(syncResp2.Commands) != 0 {
		t.Fatalf("expected 0 commands on second sync while dispatched, got %+v", syncResp2.Commands)
	}

	// Status snapshot shows dispatched, and cross-node query is rejected with ErrNotFound
	snapDispatched, err := nodeSvc.GetNodeCommandStatus(ctx, node1.ID, resp1.CommandID)
	if err != nil || snapDispatched.Status != domain.NodeCommandStatusDispatched {
		t.Fatalf("expected dispatched snapshot, got %+v err=%v", snapDispatched, err)
	}
	if _, err := nodeSvc.GetNodeCommandStatus(ctx, node2.ID, resp1.CommandID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-node GetNodeCommandStatus, got %v", err)
	}

	// 5. Cross-node ReportAgentCommandResult is rejected with ErrNotFound
	if err := nodeSvc.ReportAgentCommandResult(ctx, node2.ID, &domain.AgentCommandResult{
		ID:      resp1.CommandID,
		Type:    domain.NodeCommandTypeLogTail,
		Status:  domain.NodeCommandStatusSucceeded,
		Service: "xray",
		Lines:   []string{"spoof"},
	}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-node ReportAgentCommandResult, got %v", err)
	}

	// Owning node reports succeeded result -> updates snapshot and populates 30s logCache
	if err := nodeSvc.ReportAgentCommandResult(ctx, node1.ID, &domain.AgentCommandResult{
		ID:              resp1.CommandID,
		Type:            domain.NodeCommandTypeLogTail,
		Status:          domain.NodeCommandStatusSucceeded,
		Service:         "xray",
		Lines:           []string{"line-1", "line-2", "line-3"},
		CollectedAtUnix: 1732018802,
	}); err != nil {
		t.Fatalf("ReportAgentCommandResult failed: %v", err)
	}

	// Duplicate report on terminal command is idempotent no-op
	if err := nodeSvc.ReportAgentCommandResult(ctx, node1.ID, &domain.AgentCommandResult{
		ID:     resp1.CommandID,
		Type:   domain.NodeCommandTypeLogTail,
		Status: domain.NodeCommandStatusFailed,
		Error:  "late duplicate",
	}); err != nil {
		t.Fatalf("idempotent duplicate ReportAgentCommandResult failed: %v", err)
	}

	snapDone, err := nodeSvc.GetNodeCommandStatus(ctx, node1.ID, resp1.CommandID)
	if err != nil {
		t.Fatalf("GetNodeCommandStatus after report failed: %v", err)
	}
	expectedCollectedAt := time.Unix(1732018802, 0).UTC().Format(time.RFC3339)
	if snapDone.Status != domain.NodeCommandStatusSucceeded || snapDone.LogResult == nil ||
		len(snapDone.LogResult.Lines) != 3 || snapDone.LogResult.CollectedAt != expectedCollectedAt {
		t.Fatalf("unexpected succeeded log snapshot: %+v", snapDone)
	}

	// 6. Subsequent RequestNodeLogs within 30s hits cache without queuing another command
	cachedResp, err := nodeSvc.RequestNodeLogs(ctx, node1.ID, "xray", 2, false)
	if err != nil {
		t.Fatalf("cached RequestNodeLogs failed: %v", err)
	}
	if !cachedResp.Cached || cachedResp.Pending || len(cachedResp.Lines) != 2 || cachedResp.Lines[0] != "line-2" {
		t.Fatalf("expected cached last 2 lines [line-2 line-3], got %+v", cachedResp)
	}
	afterCacheSync, _ := nodeSvc.HandleAgentSync(ctx, node1.ID, &domain.AgentSyncRequest{})
	if len(afterCacheSync.Commands) != 0 {
		t.Fatalf("cache hit must not queue a command, got %+v", afterCacheSync.Commands)
	}

	// 7. TTL expiration: advance clock > 180s -> pending xray-agent command expires; advance > 180s again -> pruned
	now = now.Add(181 * time.Second)
	snapExpired, err := nodeSvc.GetNodeCommandStatus(ctx, node1.ID, respAgentSvc.CommandID)
	if err != nil {
		t.Fatalf("expected expired snapshot still queryable during result retention, got err=%v", err)
	}
	if snapExpired.Status != domain.NodeCommandStatusExpired || snapExpired.Error == "" {
		t.Fatalf("expected status=expired with error, got %+v", snapExpired)
	}
	// First completed command was already terminal at t=0, so at t=181s (> 180s) it is pruned
	if _, err := nodeSvc.GetNodeCommandStatus(ctx, node1.ID, resp1.CommandID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected completed command pruned after result TTL, got err=%v", err)
	}

	// 8. DeleteNode purges node's in-memory commands and logCache
	freshCmd, err := nodeSvc.RequestNodeLogs(ctx, node1.ID, "xray", 50, true)
	if err != nil {
		t.Fatalf("fresh RequestNodeLogs failed: %v", err)
	}
	if err := nodeSvc.DeleteNode(ctx, node1.ID); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}
	if _, err := nodeSvc.GetNodeCommandStatus(ctx, node1.ID, freshCmd.CommandID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after DeleteNode memory purge, got %v", err)
	}
}

func TestNodeDomain_RefreshSyncStatus_ThreeStates(t *testing.T) {
	cases := []struct {
		name     string
		node     domain.Node
		expected string
		want     string
	}{
		{
			name:     "error takes precedence over matching hashes",
			node:     domain.Node{LastConfigHash: "abc", LastSyncError: "xray reload failed"},
			expected: "abc",
			want:     domain.NodeSyncStatusError,
		},
		{
			name:     "whitespace error does not count as error",
			node:     domain.Node{LastConfigHash: "abc", LastSyncError: "   "},
			expected: "abc",
			want:     domain.NodeSyncStatusSynced,
		},
		{
			name:     "matching non-empty hashes is synced",
			node:     domain.Node{LastConfigHash: "hash-1"},
			expected: "hash-1",
			want:     domain.NodeSyncStatusSynced,
		},
		{
			name:     "both empty is pending",
			node:     domain.Node{},
			expected: "",
			want:     domain.NodeSyncStatusPending,
		},
		{
			name:     "mismatched hashes is pending",
			node:     domain.Node{LastConfigHash: "hash-old"},
			expected: "hash-new",
			want:     domain.NodeSyncStatusPending,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.node
			n.RefreshSyncStatus(tc.expected)
			if n.ExpectedConfigHash != tc.expected {
				t.Fatalf("expected ExpectedConfigHash=%q, got %q", tc.expected, n.ExpectedConfigHash)
			}
			if n.ConfigSyncStatus != tc.want {
				t.Fatalf("expected ConfigSyncStatus=%q, got %q", tc.want, n.ConfigSyncStatus)
			}
		})
	}

	// Nil receiver safety
	var nilNode *domain.Node
	nilNode.RefreshSyncStatus("abc")
}

func TestNodeService_SyncStatusPopulation_AndCompilerFallback(t *testing.T) {
	nodeSvc, _, _, nodeRepo, inboundRepo, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Sync Status Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Initially never synced -> pending with non-empty expectedConfigHash
	got, err := nodeSvc.GetNode(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if got.ExpectedConfigHash == "" || got.ConfigSyncStatus != domain.NodeSyncStatusPending {
		t.Fatalf("expected non-empty expectedConfigHash and pending status, got hash=%q status=%q",
			got.ExpectedConfigHash, got.ConfigSyncStatus)
	}

	// Agent sync reports matching ConfigHash -> synced
	syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("HandleAgentSync initial failed: %v", err)
	}
	if _, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: syncResp.ConfigHash,
		UsersHash:  syncResp.UsersHash,
	}); err != nil {
		t.Fatalf("HandleAgentSync ack failed: %v", err)
	}

	list, err := nodeSvc.ListNodes(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if list[0].ConfigSyncStatus != domain.NodeSyncStatusSynced || list[0].ExpectedConfigHash != syncResp.ConfigHash {
		t.Fatalf("expected synced after hash match, got hash=%q status=%q",
			list[0].ExpectedConfigHash, list[0].ConfigSyncStatus)
	}

	// Mutate node inbound & invalidate -> pending again with new expectedConfigHash
	if err := inboundRepo.Create(ctx, &domain.Inbound{
		NodeID:       created.ID,
		Tag:          "edge-vless",
		Port:         443,
		Protocol:     "vless",
		Enabled:      true,
		SettingsJSON: `{"decryption":"none"}`,
	}); err != nil {
		t.Fatalf("Create inbound failed: %v", err)
	}
	nodeSvc.InvalidateNode(created.ID)

	afterChange, err := nodeSvc.GetNode(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNode after inbound change failed: %v", err)
	}
	if afterChange.ExpectedConfigHash == "" || afterChange.ExpectedConfigHash == syncResp.ConfigHash {
		t.Fatalf("expected updated expectedConfigHash, got %q", afterChange.ExpectedConfigHash)
	}
	if afterChange.ConfigSyncStatus != domain.NodeSyncStatusPending {
		t.Fatalf("expected pending after config change, got %q", afterChange.ConfigSyncStatus)
	}

	// Agent reports sync error -> error
	if _, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: afterChange.ExpectedConfigHash,
		UsersHash:  syncResp.UsersHash,
		LastError:  "xray validate failed",
	}); err != nil {
		t.Fatalf("HandleAgentSync error report failed: %v", err)
	}
	afterErr, _ := nodeSvc.GetNode(ctx, created.ID)
	if afterErr.ConfigSyncStatus != domain.NodeSyncStatusError {
		t.Fatalf("expected error status, got %q", afterErr.ConfigSyncStatus)
	}

	// Degraded snapshot build (e.g. corrupt outbounds_json on one node, or nil compiler) must not fail ListNodes/GetNode
	corrupt, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Corrupt Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode corrupt failed: %v", err)
	}
	if err := nodeRepo.UpdateNodeConfig(ctx, corrupt.ID, "{bad-json", "", ""); err != nil {
		t.Fatalf("UpdateNodeConfig failed: %v", err)
	}
	nodeSvc.InvalidateNode(corrupt.ID)

	listWithCorrupt, err := nodeSvc.ListNodes(ctx)
	if err != nil || len(listWithCorrupt) != 2 {
		t.Fatalf("ListNodes must not fail when one node snapshot fails: len=%d err=%v", len(listWithCorrupt), err)
	}
	if listWithCorrupt[1].ExpectedConfigHash != "" || listWithCorrupt[1].ConfigSyncStatus != domain.NodeSyncStatusPending {
		t.Fatalf("expected graceful empty hash + pending fallback, got hash=%q status=%q",
			listWithCorrupt[1].ExpectedConfigHash, listWithCorrupt[1].ConfigSyncStatus)
	}

	nilCompilerSvc := service.NewNodeService(nodeRepo, inboundRepo, nil, nil)
	nilList, err := nilCompilerSvc.ListNodes(ctx)
	if err != nil || len(nilList) != 2 {
		t.Fatalf("ListNodes with nil compiler failed: %v", err)
	}
	if nilList[0].ExpectedConfigHash != "" || nilList[0].ConfigSyncStatus != domain.NodeSyncStatusError {
		t.Fatalf("expected empty expectedConfigHash and preserved error status with nil compiler, got %+v", nilList[0])
	}
}

func TestNodeService_RemoteGeoUpdate_AsyncCommandDedupSyncFallbackAndExpiration(t *testing.T) {
	nodeSvc, _, _, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	now := time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC)
	nodeSvc.WithNowFunc(func() time.Time { return now })

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Geo Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// 1. Input & existence checks
	if _, err := nodeSvc.RequestNodeGeoUpdate(ctx, 0); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for nodeId=0, got %v", err)
	}
	if _, err := nodeSvc.RequestNodeGeoUpdate(ctx, 99999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing node, got %v", err)
	}

	// 2. Concurrent callers deduplicate onto the same queued commandId immediately
	r1, err := nodeSvc.RequestNodeGeoUpdate(ctx, created.ID)
	if err != nil {
		t.Fatalf("RequestNodeGeoUpdate #1 failed: %v", err)
	}
	r2, err := nodeSvc.RequestNodeGeoUpdate(ctx, created.ID)
	if err != nil {
		t.Fatalf("RequestNodeGeoUpdate #2 failed: %v", err)
	}
	if !r1.Pending || r1.CommandID == "" || r1.Status != domain.NodeCommandStatusQueued {
		t.Fatalf("unexpected geo enqueue #1: %+v", r1)
	}
	if r2.CommandID != r1.CommandID {
		t.Fatalf("expected deduplicated commandId %q, got %q", r1.CommandID, r2.CommandID)
	}

	// 3. Agent picks up command via HandleAgentSync and reports result via Sync fallback CommandResults
	syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("HandleAgentSync #1 failed: %v", err)
	}
	if len(syncResp.Commands) != 1 || syncResp.Commands[0].ID != r1.CommandID || syncResp.Commands[0].Type != domain.NodeCommandTypeGeoDataUpdate {
		t.Fatalf("expected geodata_update command, got %+v", syncResp.Commands)
	}

	// Request while dispatched still reuses same commandId with status="dispatched"
	r3, err := nodeSvc.RequestNodeGeoUpdate(ctx, created.ID)
	if err != nil {
		t.Fatalf("RequestNodeGeoUpdate while dispatched failed: %v", err)
	}
	if r3.CommandID != r1.CommandID || r3.Status != domain.NodeCommandStatusDispatched {
		t.Fatalf("expected reused dispatched commandId %q, got %+v", r1.CommandID, r3)
	}

	// Fallback result via HandleAgentSync.CommandResults completes the command
	ackResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: syncResp.ConfigHash,
		UsersHash:  syncResp.UsersHash,
		CommandResults: []domain.AgentCommandResult{
			{
				ID:            r1.CommandID,
				Type:          domain.NodeCommandTypeGeoDataUpdate,
				Status:        domain.NodeCommandStatusSucceeded,
				GeoIPSize:     10485760,
				GeoSiteSize:   5242880,
				DurationMs:    3240,
				UpdatedAtUnix: 1732020004,
			},
		},
	})
	if err != nil {
		t.Fatalf("fallback CommandResults HandleAgentSync failed: %v", err)
	}
	if len(ackResp.Commands) != 0 {
		t.Fatalf("expected 0 commands after completion, got %+v", ackResp.Commands)
	}

	snap, err := nodeSvc.GetNodeCommandStatus(ctx, created.ID, r1.CommandID)
	if err != nil {
		t.Fatalf("GetNodeCommandStatus failed: %v", err)
	}
	expectedUpdatedAt := time.Unix(1732020004, 0).UTC().Format(time.RFC3339)
	if snap.Status != domain.NodeCommandStatusSucceeded || snap.GeoResult == nil ||
		snap.GeoResult.GeoIPSize != 10485760 || snap.GeoResult.GeoSiteSize != 5242880 ||
		snap.GeoResult.DurationMs != 3240 || snap.GeoResult.UpdatedAt != expectedUpdatedAt {
		t.Fatalf("unexpected geo command snapshot: %+v", snap)
	}

	// 4. After completion, new geo update creates a fresh commandId; if agent never picks it up > 180s -> expires
	r4, err := nodeSvc.RequestNodeGeoUpdate(ctx, created.ID)
	if err != nil {
		t.Fatalf("RequestNodeGeoUpdate #4 failed: %v", err)
	}
	if r4.CommandID == r1.CommandID {
		t.Fatalf("expected new commandId after prior completion")
	}
	now = now.Add(181 * time.Second)
	expiredSnap, err := nodeSvc.GetNodeCommandStatus(ctx, created.ID, r4.CommandID)
	if err != nil {
		t.Fatalf("GetNodeCommandStatus expired failed: %v", err)
	}
	if expiredSnap.Status != domain.NodeCommandStatusExpired {
		t.Fatalf("expected status=expired, got %+v", expiredSnap)
	}
}

func TestNodeService_Snapshot_IncludesRemoteSubRouteRulesWithoutRoutingSave(t *testing.T) {
	nodeSvc, configSvc, _, nodeRepo, _, _ := setupNodeServiceTestEnv(t)
	configSvc.WithNodeRepo(nodeRepo)
	ctx := context.Background()

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "JP SubRoute",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// Add a remote outbound target for the SubRoute without ever calling SaveRoutingConfigByNode.
	if err := configSvc.SaveOutboundByNode(ctx, created.ID, domain.Outbound{
		Tag:      "warp-out",
		Protocol: "freedom",
	}); err != nil {
		t.Fatalf("SaveOutboundByNode failed: %v", err)
	}

	// Create a remote inbound carrying an enabled SubRoute.
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:   created.ID,
		Tag:      "jp-vless",
		Port:     18443,
		Protocol: "vless",
		Enabled:  true,
		SubRoutesJson: `[
			{"id":"sr-1","name":"JP Warp","routeId":101,"outboundTag":"warp-out","enabled":true,"allowedUsers":["alice@example.com"]}
		]`,
	}); err != nil {
		t.Fatalf("CreateInbound failed: %v", err)
	}

	assertHasSubRouteRule := func(label string, rawJSON []byte) {
		t.Helper()
		var parsed struct {
			Routing struct {
				Rules []struct {
					InboundTag  []string `json:"inboundTag"`
					VlessRoute  string   `json:"vlessRoute"`
					OutboundTag string   `json:"outboundTag"`
					User        []string `json:"user"`
				} `json:"rules"`
			} `json:"routing"`
		}
		if err := json.Unmarshal(rawJSON, &parsed); err != nil {
			t.Fatalf("%s: unmarshal compiled config failed: %v", label, err)
		}
		for _, r := range parsed.Routing.Rules {
			if r.VlessRoute == "101" && r.OutboundTag == "warp-out" && len(r.InboundTag) == 1 && r.InboundTag[0] == "jp-vless" && len(r.User) == 1 && r.User[0] == "alice@example.com" {
				return
			}
		}
		t.Fatalf("%s: missing expected SubRoute vlessRoute=101 rule in compiled routing: %+v", label, parsed.Routing.Rules)
	}

	// 1. Agent snapshot from NodeService.HandleAgentSync
	syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("HandleAgentSync failed: %v", err)
	}
	assertHasSubRouteRule("NodeService.HandleAgentSync snapshot", []byte(syncResp.ConfigJSON))

	// 2. ConfigView preview from ConfigService.GetNodeCompiledConfigRaw
	previewRaw, err := configSvc.GetNodeCompiledConfigRaw(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetNodeCompiledConfigRaw failed: %v", err)
	}
	assertHasSubRouteRule("ConfigService.GetNodeCompiledConfigRaw", previewRaw)

	// 3. RoutingView UI from ConfigService.GetRoutingConfigByNode
	uiRouting, err := configSvc.GetRoutingConfigByNode(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRoutingConfigByNode failed: %v", err)
	}
	foundUI := false
	for _, r := range uiRouting.Rules {
		if r.VlessRoute == "101" && r.OutboundTag == "warp-out" {
			foundUI = true
			break
		}
	}
	if !foundUI {
		t.Fatalf("GetRoutingConfigByNode missing SubRoute rule vlessRoute=101: %+v", uiRouting.Rules)
	}
}

func TestNodeService_RenderInstallScript_EnvFileAndInterval(t *testing.T) {
	nodeSvc, _, _, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Secure Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	script, err := nodeSvc.RenderInstallScript(ctx, created.ID, "https://panel.example.test")
	if err != nil {
		t.Fatalf("RenderInstallScript failed: %v", err)
	}

	for _, mustHave := range []string{
		"uname -m",
		"x86_64|amd64",
		"aarch64|arm64",
		"xray binary not found",
		"AGENT_BIN_URL",
		"https://github.com/coding-leaf/xray-panel/releases/latest/download/xray-agent-linux-${ARCH}",
		"curl -fsSL",
		"wget -qO",
		`install -m 0755 "$TMP_BIN" /usr/local/bin/xray-agent`,
		"/etc/xray-agent/agent.env",
		"0600",
		"EnvironmentFile=/etc/xray-agent/agent.env",
		"MASTER_URL=",
		"AGENT_TOKEN=",
		"--interval 60s",
		"127.0.0.1:8080",
	} {
		if !strings.Contains(script, mustHave) {
			t.Errorf("install script missing %q:\n%s", mustHave, script)
		}
	}

	if strings.Contains(script, created.Token) {
		t.Fatalf("install script leaked plaintext node token")
	}

	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ExecStart=") {
			if strings.Contains(trimmed, "--token") {
				t.Errorf("ExecStart must not pass plaintext --token CLI arg: %s", trimmed)
			}
			if !strings.Contains(trimmed, "--interval 60s") {
				t.Errorf("ExecStart must use --interval 60s: %s", trimmed)
			}
		}
	}
}

func TestNodeService_WriteEndpoints_PopulateSyncStatus(t *testing.T) {
	nodeSvc, _, _, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	// 1. CreateNode must populate ExpectedConfigHash and ConfigSyncStatus
	created, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "Write Sync Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if created.ExpectedConfigHash == "" || created.ConfigSyncStatus != domain.NodeSyncStatusPending {
		t.Fatalf("CreateNode missing sync status: hash=%q status=%q", created.ExpectedConfigHash, created.ConfigSyncStatus)
	}

	// Sync agent so node becomes "synced"
	syncResp, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("initial sync failed: %v", err)
	}
	if _, err := nodeSvc.HandleAgentSync(ctx, created.ID, &domain.AgentSyncRequest{
		ConfigHash: syncResp.ConfigHash,
		UsersHash:  syncResp.UsersHash,
	}); err != nil {
		t.Fatalf("ack sync failed: %v", err)
	}

	// 2. UpdateNode must populate ExpectedConfigHash and ConfigSyncStatus
	newName := "Write Sync Node Renamed"
	updated, err := nodeSvc.UpdateNode(ctx, created.ID, domain.UpdateNodeDTO{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateNode failed: %v", err)
	}
	if updated.ExpectedConfigHash != syncResp.ConfigHash || updated.ConfigSyncStatus != domain.NodeSyncStatusSynced {
		t.Fatalf("UpdateNode missing sync status: hash=%q status=%q", updated.ExpectedConfigHash, updated.ConfigSyncStatus)
	}

	// 3. SetNodeDesiredState must populate ExpectedConfigHash and ConfigSyncStatus
	drained, err := nodeSvc.SetNodeDesiredState(ctx, created.ID, domain.NodeStateDraining)
	if err != nil {
		t.Fatalf("SetNodeDesiredState failed: %v", err)
	}
	if drained.ExpectedConfigHash != syncResp.ConfigHash || drained.ConfigSyncStatus != domain.NodeSyncStatusSynced {
		t.Fatalf("SetNodeDesiredState missing sync status: hash=%q status=%q", drained.ExpectedConfigHash, drained.ConfigSyncStatus)
	}

	// 4. ResetNodeCycleTraffic must populate ExpectedConfigHash and ConfigSyncStatus
	reset, err := nodeSvc.ResetNodeCycleTraffic(ctx, created.ID)
	if err != nil {
		t.Fatalf("ResetNodeCycleTraffic failed: %v", err)
	}
	if reset.ExpectedConfigHash != syncResp.ConfigHash || reset.ConfigSyncStatus != domain.NodeSyncStatusSynced {
		t.Fatalf("ResetNodeCycleTraffic missing sync status: hash=%q status=%q", reset.ExpectedConfigHash, reset.ConfigSyncStatus)
	}
}

func TestNodeService_HandleAgentSync_DiskConfigHashDelivery(t *testing.T) {
	nodeSvc, configSvc, userSvc, _, _, _ := setupNodeServiceTestEnv(t)
	ctx := context.Background()

	node, err := nodeSvc.CreateNode(ctx, domain.CreateNodeDTO{
		Name: "DiskHash Node",
		Host: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if err := configSvc.CreateInbound(ctx, &domain.Inbound{
		NodeID:   node.ID,
		Tag:      "edge-vless",
		Protocol: "vless",
		Port:     443,
		Listen:   "0.0.0.0",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("CreateInbound failed: %v", err)
	}
	if _, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "alice@example.com",
		InboundTags: []string{"edge-vless"},
	}); err != nil {
		t.Fatalf("CreateUser alice failed: %v", err)
	}

	// 1. Initial sync returns full ConfigJSON.
	resp1, err := nodeSvc.HandleAgentSync(ctx, node.ID, &domain.AgentSyncRequest{})
	if err != nil {
		t.Fatalf("initial HandleAgentSync failed: %v", err)
	}
	if resp1.ConfigJSON == "" {
		t.Fatalf("expected non-empty ConfigJSON on initial sync")
	}
	sum1 := sha256.Sum256([]byte(resp1.ConfigJSON))
	diskHash1 := hex.EncodeToString(sum1[:])

	// 2. Matching ConfigHash, UsersHash, and DiskConfigHash -> short-circuit omits ConfigJSON.
	resp2, err := nodeSvc.HandleAgentSync(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:     resp1.ConfigHash,
		UsersHash:      resp1.UsersHash,
		DiskConfigHash: diskHash1,
	})
	if err != nil {
		t.Fatalf("steady HandleAgentSync failed: %v", err)
	}
	if !resp2.ConfigUnchanged || !resp2.UsersUnchanged || resp2.ConfigJSON != "" {
		t.Fatalf("expected steady short-circuit without ConfigJSON, got unchanged=%v/%v configLen=%d", resp2.ConfigUnchanged, resp2.UsersUnchanged, len(resp2.ConfigJSON))
	}

	// 3. Disk tampering/drift even when ConfigHash and UsersHash match -> ConfigUnchanged=true, ConfigJSON delivered!
	respDrift, err := nodeSvc.HandleAgentSync(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:     resp1.ConfigHash,
		UsersHash:      resp1.UsersHash,
		DiskConfigHash: "tampered-disk-hash",
	})
	if err != nil {
		t.Fatalf("drift HandleAgentSync failed: %v", err)
	}
	if !respDrift.ConfigUnchanged || !respDrift.UsersUnchanged {
		t.Fatalf("expected ConfigUnchanged=true UsersUnchanged=true on disk-only drift, got %v/%v", respDrift.ConfigUnchanged, respDrift.UsersUnchanged)
	}
	if respDrift.ConfigJSON != resp1.ConfigJSON {
		t.Fatalf("expected Master to deliver authoritative ConfigJSON when DiskConfigHash mismatches")
	}

	// 4. User mutation: new Agent with old DiskConfigHash gets ConfigUnchanged=true, UsersUnchanged=false, AND updated ConfigJSON.
	if _, err := userSvc.CreateUser(ctx, service.CreateUserDTO{
		Email:       "bob@example.com",
		InboundTags: []string{"edge-vless"},
	}); err != nil {
		t.Fatalf("CreateUser bob failed: %v", err)
	}
	respUserNewAgent, err := nodeSvc.HandleAgentSync(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash:     resp1.ConfigHash,
		UsersHash:      resp1.UsersHash,
		DiskConfigHash: diskHash1,
	})
	if err != nil {
		t.Fatalf("user mutation new-agent sync failed: %v", err)
	}
	if !respUserNewAgent.ConfigUnchanged || respUserNewAgent.UsersUnchanged {
		t.Fatalf("expected ConfigUnchanged=true UsersUnchanged=false, got %v/%v", respUserNewAgent.ConfigUnchanged, respUserNewAgent.UsersUnchanged)
	}
	if respUserNewAgent.ConfigJSON == "" || !strings.Contains(respUserNewAgent.ConfigJSON, "bob@example.com") {
		t.Fatalf("expected updated ConfigJSON containing bob@example.com for new agent, got %q", respUserNewAgent.ConfigJSON)
	}

	// 5. Legacy Agent compatibility: omits DiskConfigHash ("") -> Master omits ConfigJSON when ConfigUnchanged=true.
	respLegacyAgent, err := nodeSvc.HandleAgentSync(ctx, node.ID, &domain.AgentSyncRequest{
		ConfigHash: resp1.ConfigHash,
		UsersHash:  resp1.UsersHash,
	})
	if err != nil {
		t.Fatalf("user mutation legacy-agent sync failed: %v", err)
	}
	if !respLegacyAgent.ConfigUnchanged || respLegacyAgent.UsersUnchanged {
		t.Fatalf("expected ConfigUnchanged=true UsersUnchanged=false for legacy agent, got %v/%v", respLegacyAgent.ConfigUnchanged, respLegacyAgent.UsersUnchanged)
	}
	if respLegacyAgent.ConfigJSON != "" {
		t.Fatalf("expected empty ConfigJSON for legacy agent omitting DiskConfigHash, got len=%d", len(respLegacyAgent.ConfigJSON))
	}
}
