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
