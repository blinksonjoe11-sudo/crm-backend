package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/repository"
	"gorm.io/gorm"
)

type WorkflowWorker struct {
	db           *gorm.DB
	taskRepo     repository.TaskRepository
	notifRepo    repository.NotificationRepository
	emailClient  *Client
	mailerConfig struct {
		fromEmail string
		fromName  string
	}
}

func NewWorkflowWorker(
	db *gorm.DB,
	taskRepo repository.TaskRepository,
	notifRepo repository.NotificationRepository,
	workerClient *Client,
	fromEmail, fromName string,
) *WorkflowWorker {
	w := &WorkflowWorker{
		db:          db,
		taskRepo:    taskRepo,
		notifRepo:   notifRepo,
		emailClient: workerClient,
	}
	w.mailerConfig.fromEmail = fromEmail
	w.mailerConfig.fromName = fromName
	return w
}

func (w *WorkflowWorker) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var p WorkflowPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("workflow worker: failed to unmarshal payload: %w", err)
	}

	wsID, err := uuid.Parse(p.WorkspaceID)
	if err != nil {
		return fmt.Errorf("workflow worker: invalid workspace_id: %w", err)
	}

	var workflow domain.Workflow
	if err := w.db.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND is_active = true", p.WorkflowID, wsID).
		First(&workflow).Error; err != nil {
		return fmt.Errorf("workflow worker: workflow not found or inactive: %w", err)
	}

	var entityData map[string]interface{}
	if len(p.Data) > 0 {
		_ = json.Unmarshal(p.Data, &entityData)
	}

	if !w.evaluateConditions(json.RawMessage(workflow.Conditions), entityData) {
		fmt.Printf("workflow worker: conditions not met for workflow %s\n", workflow.ID)
		return nil
	}

	if err := w.executeActions(ctx, wsID, workflow, p, entityData); err != nil {
		return fmt.Errorf("workflow worker: action execution failed: %w", err)
	}

	w.db.WithContext(ctx).Model(&workflow).Update("run_count", workflow.RunCount+1)
	return nil
}

func (w *WorkflowWorker) evaluateConditions(conditions json.RawMessage, data map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	var wrapper map[string]interface{}
	if err := json.Unmarshal(conditions, &wrapper); err != nil {
		return true
	}

	condList, ok := wrapper["conditions"]
	if !ok {
		return true
	}

	conds, ok := condList.([]interface{})
	if !ok {
		return true
	}

	for _, c := range conds {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		field, _ := cond["field"].(string)
		operator, _ := cond["operator"].(string)
		expected := cond["value"]

		actual, exists := data[field]
		if !exists {
			return false
		}

		switch operator {
		case "eq":
			if fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected) {
				return false
			}
		case "neq":
			if fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", expected) {
				return false
			}
		case "contains":
			if !strings.Contains(
				strings.ToLower(fmt.Sprintf("%v", actual)),
				strings.ToLower(fmt.Sprintf("%v", expected)),
			) {
				return false
			}
		case "gt":
			actualF, _ := toFloat(actual)
			expectedF, _ := toFloat(expected)
			if actualF <= expectedF {
				return false
			}
		case "lt":
			actualF, _ := toFloat(actual)
			expectedF, _ := toFloat(expected)
			if actualF >= expectedF {
				return false
			}
		}
	}

	return true
}

func (w *WorkflowWorker) executeActions(
	ctx context.Context,
	wsID uuid.UUID,
	workflow domain.Workflow,
	p WorkflowPayload,
	data map[string]interface{},
) error {
	if len(workflow.Actions) == 0 {
		return nil
	}

	var wrapper map[string]interface{}
	if err := json.Unmarshal(workflow.Actions, &wrapper); err != nil {
		return nil
	}

	actions, ok := wrapper["actions"]
	if !ok {
		return nil
	}

	actionList, ok := actions.([]interface{})
	if !ok {
		return nil
	}

	for _, a := range actionList {
		action, ok := a.(map[string]interface{})
		if !ok {
			continue
		}

		actionType, _ := action["type"].(string)

		switch actionType {

		case "send_email":
			to, _ := action["to"].(string)
			subject, _ := action["subject"].(string)
			body, _ := action["body"].(string)
			toName, _ := action["to_name"].(string)

			if to == "" {
				if email, ok := data["email"].(string); ok {
					to = email
				}
			}
			if to != "" {
				_ = w.emailClient.EnqueueEmail(EmailPayload{
					To:      to,
					ToName:  toName,
					Subject: interpolate(subject, data),
					Body:    interpolate(body, data),
				})
			}

		case "create_task":
			title, _ := action["title"].(string)
			assigneeID, _ := action["assignee_id"].(string)
			daysUntilDue, _ := toFloat(action["days_until_due"])

			if title == "" || assigneeID == "" {
				continue
			}
			assignee, err := uuid.Parse(assigneeID)
			if err != nil {
				continue
			}

			dueAt := time.Now().AddDate(0, 0, int(daysUntilDue))
			entityID, _ := uuid.Parse(p.EntityID)

			t := &domain.Task{
				ID:          uuid.New(),
				WorkspaceID: wsID,
				Title:       interpolate(title, data),
				Priority:    domain.TaskPriorityMedium,
				DueAt:       &dueAt,
				AssigneeID:  assignee,
				CreatedByID: workflow.CreatedByID,
			}
			switch p.EntityType {
			case "contact":
				t.ContactID = &entityID
			case "deal":
				t.DealID = &entityID
			}
			_ = w.taskRepo.Create(ctx, t)

		case "notify":
			userID, _ := action["user_id"].(string)
			title, _ := action["title"].(string)
			body, _ := action["body"].(string)
			link, _ := action["link"].(string)

			uid, err := uuid.Parse(userID)
			if err != nil {
				continue
			}
			notif := &domain.Notification{
				ID:          uuid.New(),
				WorkspaceID: wsID,
				UserID:      uid,
				Title:       interpolate(title, data),
				Body:        interpolate(body, data),
				Link:        link,
			}
			_ = w.notifRepo.Create(ctx, notif)

		case "update_deal_stage":
			stageID, _ := action["stage_id"].(string)
			if stageID == "" || p.EntityType != "deal" {
				continue
			}
			w.db.WithContext(ctx).
				Model(&domain.Deal{}).
				Where("id = ? AND workspace_id = ?", p.EntityID, wsID).
				Update("stage_id", stageID)

		default:
			fmt.Printf("workflow worker: unknown action type %q in workflow %s\n", actionType, workflow.ID)
		}
	}

	return nil
}

func interpolate(s string, data map[string]interface{}) string {
	for k, v := range data {
		s = strings.ReplaceAll(s, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	return s
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
