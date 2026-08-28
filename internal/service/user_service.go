package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"panel/internal/domain"

	"github.com/google/uuid"
)

type UserService struct {
	userRepo    domain.UserRepository
	inboundRepo domain.InboundRepository
	xrayManager domain.XrayManager
	configSvc   *ConfigService
}

func NewUserService(
	userRepo domain.UserRepository,
	inboundRepo domain.InboundRepository,
	xrayManager domain.XrayManager,
	configSvc *ConfigService,
) *UserService {
	return &UserService{
		userRepo:    userRepo,
		inboundRepo: inboundRepo,
		xrayManager: xrayManager,
		configSvc:   configSvc,
	}
}

type CreateUserDTO struct {
	Email      string `json:"email"`
	InboundTag string `json:"inboundTag"`
	Flow       string `json:"flow"`
	TotalBytes int64  `json:"totalBytes"`
	ExpireDays int    `json:"expireDays"`
}

func (s *UserService) CreateUser(ctx context.Context, dto CreateUserDTO) (*domain.User, error) {
	if dto.Email == "" || dto.InboundTag == "" {
		return nil, fmt.Errorf("%w: email and inboundTag are required", domain.ErrInvalidInput)
	}

	// 检查 Inbound 是否存在
	inbound, err := s.inboundRepo.GetByTag(ctx, dto.InboundTag)
	if err != nil {
		return nil, fmt.Errorf("inbound not found: %w", err)
	}

	// 生成安全 Token 与 UUID
	tokenBytes := make([]byte, 16)
	_, _ = rand.Read(tokenBytes)
	subToken := hex.EncodeToString(tokenBytes)

	userUUID := uuid.New().String()

	var expireTime int64 = 0
	if dto.ExpireDays > 0 {
		expireTime = time.Now().AddDate(0, 0, dto.ExpireDays).UnixMilli()
	}

	user := &domain.User{
		UUID:       userUUID,
		Email:      dto.Email,
		InboundTag: inbound.Tag,
		Flow:       dto.Flow,
		SubToken:   subToken,
		TotalBytes: dto.TotalBytes,
		ExpireTime: expireTime,
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("create user in db failed: %w", err)
	}

	// 1. gRPC 热同步到 Xray 核心内存
	_ = s.xrayManager.AddUser(ctx, inbound.Tag, user)

	// 2. 双向同步持久化回写 config.json 物理文件
	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, inbound.Tag, user, false)
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, user *domain.User) error {
	oldUser, err := s.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		return err
	}

	// 如果状态发生改变，同步 Xray
	if oldUser.Enabled != user.Enabled || oldUser.Flow != user.Flow {
		_ = s.xrayManager.RemoveUser(ctx, oldUser.InboundTag, oldUser.Email)
		if user.IsActive() {
			_ = s.xrayManager.AddUser(ctx, user.InboundTag, user)
		}
	}

	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, user.InboundTag, user, false)
	}

	return s.userRepo.Update(ctx, user)
}

func (s *UserService) DeleteUser(ctx context.Context, id uint) error {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// 1. gRPC 热移除
	_ = s.xrayManager.RemoveUser(ctx, user.InboundTag, user.Email)

	// 2. 从物理 config.json 移除
	if s.configSvc != nil {
		_ = s.configSvc.SyncUserToFile(ctx, user.InboundTag, user, true)
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
