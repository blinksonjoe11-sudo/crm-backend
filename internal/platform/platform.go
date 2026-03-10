package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yourorg/crm/internal/domain"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func NewPostgres(dsn string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		panic(fmt.Sprintf("platform: postgres connect failed: %v", err))
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err := autoMigrate(db); err != nil {
		panic(fmt.Sprintf("platform: migration failed: %v", err))
	}
	return db
}

func autoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&domain.Workspace{},
		&domain.User{},
		&domain.RefreshToken{},
		&domain.Contact{},
		&domain.Company{},
		&domain.Pipeline{},
		&domain.Deal{},
		&domain.LineItem{},
		&domain.Activity{},
		&domain.Task{},
		&domain.EmailTemplate{},
		&domain.Workflow{},
		&domain.CustomField{},
		&domain.Webhook{},
		&domain.WebhookDelivery{},
		&domain.AuditLog{},
		&domain.Notification{},
		&domain.Team{},
	)
}

func NewRedis(url string) *redis.Client {
	opts, err := redis.ParseURL(url)
	if err != nil {
		panic(fmt.Sprintf("platform: invalid redis URL: %v", err))
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(fmt.Sprintf("platform: redis ping failed: %v", err))
	}
	return rdb
}

func NewLogger(env string) *zap.Logger {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return l
}
