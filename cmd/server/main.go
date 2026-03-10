package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/crm/internal/config"
	"github.com/yourorg/crm/internal/platform"
	"github.com/yourorg/crm/internal/router"
	"go.uber.org/zap"
)

// @title           CRM API
// @version         1.0
// @description     Full-featured CRM backend built with Go + Gin
// @termsOfService  https://yourcrm.com/terms

// @contact.name   API Support
// @contact.email  support@yourcrm.com

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg := config.Load()

	db     := platform.NewPostgres(cfg.DatabaseURL)
	rdb    := platform.NewRedis(cfg.RedisURL)
	logger := platform.NewLogger(cfg.Env)
	defer logger.Sync()

	r := router.New(cfg, db, rdb, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		logger.Info("CRM server starting", zap.String("addr", srv.Addr), zap.String("env", cfg.Env))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gracefully...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
	logger.Info("server stopped")
}
