package main

import (
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/crm/internal/config"
	"github.com/yourorg/crm/internal/platform"
	"github.com/yourorg/crm/internal/repository"
	"github.com/yourorg/crm/internal/router"
	"github.com/yourorg/crm/internal/worker"
	"github.com/yourorg/crm/pkg/mailer"
	"go.uber.org/zap"
)


func main() {
	cfg := config.Load()

	db := platform.NewPostgres(cfg.DatabaseURL)
	rdb := platform.NewRedis(cfg.RedisURL)
	logger := platform.NewLogger(cfg.Env)
	defer logger.Sync()

	
	contactRepo := repository.NewContactRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	webhookRepo := repository.NewWebhookRepository(db)

	
	workerClient := worker.NewClient(cfg.RedisURL)
	defer workerClient.Close()

	
	m := mailer.New(cfg.MailjetAPIKeyPublic, cfg.MailjetAPIKeyPrivate, cfg.MailFromEmail, cfg.MailFromName)
	workers := worker.NewWorkers(
		cfg.RedisURL, db, m,
		contactRepo, taskRepo, notifRepo, webhookRepo,
		workerClient,
		cfg.MailFromEmail, cfg.MailFromName,
		logger,
	)
	if err := workers.Start(); err != nil {
		logger.Fatal("failed to start workers", zap.Error(err))
	}
	defer workers.Stop()

	r := router.New(cfg, db, rdb, workerClient, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	
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
