package config

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PostgreSQL struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	User     string `json:"user" yaml:"user"`
	Password string `json:"password" yaml:"password"`
	DBName   string `json:"db_name" yaml:"db_name"`
	SSLMode  string `json:"ssl_mode" yaml:"ssl_mode"`
}

// InitDB 初始化数据库连接 (使用环境变量配置，避免硬编码凭证)
func InitDB(cfg PostgreSQL) (*gorm.DB, error) {
	if cfg.Password == "" {
		return nil, errors.New("database password must be set via PG_PASSWORD environment variable")
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true, // 手动控制事务以提高性能
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}

	// 连接池安全配置
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(30 * time.Minute)

	// 自动迁移 (生产环境应使用显式迁移)
	//if err := db.AutoMigrate(&Account{}); err != nil {
	//	return nil, fmt.Errorf("failed to migrate database: %w", err)
	//}

	return db, nil
}
