package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"panel/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GORMNodeRepository struct {
	db *gorm.DB
}

var (
	_ domain.NodeRepository             = (*GORMNodeRepository)(nil)
	_ domain.NodeTrafficBatchRepository = (*GORMNodeRepository)(nil)
	_ domain.NodeQuotaRepository        = (*GORMNodeRepository)(nil)
	_ domain.NodeConfigRepository       = (*GORMNodeRepository)(nil)
)

func NewNodeRepository(db *gorm.DB) *GORMNodeRepository {
	return &GORMNodeRepository{db: db}
}

func nodeTrafficDailyOnConflictClause(updates map[string]interface{}) clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}, {Name: "date"}},
		DoUpdates: clause.Assignments(updates),
	}
}

func (r *GORMNodeRepository) Create(ctx context.Context, node *domain.Node) error {
	return r.db.WithContext(ctx).Create(node).Error
}

// Update persists admin metadata, token credentials, and quota policy without overwriting heartbeat or cycle traffic counters.
func (r *GORMNodeRepository) Update(ctx context.Context, node *domain.Node) error {
	res := r.db.WithContext(ctx).Model(&domain.Node{}).
		Where("id = ?", node.ID).
		Updates(map[string]interface{}{
			"name":                node.Name,
			"host":                node.Host,
			"region":              node.Region,
			"token_hash":          node.TokenHash,
			"token_prefix":        node.TokenPrefix,
			"enabled":             node.Enabled,
			"desired_state":       node.DesiredState,
			"auto_state_reason":   node.AutoStateReason,
			"remark":              node.Remark,
			"traffic_limit_bytes": node.TrafficLimitBytes,
			"billing_reset_day":   node.BillingResetDay,
			"traffic_count_mode":  node.TrafficCountMode,
			"traffic_multiplier":  node.TrafficMultiplier,
			"warn_threshold_pct":  node.WarnThresholdPct,
			"drain_threshold_pct": node.DrainThresholdPct,
			"break_threshold_pct": node.BreakThresholdPct,
			"updated_at":          node.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateHeartbeat atomically records agent heartbeat metadata, NIC traffic deltas, and daily NIC traffic logs.
func (r *GORMNodeRepository) UpdateHeartbeat(ctx context.Context, nodeID uint, req *domain.AgentSyncRequest, seenAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		isLightweightResultAck := (req.LogResult != nil || req.GeoUpdateResult != nil) && req.TrafficBatch == nil && req.HostMetrics == nil
		updates := map[string]interface{}{
			"last_seen_at": seenAt,
			"updated_at":   seenAt,
		}
		if !isLightweightResultAck {
			updates["agent_version"] = req.AgentVersion
			updates["xray_version"] = req.XrayVersion
			updates["last_config_hash"] = req.ConfigHash
			updates["last_users_hash"] = req.UsersHash
			updates["last_sync_error"] = req.LastError
		} else {
			if req.AgentVersion != "" {
				updates["agent_version"] = req.AgentVersion
			}
			if req.XrayVersion != "" {
				updates["xray_version"] = req.XrayVersion
			}
			if req.ConfigHash != "" {
				updates["last_config_hash"] = req.ConfigHash
			}
			if req.UsersHash != "" {
				updates["last_users_hash"] = req.UsersHash
			}
			if req.LastError != "" {
				updates["last_sync_error"] = req.LastError
			}
		}
		var nicUp, nicDown int64
		if req.HostMetrics != nil {
			updates["cpu_percent"] = req.HostMetrics.CPUPercent
			updates["mem_percent"] = req.HostMetrics.MemPercent
			nicUp = maxZero64(req.HostMetrics.NicUpDelta)
			nicDown = maxZero64(req.HostMetrics.NicDownDelta)
			if nicUp > 0 {
				updates["nic_up_bytes"] = gorm.Expr("nic_up_bytes + ?", nicUp)
				updates["cycle_nic_up_bytes"] = gorm.Expr("cycle_nic_up_bytes + ?", nicUp)
			}
			if nicDown > 0 {
				updates["nic_down_bytes"] = gorm.Expr("nic_down_bytes + ?", nicDown)
				updates["cycle_nic_down_bytes"] = gorm.Expr("cycle_nic_down_bytes + ?", nicDown)
			}
		}
		res := tx.Model(&domain.Node{}).Where("id = ?", nodeID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}

		if nicUp > 0 || nicDown > 0 {
			daily := domain.NodeTrafficDaily{
				NodeID:       nodeID,
				Date:         seenAt.Format("2006-01-02"),
				NicUpBytes:   nicUp,
				NicDownBytes: nicDown,
				UpdatedAt:    seenAt,
			}
			if err := tx.Clauses(nodeTrafficDailyOnConflictClause(map[string]interface{}{
				"nic_up_bytes":   gorm.Expr("node_traffic_dailies.nic_up_bytes + excluded.nic_up_bytes"),
				"nic_down_bytes": gorm.Expr("node_traffic_dailies.nic_down_bytes + excluded.nic_down_bytes"),
				"updated_at":     seenAt,
			})).Create(&daily).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GORMNodeRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&domain.Node{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *GORMNodeRepository) GetByID(ctx context.Context, id uint) (*domain.Node, error) {
	var node domain.Node
	err := r.db.WithContext(ctx).First(&node, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (r *GORMNodeRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Node, error) {
	var node domain.Node
	err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (r *GORMNodeRepository) ListAll(ctx context.Context) ([]domain.Node, error) {
	var nodes []domain.Node
	err := r.db.WithContext(ctx).Order("id ASC").Find(&nodes).Error
	return nodes, err
}

func (r *GORMNodeRepository) CountInbounds(ctx context.Context, nodeID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&domain.Inbound{}).Where("node_id = ?", nodeID).Count(&count).Error
	return count, err
}

// ApplyNodeTrafficBatch atomically deduplicates (node_id, batch_id) and accumulates user, inbound, traffic log, and node counters.
func (r *GORMNodeRepository) ApplyNodeTrafficBatch(
	ctx context.Context,
	nodeID uint,
	batch *domain.AgentTrafficBatch,
	date string,
) (bool, []domain.User, error) {
	if batch == nil || strings.TrimSpace(batch.BatchID) == "" {
		return false, nil, nil
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	now := time.Now()
	applied := false
	var affectedUsers []domain.User

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		dedup := domain.NodeTrafficBatch{
			NodeID:    nodeID,
			BatchID:   strings.TrimSpace(batch.BatchID),
			CreatedAt: now,
		}
		res := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "node_id"}, {Name: "batch_id"}},
			DoNothing: true,
		}).Create(&dedup)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			applied = false
			return nil
		}
		applied = true

		// 1. Merge and accumulate per-user deltas
		emails := make([]string, 0, len(batch.Users))
		userDeltas := make(map[string]domain.UserTrafficDelta, len(batch.Users))
		for _, d := range batch.Users {
			email := strings.TrimSpace(d.Email)
			up := maxZero64(d.UpBytes)
			down := maxZero64(d.DownBytes)
			if email == "" || (up == 0 && down == 0) {
				continue
			}
			if existing, ok := userDeltas[email]; ok {
				existing.Up += up
				existing.Down += down
				userDeltas[email] = existing
			} else {
				emails = append(emails, email)
				userDeltas[email] = domain.UserTrafficDelta{Email: email, Up: up, Down: down}
			}
		}

		for _, email := range emails {
			d := userDeltas[email]
			if err := tx.Model(&domain.User{}).
				Where("email = ?", d.Email).
				Updates(map[string]interface{}{
					"up_bytes":   gorm.Expr("up_bytes + ?", d.Up),
					"down_bytes": gorm.Expr("down_bytes + ?", d.Down),
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}

		if len(emails) > 0 {
			if err := tx.Where("email IN ?", emails).Find(&affectedUsers).Error; err != nil {
				return err
			}
			logsToUpsert := make([]domain.TrafficLog, 0, len(affectedUsers))
			for _, u := range affectedUsers {
				d, ok := userDeltas[u.Email]
				if !ok || (d.Up == 0 && d.Down == 0) {
					continue
				}
				logsToUpsert = append(logsToUpsert, domain.TrafficLog{
					UserID:    u.ID,
					UserEmail: u.Email,
					UpBytes:   d.Up,
					DownBytes: d.Down,
					Date:      date,
					CreatedAt: now,
				})
			}
			if len(logsToUpsert) > 0 {
				if err := tx.Clauses(trafficLogOnConflictClause()).CreateInBatches(logsToUpsert, 200).Error; err != nil {
					return err
				}
			}
		}

		// 2. Accumulate per-inbound deltas scoped strictly to nodeID
		var nodeXrayUp, nodeXrayDown int64
		for _, d := range batch.Inbounds {
			tag := strings.TrimSpace(d.Tag)
			up := maxZero64(d.UpBytes)
			down := maxZero64(d.DownBytes)
			if tag == "" || (up == 0 && down == 0) {
				continue
			}
			ibRes := tx.Model(&domain.Inbound{}).
				Where("tag = ? AND node_id = ?", tag, nodeID).
				Updates(map[string]interface{}{
					"up_bytes":   gorm.Expr("up_bytes + ?", up),
					"down_bytes": gorm.Expr("down_bytes + ?", down),
					"updated_at": now,
				})
			if ibRes.Error != nil {
				return ibRes.Error
			}
			if ibRes.RowsAffected > 0 {
				nodeXrayUp += up
				nodeXrayDown += down
			}
		}

		// 3. Accumulate Node Xray totals, cycle totals, and daily log
		if nodeXrayUp > 0 || nodeXrayDown > 0 {
			if err := tx.Model(&domain.Node{}).
				Where("id = ?", nodeID).
				Updates(map[string]interface{}{
					"xray_up_bytes":         gorm.Expr("xray_up_bytes + ?", nodeXrayUp),
					"xray_down_bytes":       gorm.Expr("xray_down_bytes + ?", nodeXrayDown),
					"cycle_xray_up_bytes":   gorm.Expr("cycle_xray_up_bytes + ?", nodeXrayUp),
					"cycle_xray_down_bytes": gorm.Expr("cycle_xray_down_bytes + ?", nodeXrayDown),
					"updated_at":            now,
				}).Error; err != nil {
				return err
			}

			daily := domain.NodeTrafficDaily{
				NodeID:        nodeID,
				Date:          date,
				XrayUpBytes:   nodeXrayUp,
				XrayDownBytes: nodeXrayDown,
				UpdatedAt:     now,
			}
			if err := tx.Clauses(nodeTrafficDailyOnConflictClause(map[string]interface{}{
				"xray_up_bytes":   gorm.Expr("node_traffic_dailies.xray_up_bytes + excluded.xray_up_bytes"),
				"xray_down_bytes": gorm.Expr("node_traffic_dailies.xray_down_bytes + excluded.xray_down_bytes"),
				"updated_at":      now,
			})).Create(&daily).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return applied, affectedUsers, nil
}

// UpdateAutoState updates desired_state and auto_state_reason for a node.
func (r *GORMNodeRepository) UpdateAutoState(ctx context.Context, nodeID uint, desiredState, reason string) error {
	res := r.db.WithContext(ctx).Model(&domain.Node{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"desired_state":     desiredState,
			"auto_state_reason": reason,
			"updated_at":        time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ResetCycleTraffic atomically zeros cycle counters and optionally sets last_reset_month and restores active state.
func (r *GORMNodeRepository) ResetCycleTraffic(ctx context.Context, nodeID uint, resetMonth int, restoreActive bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var node domain.Node
		if err := tx.First(&node, nodeID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}

		now := time.Now()
		updates := map[string]interface{}{
			"cycle_xray_up_bytes":   0,
			"cycle_xray_down_bytes": 0,
			"cycle_nic_up_bytes":    0,
			"cycle_nic_down_bytes":  0,
			"updated_at":            now,
		}
		if resetMonth > 0 {
			updates["last_reset_month"] = resetMonth
		}
		if restoreActive && (domain.IsAutoQuotaStateReason(node.AutoStateReason) || (node.DesiredState == domain.NodeStateActive && node.AutoStateReason == domain.AutoReasonManual)) {
			updates["desired_state"] = domain.NodeStateActive
			updates["auto_state_reason"] = ""
		}

		return tx.Model(&domain.Node{}).Where("id = ?", nodeID).Updates(updates).Error
	})
}

// ListRecentDailyTraffic returns up to days most recent daily traffic records for a node, newest first.
func (r *GORMNodeRepository) ListRecentDailyTraffic(ctx context.Context, nodeID uint, days int) ([]domain.NodeTrafficDaily, error) {
	if days <= 0 {
		days = 7
	}
	var logs []domain.NodeTrafficDaily
	err := r.db.WithContext(ctx).
		Where("node_id = ?", nodeID).
		Order("date DESC").
		Limit(days).
		Find(&logs).Error
	return logs, err
}

func (r *GORMNodeRepository) PruneBatchesBefore(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&domain.NodeTrafficBatch{}).Error
}

// UpdateNodeConfig atomically persists per-node outbounds, routing, and DNS JSON blobs using a strict column whitelist.
func (r *GORMNodeRepository) UpdateNodeConfig(ctx context.Context, nodeID uint, outboundsJSON, routingJSON, dnsJSON string) error {
	res := r.db.WithContext(ctx).Model(&domain.Node{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"outbounds_json": outboundsJSON,
			"routing_json":   routingJSON,
			"dns_json":       dnsJSON,
			"updated_at":     time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func maxZero64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
