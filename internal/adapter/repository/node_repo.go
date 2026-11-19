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
)

func NewNodeRepository(db *gorm.DB) *GORMNodeRepository {
	return &GORMNodeRepository{db: db}
}

func (r *GORMNodeRepository) Create(ctx context.Context, node *domain.Node) error {
	return r.db.WithContext(ctx).Create(node).Error
}

// Update persists admin metadata and token credentials without overwriting heartbeat or traffic counters.
func (r *GORMNodeRepository) Update(ctx context.Context, node *domain.Node) error {
	res := r.db.WithContext(ctx).Model(&domain.Node{}).
		Where("id = ?", node.ID).
		Updates(map[string]interface{}{
			"name":          node.Name,
			"host":          node.Host,
			"region":        node.Region,
			"token_hash":    node.TokenHash,
			"token_prefix":  node.TokenPrefix,
			"enabled":       node.Enabled,
			"desired_state": node.DesiredState,
			"remark":        node.Remark,
			"updated_at":    node.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateHeartbeat atomically records agent heartbeat metadata and NIC traffic deltas.
func (r *GORMNodeRepository) UpdateHeartbeat(ctx context.Context, nodeID uint, req *domain.AgentSyncRequest, seenAt time.Time) error {
	updates := map[string]interface{}{
		"last_seen_at":     seenAt,
		"agent_version":    req.AgentVersion,
		"xray_version":     req.XrayVersion,
		"last_config_hash": req.ConfigHash,
		"last_users_hash":  req.UsersHash,
		"last_sync_error":  req.LastError,
		"updated_at":       seenAt,
	}
	if req.HostMetrics != nil {
		updates["cpu_percent"] = req.HostMetrics.CPUPercent
		updates["mem_percent"] = req.HostMetrics.MemPercent
		if req.HostMetrics.NicUpDelta > 0 {
			updates["nic_up_bytes"] = gorm.Expr("nic_up_bytes + ?", req.HostMetrics.NicUpDelta)
		}
		if req.HostMetrics.NicDownDelta > 0 {
			updates["nic_down_bytes"] = gorm.Expr("nic_down_bytes + ?", req.HostMetrics.NicDownDelta)
		}
	}
	res := r.db.WithContext(ctx).Model(&domain.Node{}).Where("id = ?", nodeID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
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

		// 3. Accumulate Node Xray totals
		if nodeXrayUp > 0 || nodeXrayDown > 0 {
			if err := tx.Model(&domain.Node{}).
				Where("id = ?", nodeID).
				Updates(map[string]interface{}{
					"xray_up_bytes":   gorm.Expr("xray_up_bytes + ?", nodeXrayUp),
					"xray_down_bytes": gorm.Expr("xray_down_bytes + ?", nodeXrayDown),
					"updated_at":      now,
				}).Error; err != nil {
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

func (r *GORMNodeRepository) PruneBatchesBefore(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&domain.NodeTrafficBatch{}).Error
}

func maxZero64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
