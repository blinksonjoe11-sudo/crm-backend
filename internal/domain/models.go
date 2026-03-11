package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil || len(j) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = JSONB{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan type %T into JSONB", value)
	}
	return json.Unmarshal(bytes, j)
}


type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}

	quoted := make([]string, len(s))
	for i, v := range s {
		v = strings.ReplaceAll(v, `\`, `\\`)
		v = strings.ReplaceAll(v, `"`, `\"`)
		quoted[i] = `"` + v + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}", nil
}

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}
	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	default:
		return fmt.Errorf("cannot scan type %T into StringArray", value)
	}
	
	str = strings.TrimPrefix(str, "{")
	str = strings.TrimSuffix(str, "}")
	if str == "" {
		*s = []string{}
		return nil
	}

	var result []string
	for _, part := range strings.Split(str, ",") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, `"`)
		part = strings.ReplaceAll(part, `\"`, `"`)
		part = strings.ReplaceAll(part, `\\`, `\`)
		result = append(result, part)
	}
	*s = result
	return nil
}

//  WORKSPACE

type Plan string

const (
	PlanFree       Plan = "free"
	PlanStarter    Plan = "starter"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
)

type Workspace struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name      string     `gorm:"size:200;not null"                              json:"name"`
	Slug      string     `gorm:"size:100;uniqueIndex;not null"                  json:"slug"`
	Plan      Plan       `gorm:"size:50;default:'free'"                         json:"plan"`
	Settings  JSONB      `gorm:"type:jsonb"                                     json:"settings"`
	LogoURL   string     `gorm:"size:500"                                       json:"logo_url,omitempty"`
	CreatedAt time.Time  `                                                      json:"created_at"`
	UpdatedAt time.Time  `                                                      json:"updated_at"`
	DeletedAt *time.Time `gorm:"index"                                          json:"deleted_at,omitempty"`
}

//  USER

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleManager  Role = "manager"
	RoleSalesRep Role = "sales_rep"
	RoleReadOnly Role = "read_only"
)

type User struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID   uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	FirstName     string         `gorm:"size:100;not null"                              json:"first_name"`
	LastName      string         `gorm:"size:100"                                       json:"last_name"`
	Email         string         `gorm:"size:255;not null;uniqueIndex"                  json:"email"`
	PasswordHash  string         `gorm:"size:255"                                       json:"-"`
	Role          Role           `gorm:"size:50;default:'sales_rep'"                    json:"role"`
	AvatarURL     string         `gorm:"size:500"                                       json:"avatar_url,omitempty"`
	IsActive      bool           `gorm:"default:true"                                   json:"is_active"`
	LastLoginAt   *time.Time     `                                                      json:"last_login_at,omitempty"`
	GoogleID      string         `gorm:"size:255;index"                                 json:"-"`
	RefreshTokens []RefreshToken `gorm:"foreignKey:UserID"                         json:"-"`
	CreatedAt     time.Time      `                                                      json:"created_at"`
	UpdatedAt     time.Time      `                                                      json:"updated_at"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
}

func (u *User) FullName() string {
	if u.LastName == "" {
		return u.FirstName
	}
	return u.FirstName + " " + u.LastName
}

type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"                       json:"user_id"`
	Token     string    `gorm:"size:500;not null;uniqueIndex"                  json:"-"`
	ExpiresAt time.Time `                                                      json:"expires_at"`
	CreatedAt time.Time `                                                      json:"created_at"`
}

//  CONTACT 

type Contact struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID   `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	FirstName   string      `gorm:"size:100;not null"                              json:"first_name"`
	LastName    string      `gorm:"size:100"                                       json:"last_name"`
	Email       string      `gorm:"size:255;index"                                 json:"email,omitempty"`
	Phone       string      `gorm:"size:50"                                        json:"phone,omitempty"`
	Title       string      `gorm:"size:200"                                       json:"title,omitempty"`
	LinkedInURL string      `gorm:"size:500"                                       json:"linkedin_url,omitempty"`
	CompanyID   *uuid.UUID  `gorm:"type:uuid;index"                                json:"company_id,omitempty"`
	OwnerID     *uuid.UUID  `gorm:"type:uuid;index"                                json:"owner_id,omitempty"`
	Score       int         `gorm:"default:0"                                      json:"score"`
	Tags        StringArray `gorm:"type:text[]"                        json:"tags"`
	CustomData  JSONB       `gorm:"type:jsonb"                        json:"custom_data"`
	Source      string      `gorm:"size:100"                                       json:"source,omitempty"`
	ArchivedAt  *time.Time  `gorm:"index"                                          json:"archived_at,omitempty"`
	CreatedAt   time.Time   `                                                      json:"created_at"`
	UpdatedAt   time.Time   `                                                      json:"updated_at"`

	Company    *Company   `gorm:"foreignKey:CompanyID"     json:"company,omitempty"`
	Owner      *User      `gorm:"foreignKey:OwnerID"       json:"owner,omitempty"`
	Activities []Activity `gorm:"foreignKey:ContactID"     json:"activities,omitempty"`
	Deals      []Deal     `gorm:"many2many:deal_contacts;" json:"deals,omitempty"`
}

func (c *Contact) FullName() string {
	if c.LastName == "" {
		return c.FirstName
	}
	return c.FirstName + " " + c.LastName
}

//  COMPANY

type Company struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Name        string     `gorm:"size:200;not null"                              json:"name"`
	Domain      string     `gorm:"size:255;index"                                 json:"domain,omitempty"`
	Industry    string     `gorm:"size:100"                                       json:"industry,omitempty"`
	Size        string     `gorm:"size:50"                                        json:"size,omitempty"` // e.g. "1-10", "11-50"
	Revenue     float64    `                                                      json:"revenue,omitempty"`
	Website     string     `gorm:"size:500"                                       json:"website,omitempty"`
	Phone       string     `gorm:"size:50"                                        json:"phone,omitempty"`
	Address     JSONB      `gorm:"type:jsonb"                        json:"address"`
	CustomData  JSONB      `gorm:"type:jsonb"                        json:"custom_data"`
	OwnerID     *uuid.UUID `gorm:"type:uuid;index"                                json:"owner_id,omitempty"`
	ArchivedAt  *time.Time `gorm:"index"                                          json:"archived_at,omitempty"`
	CreatedAt   time.Time  `                                                      json:"created_at"`
	UpdatedAt   time.Time  `                                                      json:"updated_at"`

	Owner    *User     `gorm:"foreignKey:OwnerID"   json:"owner,omitempty"`
	Contacts []Contact `gorm:"foreignKey:CompanyID" json:"contacts,omitempty"`
	Deals    []Deal    `gorm:"foreignKey:CompanyID" json:"deals,omitempty"`
}

//  PIPELINE & DEAL 

type PipelineStage struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Order       int     `json:"order"`
	Probability float64 `json:"probability"` 
	Color       string  `json:"color"`
}

type Pipeline struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID       `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Name        string          `gorm:"size:200;not null"                              json:"name"`
	Stages      json.RawMessage `gorm:"type:jsonb;not null"                            json:"stages"`
	IsDefault   bool            `gorm:"default:false"                                  json:"is_default"`
	CreatedAt   time.Time       `                                                      json:"created_at"`
	UpdatedAt   time.Time       `                                                      json:"updated_at"`
}

type DealStatus string

const (
	DealStatusOpen DealStatus = "open"
	DealStatusWon  DealStatus = "won"
	DealStatusLost DealStatus = "lost"
)

type Deal struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID   uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Title         string     `gorm:"size:300;not null"                              json:"title"`
	PipelineID    uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"pipeline_id"`
	StageID       string     `gorm:"size:100;not null"                              json:"stage_id"`
	Status        DealStatus `gorm:"size:20;default:'open'"                         json:"status"`
	Value         float64    `gorm:"default:0"                                      json:"value"`
	Currency      string     `gorm:"size:3;default:'USD'"                           json:"currency"`
	Probability   float64    `gorm:"default:0"                                      json:"probability"`
	ExpectedClose *time.Time `                                                      json:"expected_close,omitempty"`
	ClosedAt      *time.Time `                                                      json:"closed_at,omitempty"`
	LostReason    string     `gorm:"size:500"                                       json:"lost_reason,omitempty"`
	OwnerID       uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"owner_id"`
	CompanyID     *uuid.UUID `gorm:"type:uuid;index"                                json:"company_id,omitempty"`
	CustomData    JSONB      `gorm:"type:jsonb"                        json:"custom_data"`
	CreatedAt     time.Time  `                                                      json:"created_at"`
	UpdatedAt     time.Time  `                                                      json:"updated_at"`

	Pipeline   *Pipeline  `gorm:"foreignKey:PipelineID"    json:"pipeline,omitempty"`
	Owner      *User      `gorm:"foreignKey:OwnerID"       json:"owner,omitempty"`
	Company    *Company   `gorm:"foreignKey:CompanyID"     json:"company,omitempty"`
	Contacts   []Contact  `gorm:"many2many:deal_contacts;" json:"contacts,omitempty"`
	LineItems  []LineItem `gorm:"foreignKey:DealID"        json:"line_items,omitempty"`
	Activities []Activity `gorm:"foreignKey:DealID"       json:"activities,omitempty"`
}

type LineItem struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	DealID      uuid.UUID `gorm:"type:uuid;not null;index"                       json:"deal_id"`
	Name        string    `gorm:"size:300;not null"                              json:"name"`
	Description string    `gorm:"size:1000"                                      json:"description,omitempty"`
	Quantity    float64   `gorm:"default:1"                                      json:"quantity"`
	UnitPrice   float64   `gorm:"default:0"                                      json:"unit_price"`
	Discount    float64   `gorm:"default:0"                                      json:"discount"` // percentage
	CreatedAt   time.Time `                                                      json:"created_at"`
}

func (li *LineItem) Total() float64 {
	return li.Quantity * li.UnitPrice * (1 - li.Discount/100)
}

//  ACTIVITY 

type ActivityType string

const (
	ActivityCall    ActivityType = "call"
	ActivityEmail   ActivityType = "email"
	ActivityMeeting ActivityType = "meeting"
	ActivityNote    ActivityType = "note"
	ActivityTask    ActivityType = "task"
)

type Activity struct {
	ID          uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Type        ActivityType `gorm:"size:50;not null"                               json:"type"`
	Subject     string       `gorm:"size:300"                                       json:"subject,omitempty"`
	Body        string       `gorm:"type:text"                                      json:"body,omitempty"`
	ContactID   *uuid.UUID   `gorm:"type:uuid;index"                                json:"contact_id,omitempty"`
	DealID      *uuid.UUID   `gorm:"type:uuid;index"                                json:"deal_id,omitempty"`
	CompanyID   *uuid.UUID   `gorm:"type:uuid;index"                                json:"company_id,omitempty"`
	UserID      uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"user_id"`
	OccurredAt  time.Time    `gorm:"not null"                                       json:"occurred_at"`
	Duration    int          `gorm:"default:0"                                      json:"duration"` // minutes
	Metadata    JSONB        `gorm:"type:jsonb"                        json:"metadata"`
	CreatedAt   time.Time    `                                                      json:"created_at"`
	UpdatedAt   time.Time    `                                                      json:"updated_at"`

	Contact *Contact `gorm:"foreignKey:ContactID" json:"contact,omitempty"`
	Deal    *Deal    `gorm:"foreignKey:DealID"    json:"deal,omitempty"`
	User    *User    `gorm:"foreignKey:UserID"    json:"user,omitempty"`
}

//  TASK 

type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
)

type Task struct {
	ID          uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Title       string       `gorm:"size:500;not null"                              json:"title"`
	Description string       `gorm:"type:text"                                      json:"description,omitempty"`
	Priority    TaskPriority `gorm:"size:20;default:'medium'"                       json:"priority"`
	DueAt       *time.Time   `gorm:"index"                                          json:"due_at,omitempty"`
	CompletedAt *time.Time   `                                                      json:"completed_at,omitempty"`
	AssigneeID  uuid.UUID    `gorm:"type:uuid;not null;index"                       json:"assignee_id"`
	CreatedByID uuid.UUID    `gorm:"type:uuid;not null"                             json:"created_by_id"`
	ContactID   *uuid.UUID   `gorm:"type:uuid;index"                                json:"contact_id,omitempty"`
	DealID      *uuid.UUID   `gorm:"type:uuid;index"                                json:"deal_id,omitempty"`
	CreatedAt   time.Time    `                                                      json:"created_at"`
	UpdatedAt   time.Time    `                                                      json:"updated_at"`

	Assignee *User    `gorm:"foreignKey:AssigneeID"  json:"assignee,omitempty"`
	Contact  *Contact `gorm:"foreignKey:ContactID"   json:"contact,omitempty"`
	Deal     *Deal    `gorm:"foreignKey:DealID"      json:"deal,omitempty"`
}

//  EMAIL TEMPLATE 

type EmailTemplate struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Name        string    `gorm:"size:200;not null"                              json:"name"`
	Subject     string    `gorm:"size:500;not null"                              json:"subject"`
	Body        string    `gorm:"type:text;not null"                             json:"body"`
	CreatedByID uuid.UUID `gorm:"type:uuid;not null"                             json:"created_by_id"`
	CreatedAt   time.Time `                                                      json:"created_at"`
	UpdatedAt   time.Time `                                                      json:"updated_at"`
}

//  WORKFLOW 

type Workflow struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID       `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Name        string          `gorm:"size:200;not null"                              json:"name"`
	Trigger     json.RawMessage `gorm:"type:jsonb;not null"                            json:"trigger"`
	Conditions  json.RawMessage `gorm:"type:jsonb"  json:"conditions"`
	Actions     json.RawMessage `gorm:"type:jsonb"  json:"actions"`
	IsActive    bool            `gorm:"default:false"                                  json:"is_active"`
	CreatedByID uuid.UUID       `gorm:"type:uuid;not null"                             json:"created_by_id"`
	RunCount    int             `gorm:"default:0"                                      json:"run_count"`
	CreatedAt   time.Time       `                                                      json:"created_at"`
	UpdatedAt   time.Time       `                                                      json:"updated_at"`
}

//  CUSTOM FIELD 

type FieldType string

const (
	FieldTypeText        FieldType = "text"
	FieldTypeNumber      FieldType = "number"
	FieldTypeDate        FieldType = "date"
	FieldTypeDropdown    FieldType = "dropdown"
	FieldTypeMultiSelect FieldType = "multi_select"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeURL         FieldType = "url"
)

type CustomField struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID       `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Entity      string          `gorm:"size:50;not null;index"                         json:"entity"` // contact, company, deal
	Name        string          `gorm:"size:100;not null"                              json:"name"`
	Key         string          `gorm:"size:100;not null"                              json:"key"`
	FieldType   FieldType       `gorm:"size:50;not null"                               json:"field_type"`
	Options     json.RawMessage `gorm:"type:jsonb;default:'[]'"                        json:"options"`
	IsRequired  bool            `gorm:"default:false"                                  json:"is_required"`
	Order       int             `gorm:"default:0"                                      json:"order"`
	CreatedAt   time.Time       `                                                      json:"created_at"`
}

//  WEBHOOK 

type Webhook struct {
	ID          uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID   `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	URL         string      `gorm:"size:1000;not null"                             json:"url"`
	Events      StringArray `gorm:"type:jsonb;not null"                            json:"events"`
	Secret      string      `gorm:"size:255"                                       json:"-"`
	IsActive    bool        `gorm:"default:true"                                   json:"is_active"`
	CreatedByID uuid.UUID   `gorm:"type:uuid;not null"                             json:"created_by_id"`
	CreatedAt   time.Time   `                                                      json:"created_at"`
	UpdatedAt   time.Time   `                                                      json:"updated_at"`
}

type WebhookDelivery struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WebhookID   uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"webhook_id"`
	Event       string     `gorm:"size:100;not null"                              json:"event"`
	Payload     JSONB      `gorm:"type:jsonb;not null"                            json:"payload"`
	StatusCode  int        `                                                      json:"status_code"`
	Response    string     `gorm:"type:text"                                      json:"response,omitempty"`
	Attempts    int        `gorm:"default:0"                                      json:"attempts"`
	DeliveredAt *time.Time `                                                     json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `                                                     json:"created_at"`
}

//  AUDIT LOG 

type AuditLog struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	ActorID     uuid.UUID `gorm:"type:uuid;not null;index"                       json:"actor_id"`
	Action      string    `gorm:"size:50;not null"                               json:"action"` // create, update, delete
	Entity      string    `gorm:"size:50;not null"                               json:"entity"` // contact, deal, etc.
	EntityID    uuid.UUID `gorm:"type:uuid;not null;index"                       json:"entity_id"`
	Diff        JSONB     `gorm:"type:jsonb;default:'{}'"                        json:"diff"`
	IPAddress   string    `gorm:"size:45"                                        json:"ip_address,omitempty"`
	CreatedAt   time.Time `gorm:"index"                                          json:"created_at"`

	Actor *User `gorm:"foreignKey:ActorID" json:"actor,omitempty"`
}

//  NOTIFICATION 

type Notification struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	UserID      uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"user_id"`
	Title       string     `gorm:"size:300;not null"                              json:"title"`
	Body        string     `gorm:"size:1000"                                      json:"body,omitempty"`
	Link        string     `gorm:"size:500"                                       json:"link,omitempty"`
	ReadAt      *time.Time `                                                      json:"read_at,omitempty"`
	CreatedAt   time.Time  `                                                      json:"created_at"`
}

//  TEAM 

type Team struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;index"                       json:"workspace_id"`
	Name        string    `gorm:"size:200;not null"                              json:"name"`
	Description string    `gorm:"size:1000"                                      json:"description,omitempty"`
	CreatedAt   time.Time `                                                      json:"created_at"`
	UpdatedAt   time.Time `                                                      json:"updated_at"`

	Members []User `gorm:"many2many:team_members;" json:"members,omitempty"`
}
