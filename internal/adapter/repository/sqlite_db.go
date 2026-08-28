package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"panel/internal/domain"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func InitSQLite(dbPath string) (*gorm.DB, error) {
	if dbPath == "" {
		dbPath = "data/panel.db"
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir failed: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite db failed: %w", err)
	}

	// 自动迁移数据表
	err = db.AutoMigrate(
		&domain.User{},
		&domain.Inbound{},
		&domain.TrafficLog{},
		&domain.ConfigSnapshot{},
		&domain.Setting{},
		&domain.AdminUser{},
	)
	if err != nil {
		return nil, fmt.Errorf("db automigrate failed: %w", err)
	}

	// 初始化默认管理员 (admin / admin123)
	var count int64
	db.Model(&domain.AdminUser{}).Count(&count)
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		defaultAdmin := &domain.AdminUser{
			Username:     "admin",
			PasswordHash: string(hash),
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		db.Create(defaultAdmin)
	}

	return db, nil
}
