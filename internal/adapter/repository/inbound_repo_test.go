package repository

import (
	"context"
	"errors"
	"testing"

	"panel/internal/domain"
)

func TestInboundRepository_Update_PreservesTraffic(t *testing.T) {
	db := setupTestDB(t)
	repo := NewInboundRepository(db)
	ctx := context.Background()

	inbound := &domain.Inbound{
		Tag:       "vless-in",
		Port:      443,
		Listen:    "0.0.0.0",
		Protocol:  "vless",
		Remark:    "Original Remark",
		UpBytes:   1024,
		DownBytes: 2048,
		Enabled:   true,
	}
	if err := repo.Create(ctx, inbound); err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	// 模拟前端更新配置：构造更新对象，流量字段为 0，更新端口和备注
	updateInbound := &domain.Inbound{
		ID:        inbound.ID,
		Tag:       "vless-in",
		Port:      8443,
		Listen:    "0.0.0.0",
		Protocol:  "vless",
		Remark:    "Updated Remark",
		UpBytes:   0,
		DownBytes: 0,
		Enabled:   true,
	}

	if err := repo.Update(ctx, updateInbound); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	fresh, err := repo.GetByID(ctx, inbound.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fresh.Port != 8443 {
		t.Errorf("expected Port=8443, got %d", fresh.Port)
	}
	if fresh.Remark != "Updated Remark" {
		t.Errorf("expected Remark='Updated Remark', got %s", fresh.Remark)
	}
	if fresh.UpBytes != 1024 || fresh.DownBytes != 2048 {
		t.Errorf("traffic was overwritten by Update! UpBytes=%d (want 1024), DownBytes=%d (want 2048)",
			fresh.UpBytes, fresh.DownBytes)
	}
}

func TestInboundRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewInboundRepository(db)
	ctx := context.Background()

	// 1. NotFound tests
	_, err := repo.GetByID(ctx, 999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent ID, got %v", err)
	}

	_, err = repo.GetByTag(ctx, "non-existent")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent Tag, got %v", err)
	}

	// 2. Create
	in1 := &domain.Inbound{
		Tag:      "trojan-in",
		Port:     10443,
		Listen:   "127.0.0.1",
		Protocol: "trojan",
		Remark:   "Trojan Node",
		Enabled:  true,
	}
	in2 := &domain.Inbound{
		Tag:      "ss-in",
		Port:     10444,
		Listen:   "0.0.0.0",
		Protocol: "shadowsocks",
		Remark:   "SS Node",
		Enabled:  false,
	}

	if err := repo.Create(ctx, in1); err != nil {
		t.Fatalf("failed to create in1: %v", err)
	}
	if err := repo.Create(ctx, in2); err != nil {
		t.Fatalf("failed to create in2: %v", err)
	}

	// 3. GetByID
	got1, err := repo.GetByID(ctx, in1.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got1.Tag != "trojan-in" || got1.Port != 10443 {
		t.Errorf("unexpected inbound fields: %+v", got1)
	}

	// 4. GetByTag
	got2, err := repo.GetByTag(ctx, "ss-in")
	if err != nil {
		t.Fatalf("GetByTag failed: %v", err)
	}
	if got2.ID != in2.ID || got2.Protocol != "shadowsocks" {
		t.Errorf("unexpected inbound fields: %+v", got2)
	}

	// 5. ListAll
	list, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 inbounds, got %d", len(list))
	}

	// 6. Delete
	if err := repo.Delete(ctx, in1.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	_, err = repo.GetByID(ctx, in1.ID)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}

	remaining, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll after delete failed: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != in2.ID {
		t.Fatalf("expected remaining in2, got %v", remaining)
	}
}

func TestInboundRepository_AddTraffic(t *testing.T) {
	db := setupTestDB(t)
	repo := NewInboundRepository(db)
	ctx := context.Background()

	in := &domain.Inbound{
		Tag:       "vmess-in",
		Port:      10086,
		Listen:    "0.0.0.0",
		Protocol:  "vmess",
		UpBytes:   100,
		DownBytes: 200,
		Enabled:   true,
	}
	if err := repo.Create(ctx, in); err != nil {
		t.Fatalf("failed to create inbound: %v", err)
	}

	if err := repo.AddTraffic(ctx, "vmess-in", 50, 80); err != nil {
		t.Fatalf("AddTraffic failed: %v", err)
	}

	fresh, err := repo.GetByTag(ctx, "vmess-in")
	if err != nil {
		t.Fatalf("GetByTag failed: %v", err)
	}
	if fresh.UpBytes != 150 || fresh.DownBytes != 280 {
		t.Errorf("expected UpBytes=150, DownBytes=280, got up=%d down=%d", fresh.UpBytes, fresh.DownBytes)
	}
}
