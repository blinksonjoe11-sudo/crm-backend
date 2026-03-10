package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/yourorg/crm/pkg/mailer"
)

type EmailWorker struct {
	mailer *mailer.Mailer
}

func NewEmailWorker(m *mailer.Mailer) *EmailWorker {
	return &EmailWorker{mailer: m}
}

func (w *EmailWorker) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var p EmailPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("email worker: failed to unmarshal payload: %w", err)
	}

	if p.To == "" {
		return fmt.Errorf("email worker: missing recipient address")
	}

	err := w.mailer.Send(mailer.Mail{
		To:      p.To,
		ToName:  p.ToName,
		Subject: p.Subject,
		Body:    p.Body,
	})
	if err != nil {
		return fmt.Errorf("email worker: send failed to %s: %w", p.To, err)
	}

	return nil
}