package worker

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/repository"
	"gorm.io/gorm"
)

type WebhookWorker struct {
	db         *gorm.DB
	webhookRepo repository.WebhookRepository
	httpClient  *http.Client
}

func NewWebhookWorker(db *gorm.DB, webhookRepo repository.WebhookRepository) *WebhookWorker {
	return &WebhookWorker{
		db:          db,
		webhookRepo: webhookRepo,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (w *WebhookWorker) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var p WebhookPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("webhook worker: failed to unmarshal payload: %w", err)
	}

	delivery := &domain.WebhookDelivery{
		ID:        uuid.New(),
		WebhookID: uuid.MustParse(p.WebhookID),
		Event:     p.Event,
		Payload:   domain.JSONB{"data": json.RawMessage(p.Payload)},
		Attempts:  1,
	}

	statusCode, response, err := w.deliver(p)

	delivery.StatusCode = statusCode
	delivery.Response   = response
	now := time.Now()

	if err != nil {
		delivery.StatusCode = 0
		delivery.Response   = err.Error()
		_ = w.webhookRepo.SaveDelivery(ctx, delivery)
		return fmt.Errorf("webhook worker: delivery failed to %s: %w", p.URL, err)
	}

	if statusCode < 200 || statusCode > 299 {
		_ = w.webhookRepo.SaveDelivery(ctx, delivery)
		return fmt.Errorf("webhook worker: non-2xx response %d from %s", statusCode, p.URL)
	}

	delivery.DeliveredAt = &now
	_ = w.webhookRepo.SaveDelivery(ctx, delivery)
	return nil
}

func (w *WebhookWorker) deliver(p WebhookPayload) (int, string, error) {
	body, err := json.Marshal(map[string]interface{}{
		"event":        p.Event,
		"workspace_id": p.WorkspaceID,
		"data":         json.RawMessage(p.Payload),
		"delivered_at": time.Now().UTC(),
	})
	if err != nil {
		return 0, "", err
	}

	req, err := http.NewRequest(http.MethodPost, p.URL, bytes.NewBuffer(body))
	if err != nil {
		return 0, "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CRM-Webhook/1.0")

	// HMAC-SHA256 signature so receiver can verify authenticity
	if p.Secret != "" {
		mac := hmac.New(sha256.New, []byte(p.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-CRM-Signature", "sha256="+sig)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(respBody), nil
}

// FireEvent is called from services to enqueue a webhook delivery for all
// registered webhooks in the workspace that subscribe to the given event.
func FireEvent(ctx context.Context, client *Client, webhookRepo repository.WebhookRepository, wsID uuid.UUID, event string, data interface{}) {
	hooks, err := webhookRepo.FindByEvent(ctx, wsID, event)
	if err != nil || len(hooks) == 0 {
		return
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return
	}

	for _, hook := range hooks {
		_ = client.EnqueueWebhook(WebhookPayload{
			WebhookID:   hook.ID.String(),
			WorkspaceID: wsID.String(),
			Event:       event,
			URL:         hook.URL,
			Secret:      hook.Secret,
			Payload:     payload,
		})
	}
}