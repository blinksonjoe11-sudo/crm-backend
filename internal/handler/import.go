package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yourorg/crm/internal/middleware"
	"github.com/yourorg/crm/internal/worker"
)

type ImportHandler struct {
	workerClient *worker.Client
}

func NewImportHandler(workerClient *worker.Client) *ImportHandler {
	return &ImportHandler{workerClient: workerClient}
}

// ImportContacts — POST /contacts/import
// Accepts multipart/form-data with field "file" (CSV or JSON)
func (h *ImportHandler) ImportContacts(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	userID := middleware.UserIDFromCtx(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"message": "file field is required"},
		})
		return
	}
	defer file.Close()

	if header.Size > 10<<20 { // 10 MB limit
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"message": "file too large, max 10 MB"},
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))

	var rows [][]string

	switch ext {
	case ".csv":
		rows, err = parseCSV(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   gin.H{"message": fmt.Sprintf("invalid CSV: %s", err.Error())},
			})
			return
		}

	case ".json":
		rows, err = parseJSON(file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   gin.H{"message": fmt.Sprintf("invalid JSON: %s", err.Error())},
			})
			return
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"message": "only .csv and .json files are supported"},
		})
		return
	}

	if len(rows) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"message": "file must have a header row and at least one data row"},
		})
		return
	}

	if err := h.workerClient.EnqueueImport(worker.ImportPayload{
		WorkspaceID: wsID.String(),
		OwnerID:     userID.String(),
		Rows:        rows,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   gin.H{"message": "failed to queue import"},
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"message":    "import queued successfully",
			"total_rows": len(rows) - 1, // exclude header
			"status":     "processing",
		},
	})
}

// ─── parsers ─────────────────────────────────────────────────────────────────

func parseCSV(r io.Reader) ([][]string, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // allow variable columns

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// parseJSON converts a JSON array of objects into [][]string (header + rows)
// so it matches the same format the import worker expects.
//
// Input JSON format:
// [
//   {"first_name":"Jane","last_name":"Smith","email":"jane@example.com"},
//   {"first_name":"Bob","last_name":"Jones","email":"bob@example.com"}
// ]
func parseJSON(r io.Reader) ([][]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var records []map[string]string
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("expected a JSON array of objects: %w", err)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("JSON array is empty")
	}

	// Use a fixed column order matching what the import worker expects
	headers := []string{
		"first_name", "last_name", "email", "phone",
		"title", "source", "tags",
	}

	rows := [][]string{headers}

	for _, record := range records {
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = record[h]
		}
		rows = append(rows, row)
	}

	return rows, nil
}