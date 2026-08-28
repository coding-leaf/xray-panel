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

func (s *UserService) UpdateUser(ctx context.Context, user *domain.User) error {
	oldUser, err := s.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		return err
	}

	oldTags := oldUser.GetInboundTagList()
	newTags := user.GetInboundTagList()

	// 找出需要移除的节点
	newTagSet := make(map[string]bool)
	for _, t := range newTags {
		newTagSet[t] = true
	}
	for _, oldT := range oldTags {
		if !newTagSet[oldT] {
			_ = s.xrayManager.RemoveUser(ctx, oldT, oldUser.Email)
		}
	}

	// 注入/刷新新节点
	if user.IsActive() {
		for _, newT := range newTags {
			_ = s.xrayManager.AddUser(ctx, newT, user)
		}
	} else {
		for _, newT := range newTags {
			_ = s.xrayManager.RemoveUser(ctx, newT, user.Email)
		}
	}

	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, newTags, user, false)
	}

	return s.userRepo.Update(ctx, user)
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
	return s.userRepo.GetByID(ctx, id)
}

func (s *UserService) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.userRepo.ListAll(ctx)
}

func (s *UserService) ResetTraffic(ctx context.Context, id uint) error {
	return s.userRepo.ResetTraffic(ctx, id)
}

func (s *UserService) GetTrafficHistory(ctx context.Context, userID uint, days int) ([]domain.TrafficLog, error) {
	if s.trafficLogRepo == nil {
		return []domain.TrafficLog{}, nil
	}
	return s.trafficLogRepo.GetHistoryByUserID(ctx, userID, days)
}
