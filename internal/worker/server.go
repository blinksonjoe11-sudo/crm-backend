package worker

import (
	"github.com/hibiken/asynq"
	"github.com/yourorg/crm/internal/repository"
	"github.com/yourorg/crm/pkg/mailer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Workers struct {
	server *asynq.Server
	mux    *asynq.ServeMux
	logger *zap.Logger
}

func NewWorkers(
	redisURL string,
	db *gorm.DB,
	m *mailer.Mailer,
	contactRepo  repository.ContactRepository,
	taskRepo     repository.TaskRepository,
	notifRepo    repository.NotificationRepository,
	webhookRepo  repository.WebhookRepository,
	workerClient *Client,
	fromEmail, fromName string,
	logger *zap.Logger,
) *Workers {
	srv := NewServer(redisURL)
	mux := asynq.NewServeMux()

	// Register all workers
	emailWorker    := NewEmailWorker(m)
	webhookWorker  := NewWebhookWorker(db, webhookRepo)
	importWorker   := NewImportWorker(contactRepo)
	workflowWorker := NewWorkflowWorker(db, taskRepo, notifRepo, workerClient, fromEmail, fromName)

	mux.HandleFunc(TypeEmailSend,       emailWorker.ProcessTask)
	mux.HandleFunc(TypeWebhookDeliver,  webhookWorker.ProcessTask)
	mux.HandleFunc(TypeContactImport,   importWorker.ProcessTask)
	mux.HandleFunc(TypeWorkflowTrigger, workflowWorker.ProcessTask)

	return &Workers{server: srv, mux: mux, logger: logger}
}

func (w *Workers) Start() error {
	w.logger.Info("starting async workers")
	return w.server.Start(w.mux)
}

func (w *Workers) Stop() {
	w.logger.Info("stopping async workers")
	w.server.Shutdown()
}