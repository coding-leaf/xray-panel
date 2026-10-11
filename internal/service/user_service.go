package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"panel/internal/domain"

	"github.com/google/uuid"
)

type CreateUserDTO struct {
	Email       string   `json:"email" binding:"required"`
	InboundTag  string   `json:"inboundTag"`
	InboundTags []string `json:"inboundTags"`
	Flow        string   `json:"flow"`
	TotalBytes  int64    `json:"totalBytes"`
	ExpireDays  int      `json:"expireDays"`
	ResetDay    int      `json:"resetDay"`
	IPLimit     int      `json:"ipLimit"`
}

type XrayUserManager interface {
	AddUser(ctx context.Context, inboundTag string, user *domain.User) error
	RemoveUser(ctx context.Context, inboundTag string, email string) error
}

type ConfigFileSynchronizer interface {
	SyncUserToFile(ctx context.Context, authorizedTags []string, user *domain.User, isDelete bool) error
	SaveConfigQuietly(ctx context.Context, remark string) error
}

type UserService struct {
	userRepo       domain.UserRepository
	inboundRepo    domain.InboundRepository
	trafficLogRepo domain.TrafficLogRepository
	xrayManager    XrayUserManager
	configSvc      ConfigFileSynchronizer
}

func NewUserService(
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	trafficLogRepo domain.TrafficLogRepository,
	xrayManager XrayUserManager,
	configSvc ConfigFileSynchronizer,
) *UserService {
	return &UserService{
		userRepo:       userRepo,
		inboundRepo:    inboundRepo,
		trafficLogRepo: trafficLogRepo,
		xrayManager:    xrayManager,
		configSvc:      configSvc,
	}
}

func isXrayUserNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "not exist")
}

func createCompensatingContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}

func (s *UserService) CreateUser(ctx context.Context, dto CreateUserDTO) (*domain.User, error) {
	existing, err := s.userRepo.GetByEmail(ctx, dto.Email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("check existing user failed: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: 用户名/邮箱已存在", domain.ErrAlreadyExists)
	}

	tags := dto.InboundTags
	if len(tags) == 0 && dto.InboundTag != "" {
		tags = []string{dto.InboundTag}
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("请至少选择一个归属的入站节点")
	}

	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate sub token failed: %w", err)
	}
	subToken := hex.EncodeToString(tokenBytes)

	userUUID := uuid.New().String()

	var expireTime int64 = 0
	if dto.ExpireDays > 0 {
		expireTime = time.Now().AddDate(0, 0, dto.ExpireDays).UnixMilli()
	}

	tagsStr := strings.Join(tags, ",")
	user := &domain.User{
		UUID:        userUUID,
		Email:       dto.Email,
		InboundTag:  tags[0],
		InboundTags: tagsStr,
		Flow:        dto.Flow,
		SubToken:    subToken,
		TotalBytes:  dto.TotalBytes,
		ExpireTime:  expireTime,
		ResetDay:    dto.ResetDay,
		IPLimit:     dto.IPLimit,
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user in db failed: %w", err)
	}

	// 1. gRPC 热同步到所有已选节点内存，维护 addedTags 用于 Saga 反向补偿
	addedTags := make([]string, 0, len(tags))
	if s.xrayManager != nil {
		for _, t := range tags {
			if err := s.xrayManager.AddUser(ctx, t, user); err != nil {
				// 触发 Saga 反向补偿：逆序撤销已注入节点并清理 DB 孤儿记录
				compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
				defer cancel()
				for i := len(addedTags) - 1; i >= 0; i-- {
					if rErr := s.xrayManager.RemoveUser(compCtx, addedTags[i], user.Email); rErr != nil && !isXrayUserNotFoundError(rErr) {
						slog.Warn("compensation: failed to remove user from xray inbound", slog.String("inbound", addedTags[i]), slog.String("email", user.Email), slog.Any("error", rErr))
					}
				}
				if dErr := s.userRepo.Delete(compCtx, user.ID); dErr != nil {
					slog.Error("compensation: failed to delete orphan user from database", slog.Uint64("user_id", uint64(user.ID)), slog.Any("error", dErr))
				}
				return nil, fmt.Errorf("add user to xray inbound %s failed: %w", t, err)
			}
			addedTags = append(addedTags, t)
		}
	}

	// 2. 双向同步持久化回写 config.json 物理文件
	if s.configSvc != nil {
		if err := s.configSvc.SyncUserToFile(ctx, tags, user, false); err != nil {
			// 触发 Saga 反向补偿：清理全部已加入节点与 DB 孤儿记录
			compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
			defer cancel()
			if s.xrayManager != nil {
				for i := len(addedTags) - 1; i >= 0; i-- {
					if rErr := s.xrayManager.RemoveUser(compCtx, addedTags[i], user.Email); rErr != nil && !isXrayUserNotFoundError(rErr) {
						slog.Warn("compensation: failed to remove user from xray inbound", slog.String("inbound", addedTags[i]), slog.String("email", user.Email), slog.Any("error", rErr))
					}
				}
			}
			if dErr := s.userRepo.Delete(compCtx, user.ID); dErr != nil {
				slog.Error("compensation: failed to delete orphan user from database", slog.Uint64("user_id", uint64(user.ID)), slog.Any("error", dErr))
			}
			return nil, fmt.Errorf("sync user to config file failed: %w", err)
		}
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id uint, dto domain.UpdateUserDTO) (*domain.User, error) {
	oldUser, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if oldUser == nil {
		return nil, domain.ErrNotFound
	}

	// 制作前置状态深拷贝快照，用于故障时触发 Saga 反向补偿
	oldSnapshot := *oldUser
	oldTags := oldSnapshot.GetInboundTagList()

	// 仅合并允许修改的业务字段，严格保留 UpBytes、DownBytes、UUID、SubToken、CreatedAt
	updatedUser := *oldUser
	tags := dto.InboundTags
	if len(tags) == 0 && dto.InboundTag != "" {
		tags = []string{dto.InboundTag}
	}
	if len(tags) > 0 {
		updatedUser.InboundTags = strings.Join(tags, ",")
		if dto.InboundTag != "" {
			updatedUser.InboundTag = dto.InboundTag
		} else {
			updatedUser.InboundTag = tags[0]
		}
	}
	updatedUser.Flow = dto.Flow
	updatedUser.TotalBytes = dto.TotalBytes
	updatedUser.ExpireTime = dto.ExpireTime
	updatedUser.ResetDay = dto.ResetDay
	updatedUser.IPLimit = dto.IPLimit
	if dto.Enabled != nil {
		updatedUser.Enabled = *dto.Enabled
	}
	updatedUser.UpdatedAt = time.Now()

	newTags := updatedUser.GetInboundTagList()

	// 1. 更新数据库
	if err := s.userRepo.Update(ctx, &updatedUser); err != nil {
		return nil, fmt.Errorf("update user in db failed: %w", err)
	}

	// 定义 Saga 补偿逻辑：恢复 Xray 内存至旧快照并回滚数据库
	rollback := func() {
		compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
		defer cancel()
		if s.xrayManager != nil {
			for _, t := range newTags {
				if err := s.xrayManager.RemoveUser(compCtx, t, updatedUser.Email); err != nil && !isXrayUserNotFoundError(err) {
					slog.Warn("compensation: failed to remove user from new inbound during update rollback", slog.String("inbound", t), slog.String("email", updatedUser.Email), slog.Any("error", err))
				}
			}
			if oldSnapshot.IsActive() {
				for _, t := range oldTags {
					if err := s.xrayManager.RemoveUser(compCtx, t, oldSnapshot.Email); err != nil && !isXrayUserNotFoundError(err) {
						slog.Warn("compensation: failed to remove user before restoring snapshot", slog.String("inbound", t), slog.String("email", oldSnapshot.Email), slog.Any("error", err))
					}
					if err := s.xrayManager.AddUser(compCtx, t, &oldSnapshot); err != nil {
						slog.Error("compensation: failed to re-add old snapshot to xray", slog.String("inbound", t), slog.String("email", oldSnapshot.Email), slog.Any("error", err))
					}
				}
			}
		}
		if err := s.userRepo.Update(compCtx, &oldSnapshot); err != nil {
			slog.Error("compensation: failed to restore user snapshot in db during update rollback", slog.Uint64("user_id", uint64(oldSnapshot.ID)), slog.Any("error", err))
		}
	}

	// 2. 刷新 Xray 内存
	if s.xrayManager != nil {
		newTagSet := make(map[string]bool, len(newTags))
		for _, t := range newTags {
			newTagSet[t] = true
		}
		// 找出需要移除的旧节点
		for _, oldT := range oldTags {
			if !newTagSet[oldT] {
				if err := s.xrayManager.RemoveUser(ctx, oldT, updatedUser.Email); err != nil {
					rollback()
					return nil, fmt.Errorf("remove user from old xray inbound %s failed: %w", oldT, err)
				}
			}
		}

		if updatedUser.IsActive() {
			for _, newT := range newTags {
				// 针对现有节点先 RemoveUser 再 AddUser 以避免冲突
				if err := s.xrayManager.RemoveUser(ctx, newT, updatedUser.Email); err != nil && !isXrayUserNotFoundError(err) {
					rollback()
					return nil, fmt.Errorf("remove existing user before update from xray inbound %s failed: %w", newT, err)
				}
				if err := s.xrayManager.AddUser(ctx, newT, &updatedUser); err != nil {
					rollback()
					return nil, fmt.Errorf("add updated user to xray inbound %s failed: %w", newT, err)
				}
			}
		} else {
			for _, newT := range newTags {
				if err := s.xrayManager.RemoveUser(ctx, newT, updatedUser.Email); err != nil {
					rollback()
					return nil, fmt.Errorf("remove inactive user from xray inbound %s failed: %w", newT, err)
				}
			}
		}
	}

	// 3. 落盘持久化到物理文件
	if s.configSvc != nil {
		if err := s.configSvc.SyncUserToFile(ctx, newTags, &updatedUser, false); err != nil {
			rollback()
			return nil, fmt.Errorf("sync updated user to config file failed: %w", err)
		}
	}

	return &updatedUser, nil
}

func (s *UserService) DeleteUser(ctx context.Context, id uint) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if user == nil {
		return domain.ErrNotFound
	}

	oldSnapshot := *user
	tags := user.GetInboundTagList()

	// 1. gRPC 热移除该用户在所有节点的内存状态，记录 removedTags 用于失败回滚
	removedTags := make([]string, 0, len(tags))
	rollbackXray := func() {
		compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
		defer cancel()
		if s.xrayManager != nil && oldSnapshot.IsActive() {
			for _, t := range removedTags {
				if err := s.xrayManager.AddUser(compCtx, t, &oldSnapshot); err != nil {
					slog.Error("compensation: failed to re-add user to xray during delete rollback",
						slog.String("inbound", t), slog.String("email", oldSnapshot.Email), slog.Any("error", err))
				}
			}
		}
	}

	if s.xrayManager != nil {
		for _, t := range tags {
			if err := s.xrayManager.RemoveUser(ctx, t, user.Email); err != nil {
				if !isXrayUserNotFoundError(err) {
					rollbackXray()
					return fmt.Errorf("remove user from xray inbound %s failed: %w", t, err)
				}
			}
			removedTags = append(removedTags, t)
		}
	}

	// 2. 从数据库删除用户记录（保留内存快照用于写盘失败补偿）
	if err := s.userRepo.Delete(ctx, id); err != nil {
		rollbackXray()
		return fmt.Errorf("delete user from db failed: %w", err)
	}

	// 3. 从物理 config.json 移除写盘（此时 DB 中已无该用户，重新编译出的配置必然干净）
	if s.configSvc != nil {
		if err := s.configSvc.SyncUserToFile(ctx, nil, user, true); err != nil {
			// 触发 Saga 完整反向补偿：恢复数据库记录，并恢复 Xray 内存节点
			compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
			defer cancel()
			if cErr := s.userRepo.Create(compCtx, &oldSnapshot); cErr != nil {
				slog.Error("compensation: failed to restore user in db during delete rollback",
					slog.Uint64("user_id", uint64(oldSnapshot.ID)), slog.String("email", oldSnapshot.Email), slog.Any("error", cErr))
			}
			rollbackXray()
			return fmt.Errorf("sync delete user to config file failed: %w", err)
		}
	}

	// 4. 清理内存运行时速率追踪器，杜绝幽灵对象内存泄漏
	domain.RemoveUserRuntimeSpeed(user.Email)

	return nil
}

func (s *UserService) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil || u == nil {
		return u, err
	}
	u.UpSpeed, u.DownSpeed, u.LastActive, u.IsOnline = domain.GetUserRuntimeSpeed(u.Email)
	return u, nil
}

func (s *UserService) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil || u == nil {
		return u, err
	}
	u.UpSpeed, u.DownSpeed, u.LastActive, u.IsOnline = domain.GetUserRuntimeSpeed(u.Email)
	return u, nil
}

func (s *UserService) ListUsers(ctx context.Context) ([]domain.User, error) {
	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].UpSpeed, users[i].DownSpeed, users[i].LastActive, users[i].IsOnline = domain.GetUserRuntimeSpeed(users[i].Email)
	}
	return users, nil
}

func (s *UserService) ResetTraffic(ctx context.Context, id uint) error {
	if err := s.userRepo.ResetTraffic(ctx, id); err != nil {
		return fmt.Errorf("reset user traffic in db failed: %w", err)
	}
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get user after reset traffic failed: %w", err)
	}
	if user == nil {
		return domain.ErrNotFound
	}
	domain.RecordUserTrafficReset(user.Email)
	if user.IsActive() {
		if s.xrayManager != nil {
			for _, t := range user.GetInboundTagList() {
				if err := s.xrayManager.AddUser(ctx, t, user); err != nil {
					return fmt.Errorf("add user to xray inbound %s on reset traffic failed: %w", t, err)
				}
			}
		}
		if s.configSvc != nil {
			if err := s.configSvc.SyncUserToFile(ctx, user.GetInboundTagList(), user, false); err != nil {
				return fmt.Errorf("sync user to config file on reset traffic failed: %w", err)
			}
		}
	}
	return nil
}

func (s *UserService) ResetSubToken(ctx context.Context, id uint) (*domain.User, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, domain.ErrNotFound
	}

	oldSnapshot := *user
	oldUUID := user.UUID
	oldToken := user.SubToken
	inboundTags := user.GetInboundTagList()

	// 1. 生成全新 SubToken 与全新高熵 UUID
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate new sub token failed: %w", err)
	}
	newToken := hex.EncodeToString(tokenBytes)
	newUUID := uuid.New().String()

	user.SubToken = newToken
	user.UUID = newUUID
	user.UpdatedAt = time.Now()

	// 2. 更新数据库
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update reset sub token in db failed: %w", err)
	}

	// 定义 Saga 补偿逻辑：恢复原凭证到数据库与 Xray 挂载
	rollback := func() {
		compCtx, cancel := createCompensatingContext(ctx, 15*time.Second)
		defer cancel()
		user.SubToken = oldToken
		user.UUID = oldUUID
		if err := s.userRepo.Update(compCtx, user); err != nil {
			slog.Error("compensation: failed to restore original credentials in db", slog.Uint64("user_id", uint64(user.ID)), slog.Any("error", err))
		}

		if s.xrayManager != nil && oldSnapshot.IsActive() {
			for _, t := range inboundTags {
				if err := s.xrayManager.RemoveUser(compCtx, t, user.Email); err != nil && !isXrayUserNotFoundError(err) {
					slog.Warn("compensation: failed to remove user before restoring credentials in xray", slog.String("inbound", t), slog.String("email", user.Email), slog.Any("error", err))
				}
				if err := s.xrayManager.AddUser(compCtx, t, &oldSnapshot); err != nil {
					slog.Error("compensation: failed to re-add user with original credentials in xray", slog.String("inbound", t), slog.String("email", oldSnapshot.Email), slog.Any("error", err))
				}
			}
		}
	}

	// 3. gRPC 热注册新 UUID：先移除旧凭证再注入新凭证
	if s.xrayManager != nil && user.IsActive() {
		for _, t := range inboundTags {
			if err := s.xrayManager.RemoveUser(ctx, t, user.Email); err != nil && !isXrayUserNotFoundError(err) {
				rollback()
				return nil, fmt.Errorf("remove old user credentials from xray inbound %s failed: %w", t, err)
			}
			if err := s.xrayManager.AddUser(ctx, t, user); err != nil {
				rollback()
				return nil, fmt.Errorf("register reset token in xray inbound %s failed: %w", t, err)
			}
		}
	}

	// 4. 持久化回写 config.json 物理文件
	if s.configSvc != nil {
		if err := s.configSvc.SyncUserToFile(ctx, inboundTags, user, false); err != nil {
			rollback()
			return nil, fmt.Errorf("sync reset token to config file failed: %w", err)
		}
	}

	return user, nil
}

func (s *UserService) BatchRenew(ctx context.Context, ids []uint, addDays int) error {
	if addDays == 0 {
		return nil
	}
	var errs []error
	for _, id := range ids {
		u, err := s.userRepo.GetByID(ctx, id)
		if err != nil || u == nil {
			if err == nil {
				err = domain.ErrNotFound
			}
			errs = append(errs, fmt.Errorf("renew user %d get failed: %w", id, err))
			continue
		}
		now := time.Now().UnixMilli()
		var newExpireTime int64
		if u.ExpireTime <= 0 || u.ExpireTime < now {
			newExpireTime = time.Now().AddDate(0, 0, addDays).UnixMilli()
		} else {
			newExpireTime = time.UnixMilli(u.ExpireTime).AddDate(0, 0, addDays).UnixMilli()
		}
		dto := domain.UpdateUserDTO{
			InboundTags: u.GetInboundTagList(),
			InboundTag:  u.InboundTag,
			Flow:        u.Flow,
			TotalBytes:  u.TotalBytes,
			ExpireTime:  newExpireTime,
			ResetDay:    u.ResetDay,
			IPLimit:     u.IPLimit,
			Enabled:     &u.Enabled,
		}
		if _, err := s.UpdateUser(ctx, id, dto); err != nil {
			errs = append(errs, fmt.Errorf("renew user %d update failed: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *UserService) BatchResetTraffic(ctx context.Context, ids []uint) error {
	var errs []error
	for _, id := range ids {
		if err := s.ResetTraffic(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("reset traffic user %d failed: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *UserService) BatchSetStatus(ctx context.Context, ids []uint, enabled bool) error {
	var errs []error
	for _, id := range ids {
		u, err := s.userRepo.GetByID(ctx, id)
		if err != nil || u == nil {
			if err == nil {
				err = domain.ErrNotFound
			}
			errs = append(errs, fmt.Errorf("set status user %d get failed: %w", id, err))
			continue
		}
		dto := domain.UpdateUserDTO{
			InboundTags: u.GetInboundTagList(),
			InboundTag:  u.InboundTag,
			Flow:        u.Flow,
			TotalBytes:  u.TotalBytes,
			ExpireTime:  u.ExpireTime,
			ResetDay:    u.ResetDay,
			IPLimit:     u.IPLimit,
			Enabled:     &enabled,
		}
		if _, err := s.UpdateUser(ctx, id, dto); err != nil {
			errs = append(errs, fmt.Errorf("set status user %d update failed: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func (s *UserService) CheckAndResetMonthlyTraffic(ctx context.Context) error {
	now := time.Now()
	today := now.Day()
	currentYearMonth := now.Year()*100 + int(now.Month())
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()

	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	var errs []error
	anyActiveReset := false
	for _, u := range users {
		if u.ResetDay <= 0 {
			continue
		}

		// 自适应当月最大天数（如平年 2 月为 28，小月为 30，设为 31 号的用户将在该月最后一天重置）
		effectiveResetDay := u.ResetDay
		if effectiveResetDay > lastDayOfMonth {
			effectiveResetDay = lastDayOfMonth
		}

		// 当今天已达到或越过有效重置日，且本月尚未重置时触发（具备短月份自适应与停机恢复补偿能力）
		if today >= effectiveResetDay && u.LastResetMonth != currentYearMonth {
			if err := s.userRepo.ResetTraffic(ctx, u.ID); err != nil {
				errs = append(errs, fmt.Errorf("reset user %d traffic failed: %w", u.ID, err))
				continue
			}
			domain.RecordUserTrafficReset(u.Email)
			u.UpBytes = 0
			u.DownBytes = 0
			u.LastResetMonth = currentYearMonth
			if err := s.userRepo.Update(ctx, &u); err != nil {
				errs = append(errs, fmt.Errorf("update user %d reset month failed: %w", u.ID, err))
				continue
			}

			if u.IsActive() {
				if s.xrayManager != nil {
					for _, t := range u.GetInboundTagList() {
						if err := s.xrayManager.AddUser(ctx, t, &u); err != nil {
							errs = append(errs, fmt.Errorf("add user %d to xray %s failed: %w", u.ID, t, err))
						}
					}
				}
				anyActiveReset = true
			}
		}
	}
	if anyActiveReset && s.configSvc != nil {
		if err := s.configSvc.SaveConfigQuietly(ctx, "月度流量重置配置静默同步"); err != nil {
			errs = append(errs, fmt.Errorf("save config quietly failed: %w", err))
		}
	}
	return errors.Join(errs...)
}

func (s *UserService) GetTrafficHistory(ctx context.Context, userID uint, days int) ([]domain.TrafficLog, error) {
	if s.trafficLogRepo == nil {
		return []domain.TrafficLog{}, nil
	}
	return s.trafficLogRepo.GetHistoryByUserID(ctx, userID, days)
}
