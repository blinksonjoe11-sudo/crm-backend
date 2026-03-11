package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/repository"
)

type ImportWorker struct {
	contactRepo repository.ContactRepository
}

func NewImportWorker(contactRepo repository.ContactRepository) *ImportWorker {
	return &ImportWorker{contactRepo: contactRepo}
}

func (w *ImportWorker) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var p ImportPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return fmt.Errorf("import worker: failed to unmarshal payload: %w", err)
	}

	wsID, err := uuid.Parse(p.WorkspaceID)
	if err != nil {
		return fmt.Errorf("import worker: invalid workspace_id: %w", err)
	}

	ownerID, err := uuid.Parse(p.OwnerID)
	if err != nil {
		return fmt.Errorf("import worker: invalid owner_id: %w", err)
	}

	if len(p.Rows) < 2 {
		return fmt.Errorf("import worker: no data rows found")
	}

	
	headers := p.Rows[0]
	colIndex := map[string]int{}
	for i, h := range headers {
		colIndex[strings.TrimSpace(strings.ToLower(h))] = i
	}

	col := func(row []string, name string) string {
		i, ok := colIndex[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}

	var contacts []domain.Contact
	var skipped int

	for i, row := range p.Rows[1:] {
		if len(row) == 0 {
			continue
		}

		firstName := col(row, "first_name")
		if firstName == "" {
			firstName = col(row, "name")
		}
		if firstName == "" {
			skipped++
			fmt.Printf("import worker: skipping row %d — missing first_name\n", i+2)
			continue
		}

		email := strings.ToLower(col(row, "email"))

	
		duplicate := false
		for _, existing := range contacts {
			if email != "" && existing.Email == email {
				duplicate = true
				break
			}
		}
		if duplicate {
			skipped++
			continue
		}

		var tags []string
		if t := col(row, "tags"); t != "" {
			for _, tag := range strings.Split(t, ";") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
		}

		contact := domain.Contact{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			FirstName:   firstName,
			LastName:    col(row, "last_name"),
			Email:       email,
			Phone:       col(row, "phone"),
			Title:       col(row, "title"),
			Source:      col(row, "source"),
			Tags:        tags,
			OwnerID:     &ownerID,
		}

		contacts = append(contacts, contact)
	}

	if len(contacts) == 0 {
		return fmt.Errorf("import worker: no valid contacts to import (skipped %d rows)", skipped)
	}

	if err := w.contactRepo.BulkCreate(ctx, contacts); err != nil {
		return fmt.Errorf("import worker: bulk insert failed: %w", err)
	}

	fmt.Printf("import worker: imported %d contacts, skipped %d rows for workspace %s\n",
		len(contacts), skipped, p.WorkspaceID)
	return nil
}