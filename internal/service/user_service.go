package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"panel/internal/adapter/xray"
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

type UserService struct {
	userRepo       domain.UserRepository
	inboundRepo    domain.InboundRepository
	trafficLogRepo domain.TrafficLogRepository
	xrayManager    *xray.Manager
	configSvc      *ConfigService
}

func NewUserService(
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	trafficLogRepo domain.TrafficLogRepository,
	xrayManager *xray.Manager,
	configSvc *ConfigService,
) *UserService {
	return &UserService{
		userRepo:       userRepo,
		inboundRepo:    inboundRepo,
		trafficLogRepo: trafficLogRepo,
		xrayManager:    xrayManager,
		configSvc:      configSvc,
	}
}

func (s *UserService) CreateUser(ctx context.Context, dto CreateUserDTO) (*domain.User, error) {
	existing, _ := s.userRepo.GetByEmail(ctx, dto.Email)
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
	_, _ = rand.Read(tokenBytes)
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

	// 1. gRPC 热同步到所有已选节点内存
	for _, t := range tags {
		_ = s.xrayManager.AddUser(ctx, t, user)
	}

	// 2. 双向同步持久化回写 config.json 物理文件
	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, tags, user, false)
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id uint, dto domain.UpdateUserDTO) (*domain.User, error) {
	oldUser, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	oldTags := oldUser.GetInboundTagList()

	// 仅合并允许修改的业务字段，严格保留 UpBytes、DownBytes、UUID、SubToken、CreatedAt
	tags := dto.InboundTags
	if len(tags) == 0 && dto.InboundTag != "" {
		tags = []string{dto.InboundTag}
	}
	if len(tags) > 0 {
		oldUser.InboundTags = strings.Join(tags, ",")
		if dto.InboundTag != "" {
			oldUser.InboundTag = dto.InboundTag
		} else {
			oldUser.InboundTag = tags[0]
		}
	}
	oldUser.Flow = dto.Flow
	oldUser.TotalBytes = dto.TotalBytes
	oldUser.ExpireTime = dto.ExpireTime
	oldUser.ResetDay = dto.ResetDay
	oldUser.IPLimit = dto.IPLimit
	if dto.Enabled != nil {
		oldUser.Enabled = *dto.Enabled
	}
	oldUser.UpdatedAt = time.Now()

	newTags := oldUser.GetInboundTagList()

	// 找出需要移除的节点
	newTagSet := make(map[string]bool)
	for _, t := range newTags {
		newTagSet[t] = true
	}
	for _, oldT := range oldTags {
		if !newTagSet[oldT] {
			if s.xrayManager != nil {
				_ = s.xrayManager.RemoveUser(ctx, oldT, oldUser.Email)
			}
		}
	}

	// 注入/刷新新节点
	if oldUser.IsActive() {
		for _, newT := range newTags {
			if s.xrayManager != nil {
				_ = s.xrayManager.AddUser(ctx, newT, oldUser)
			}
		}
	} else {
		for _, newT := range newTags {
			if s.xrayManager != nil {
				_ = s.xrayManager.RemoveUser(ctx, newT, oldUser.Email)
			}
		}
	}

	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, newTags, oldUser, false)
	}

	if err := s.userRepo.Update(ctx, oldUser); err != nil {
		return nil, err
	}

	return oldUser, nil
}


func (s *UserService) DeleteUser(ctx context.Context, id uint) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 1. gRPC 热移除该用户在所有节点的内存状态
	for _, t := range user.GetInboundTagList() {
		_ = s.xrayManager.RemoveUser(ctx, t, user.Email)
	}

	// 2. 从物理 config.json 移除
	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, nil, user, true)
	}

	return s.userRepo.Delete(ctx, id)
}

func (s *UserService) GetByID(ctx context.Context, id uint) (*domain.User, error) {
	u, err := s.userRepo.GetByID(ctx, id)
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
	return s.userRepo.ResetTraffic(ctx, id)
}

func (s *UserService) ResetSubToken(ctx context.Context, id uint) (string, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	newToken := hex.EncodeToString(tokenBytes)
	user.SubToken = newToken
	if err := s.userRepo.Update(ctx, user); err != nil {
		return "", err
	}
	return newToken, nil
}

func (s *UserService) BatchRenew(ctx context.Context, ids []uint, addDays int) error {
	if addDays == 0 {
		return nil
	}
	for _, id := range ids {
		u, err := s.userRepo.GetByID(ctx, id)
		if err != nil {
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
		_, _ = s.UpdateUser(ctx, id, dto)
	}
	return nil
}

func (s *UserService) BatchResetTraffic(ctx context.Context, ids []uint) error {
	for _, id := range ids {
		_ = s.userRepo.ResetTraffic(ctx, id)
	}
	return nil
}

func (s *UserService) BatchSetStatus(ctx context.Context, ids []uint, enabled bool) error {
	for _, id := range ids {
		u, err := s.userRepo.GetByID(ctx, id)
		if err != nil {
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
		_, _ = s.UpdateUser(ctx, id, dto)
	}
	return nil
}

func (s *UserService) CheckAndResetMonthlyTraffic(ctx context.Context) error {
	now := time.Now()
	today := now.Day()
	currentYearMonth := now.Year()*100 + int(now.Month())

	users, err := s.userRepo.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.ResetDay > 0 && u.ResetDay == today && u.LastResetMonth != currentYearMonth {
			_ = s.userRepo.ResetTraffic(ctx, u.ID)
			u.UpBytes = 0
			u.DownBytes = 0
			u.LastResetMonth = currentYearMonth
			_ = s.userRepo.Update(ctx, &u)
		}
	}
	return nil
}

func (s *UserService) GetTrafficHistory(ctx context.Context, userID uint, days int) ([]domain.TrafficLog, error) {
	if s.trafficLogRepo == nil {
		return []domain.TrafficLog{}, nil
	}
	return s.trafficLogRepo.GetHistoryByUserID(ctx, userID, days)
}
