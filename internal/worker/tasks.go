package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

//  Task type constants 

const (
	TypeEmailSend       = "email:send"
	TypeWebhookDeliver  = "webhook:deliver"
	TypeContactImport   = "contact:import"
	TypeWorkflowTrigger = "workflow:trigger"
)

//  Payloads 

type EmailPayload struct {
	To       string `json:"to"`
	ToName   string `json:"to_name"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

type WebhookPayload struct {
	WebhookID   string          `json:"webhook_id"`
	WorkspaceID string          `json:"workspace_id"`
	Event       string          `json:"event"`
	URL         string          `json:"url"`
	Secret      string          `json:"secret"`
	Payload     json.RawMessage `json:"payload"`
}

type ImportPayload struct {
	WorkspaceID string   `json:"workspace_id"`
	OwnerID     string   `json:"owner_id"`
	Rows        [][]string `json:"rows"`
}

type WorkflowPayload struct {
	WorkspaceID string          `json:"workspace_id"`
	WorkflowID  string          `json:"workflow_id"`
	Trigger     string          `json:"trigger"`
	EntityID    string          `json:"entity_id"`
	EntityType  string          `json:"entity_type"`
	Data        json.RawMessage `json:"data"`
}

//  Enqueue helpers 

func NewEmailTask(p EmailPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeEmailSend, b), nil
}

func NewWebhookTask(p WebhookPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeWebhookDeliver, b, asynq.MaxRetry(5)), nil
}

func NewImportTask(p ImportPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeContactImport, b, asynq.Queue("imports")), nil
}

func NewWorkflowTask(p WorkflowPayload) (*asynq.Task, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeWorkflowTrigger, b), nil
}

//  Client wrapper 

type Client struct {
	asynq *asynq.Client
}

func NewClient(redisURL string) *Client {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		panic(fmt.Sprintf("worker: invalid redis URL: %v", err))
	}
	return &Client{asynq: asynq.NewClient(opt)}
}

func (c *Client) Enqueue(task *asynq.Task, opts ...asynq.Option) error {
	_, err := c.asynq.Enqueue(task, opts...)
	return err
}

func (c *Client) EnqueueEmail(p EmailPayload) error {
	t, err := NewEmailTask(p)
	if err != nil {
		return err
	}
	return c.Enqueue(t)
}

func (c *Client) EnqueueWebhook(p WebhookPayload) error {
	t, err := NewWebhookTask(p)
	if err != nil {
		return err
	}
	return c.Enqueue(t)
}

func (c *Client) EnqueueImport(p ImportPayload) error {
	t, err := NewImportTask(p)
	if err != nil {
		return err
	}
	return c.Enqueue(t, asynq.Queue("imports"))
}

func (c *Client) EnqueueWorkflow(p WorkflowPayload) error {
	t, err := NewWorkflowTask(p)
	if err != nil {
		return err
	}
	return c.Enqueue(t)
}

func (c *Client) Close() error {
	return c.asynq.Close()
}

//  Server 

func NewServer(redisURL string) *asynq.Server {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		panic(fmt.Sprintf("worker: invalid redis URL: %v", err))
	}
	return asynq.NewServer(opt, asynq.Config{
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"imports":  1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			fmt.Printf("worker error: task=%s err=%v\n", task.Type(), err)
		}),
	})
}