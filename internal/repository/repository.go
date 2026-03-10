package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yourorg/crm/internal/domain"
	"gorm.io/gorm"
)

// ─── Pagination ───────────────────────────────────────────────────────────────

type Page struct {
	Page    int `form:"page,default=1"`
	PerPage int `form:"per_page,default=25"`
}

func (p Page) Offset() int {
	if p.Page < 1 {
		p.Page = 1
	}
	return (p.Page - 1) * p.PerPage
}

func (p Page) Limit() int {
	if p.PerPage <= 0 || p.PerPage > 100 {
		return 25
	}
	return p.PerPage
}


// ─── Contact Repository ───────────────────────────────────────────────────────

type ContactFilter struct {
	Search    string
	CompanyID *uuid.UUID
	OwnerID   *uuid.UUID
	Tags      []string
	Page
}

type ContactRepository interface {
	Create(ctx context.Context, c *domain.Contact) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Contact, error)
	GetByEmail(ctx context.Context, wsID uuid.UUID, email string) (*domain.Contact, error)
	List(ctx context.Context, wsID uuid.UUID, f ContactFilter) ([]domain.Contact, int64, error)
	Update(ctx context.Context, c *domain.Contact) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
	BulkCreate(ctx context.Context, contacts []domain.Contact) error
}

type contactRepo struct{ db *gorm.DB }

func NewContactRepository(db *gorm.DB) ContactRepository { return &contactRepo{db: db} }

func (r *contactRepo) Create(ctx context.Context, c *domain.Contact) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *contactRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Contact, error) {
	var c domain.Contact

	err := r.db.WithContext(ctx).
		Preload("Company").Preload("Owner").
		Where("workspace_id = ? AND id = ? AND archived_at IS NULL", wsID, id).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *contactRepo) GetByEmail(ctx context.Context, wsID uuid.UUID, email string) (*domain.Contact, error) {
	var c domain.Contact
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND email = ? AND archived_at IS NULL", wsID, email).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *contactRepo) List(ctx context.Context, wsID uuid.UUID, f ContactFilter) ([]domain.Contact, int64, error) {
	q := r.db.WithContext(ctx).
		Where("workspace_id = ? AND archived_at IS NULL", wsID)

	if f.Search != "" {
		like := "%" + f.Search + "%"
		q = q.Where("first_name ILIKE ? OR last_name ILIKE ? OR email ILIKE ?", like, like, like)
	}
	if f.CompanyID != nil {
		q = q.Where("company_id = ?", f.CompanyID)
	}
	if f.OwnerID != nil {
		q = q.Where("owner_id = ?", f.OwnerID)
	}

	var total int64
	q.Model(&domain.Contact{}).Count(&total)

	var contacts []domain.Contact
	err := q.Preload("Company").Preload("Owner").
		Order("created_at DESC").
		Limit(f.Limit()).Offset(f.Offset()).
		Find(&contacts).Error
	return contacts, total, err
}

func (r *contactRepo) Update(ctx context.Context, c *domain.Contact) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *contactRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.Contact{}).
		Where("workspace_id = ? AND id = ?", wsID, id).
		Update("archived_at", now).Error
}

func (r *contactRepo) BulkCreate(ctx context.Context, contacts []domain.Contact) error {
	return r.db.WithContext(ctx).CreateInBatches(contacts, 100).Error
}

// ─── Company Repository ───────────────────────────────────────────────────────

type CompanyFilter struct {
	Search   string
	Industry string
	OwnerID  *uuid.UUID
	Page
}

type CompanyRepository interface {
	Create(ctx context.Context, c *domain.Company) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Company, error)
	GetByDomain(ctx context.Context, wsID uuid.UUID, domain string) (*domain.Company, error)
	List(ctx context.Context, wsID uuid.UUID, f CompanyFilter) ([]domain.Company, int64, error)
	Update(ctx context.Context, c *domain.Company) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type companyRepo struct{ db *gorm.DB }

func NewCompanyRepository(db *gorm.DB) CompanyRepository { return &companyRepo{db: db} }

func (r *companyRepo) Create(ctx context.Context, c *domain.Company) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *companyRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Company, error) {
	var c domain.Company
	err := r.db.WithContext(ctx).Preload("Owner").
		Where("workspace_id = ? AND id = ? AND archived_at IS NULL", wsID, id).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *companyRepo) GetByDomain(ctx context.Context, wsID uuid.UUID, dom string) (*domain.Company, error) {
	var c domain.Company
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND domain = ? AND archived_at IS NULL", wsID, dom).
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *companyRepo) List(ctx context.Context, wsID uuid.UUID, f CompanyFilter) ([]domain.Company, int64, error) {
	q := r.db.WithContext(ctx).Where("workspace_id = ? AND archived_at IS NULL", wsID)
	if f.Search != "" {
		q = q.Where("name ILIKE ? OR domain ILIKE ?", "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.Industry != "" {
		q = q.Where("industry = ?", f.Industry)
	}
	var total int64
	q.Model(&domain.Company{}).Count(&total)
	var companies []domain.Company
	err := q.Preload("Owner").Order("name ASC").Limit(f.Limit()).Offset(f.Offset()).Find(&companies).Error
	return companies, total, err
}

func (r *companyRepo) Update(ctx context.Context, c *domain.Company) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *companyRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&domain.Company{}).
		Where("workspace_id = ? AND id = ?", wsID, id).
		Update("archived_at", now).Error
}

// ─── Deal Repository ──────────────────────────────────────────────────────────

type DealFilter struct {
	PipelineID *uuid.UUID
	StageID    string
	Status     domain.DealStatus
	OwnerID    *uuid.UUID
	Search     string
	Page
}

type DealRepository interface {
	Create(ctx context.Context, d *domain.Deal) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error)
	List(ctx context.Context, wsID uuid.UUID, f DealFilter) ([]domain.Deal, int64, error)
	Update(ctx context.Context, d *domain.Deal) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
	AddContact(ctx context.Context, dealID, contactID uuid.UUID) error
	RemoveContact(ctx context.Context, dealID, contactID uuid.UUID) error
}

type dealRepo struct{ db *gorm.DB }

func NewDealRepository(db *gorm.DB) DealRepository { return &dealRepo{db: db} }

func (r *dealRepo) Create(ctx context.Context, d *domain.Deal) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *dealRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error) {
	var d domain.Deal
	err := r.db.WithContext(ctx).
		Preload("Pipeline").Preload("Owner").Preload("Company").
		Preload("Contacts").Preload("LineItems").
		Where("workspace_id = ? AND id = ?", wsID, id).
		First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *dealRepo) List(ctx context.Context, wsID uuid.UUID, f DealFilter) ([]domain.Deal, int64, error) {
	q := r.db.WithContext(ctx).Where("workspace_id = ?", wsID)
	if f.PipelineID != nil {
		q = q.Where("pipeline_id = ?", f.PipelineID)
	}
	if f.StageID != "" {
		q = q.Where("stage_id = ?", f.StageID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.OwnerID != nil {
		q = q.Where("owner_id = ?", f.OwnerID)
	}
	if f.Search != "" {
		q = q.Where("title ILIKE ?", "%"+f.Search+"%")
	}
	var total int64
	q.Model(&domain.Deal{}).Count(&total)
	var deals []domain.Deal
	err := q.Preload("Owner").Preload("Pipeline").
		Order("created_at DESC").
		Limit(f.Limit()).Offset(f.Offset()).
		Find(&deals).Error
	return deals, total, err
}

func (r *dealRepo) Update(ctx context.Context, d *domain.Deal) error {
	return r.db.WithContext(ctx).Save(d).Error
}

func (r *dealRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND id = ?", wsID, id).
		Delete(&domain.Deal{}).Error
}

func (r *dealRepo) AddContact(ctx context.Context, dealID, contactID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Exec("INSERT INTO deal_contacts (deal_id, contact_id) VALUES (?,?) ON CONFLICT DO NOTHING", dealID, contactID).Error
}

func (r *dealRepo) RemoveContact(ctx context.Context, dealID, contactID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Exec("DELETE FROM deal_contacts WHERE deal_id=? AND contact_id=?", dealID, contactID).Error
}

// ─── Activity Repository ──────────────────────────────────────────────────────

type ActivityFilter struct {
	ContactID *uuid.UUID
	DealID    *uuid.UUID
	CompanyID *uuid.UUID
	UserID    *uuid.UUID
	Type      domain.ActivityType
	Page
}

type ActivityRepository interface {
	Create(ctx context.Context, a *domain.Activity) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Activity, error)
	List(ctx context.Context, wsID uuid.UUID, f ActivityFilter) ([]domain.Activity, int64, error)
	Update(ctx context.Context, a *domain.Activity) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type activityRepo struct{ db *gorm.DB }

func NewActivityRepository(db *gorm.DB) ActivityRepository { return &activityRepo{db: db} }

func (r *activityRepo) Create(ctx context.Context, a *domain.Activity) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *activityRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Activity, error) {
	var a domain.Activity
	err := r.db.WithContext(ctx).Preload("User").Preload("Contact").Preload("Deal").
		Where("workspace_id = ? AND id = ?", wsID, id).First(&a).Error
	return &a, err
}

func (r *activityRepo) List(ctx context.Context, wsID uuid.UUID, f ActivityFilter) ([]domain.Activity, int64, error) {
	q := r.db.WithContext(ctx).Where("workspace_id = ?", wsID)
	if f.ContactID != nil {
		q = q.Where("contact_id = ?", f.ContactID)
	}
	if f.DealID != nil {
		q = q.Where("deal_id = ?", f.DealID)
	}
	if f.UserID != nil {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	var total int64
	q.Model(&domain.Activity{}).Count(&total)
	var activities []domain.Activity
	err := q.Preload("User").Order("occurred_at DESC").
		Limit(f.Limit()).Offset(f.Offset()).Find(&activities).Error
	return activities, total, err
}

func (r *activityRepo) Update(ctx context.Context, a *domain.Activity) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *activityRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND id = ?", wsID, id).
		Delete(&domain.Activity{}).Error
}

// ─── Task Repository ──────────────────────────────────────────────────────────

type TaskFilter struct {
	AssigneeID  *uuid.UUID
	ContactID   *uuid.UUID
	DealID      *uuid.UUID
	Completed   *bool
	OverdueOnly bool
	Page
}

type TaskRepository interface {
	Create(ctx context.Context, t *domain.Task) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error)
	List(ctx context.Context, wsID uuid.UUID, f TaskFilter) ([]domain.Task, int64, error)
	Update(ctx context.Context, t *domain.Task) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type taskRepo struct{ db *gorm.DB }

func NewTaskRepository(db *gorm.DB) TaskRepository { return &taskRepo{db: db} }

func (r *taskRepo) Create(ctx context.Context, t *domain.Task) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *taskRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	err := r.db.WithContext(ctx).Preload("Assignee").Preload("Contact").Preload("Deal").
		Where("workspace_id = ? AND id = ?", wsID, id).First(&t).Error
	return &t, err
}

func (r *taskRepo) List(ctx context.Context, wsID uuid.UUID, f TaskFilter) ([]domain.Task, int64, error) {
	q := r.db.WithContext(ctx).Where("workspace_id = ?", wsID)
	if f.AssigneeID != nil {
		q = q.Where("assignee_id = ?", f.AssigneeID)
	}
	if f.ContactID != nil {
		q = q.Where("contact_id = ?", f.ContactID)
	}
	if f.DealID != nil {
		q = q.Where("deal_id = ?", f.DealID)
	}
	if f.Completed != nil {
		if *f.Completed {
			q = q.Where("completed_at IS NOT NULL")
		} else {
			q = q.Where("completed_at IS NULL")
		}
	}
	if f.OverdueOnly {
		q = q.Where("due_at < ? AND completed_at IS NULL", time.Now())
	}
	var total int64
	q.Model(&domain.Task{}).Count(&total)
	var tasks []domain.Task
	err := q.Preload("Assignee").Order("due_at ASC NULLS LAST").
		Limit(f.Limit()).Offset(f.Offset()).Find(&tasks).Error
	return tasks, total, err
}

func (r *taskRepo) Update(ctx context.Context, t *domain.Task) error {
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *taskRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("workspace_id = ? AND id = ?", wsID, id).Delete(&domain.Task{}).Error
}

// ─── User Repository ──────────────────────────────────────────────────────────

type UserRepository interface {
	CreateWorkspace(ctx context.Context, ws *domain.Workspace) error
	Create(ctx context.Context, u *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByGoogleID(ctx context.Context, googleID string) (*domain.User, error)
	ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]domain.User, error)
	Update(ctx context.Context, u *domain.User) error
	Delete(ctx context.Context, id uuid.UUID) error
	SaveRefreshToken(ctx context.Context, rt *domain.RefreshToken) error
	GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, token string) error
	DeleteAllRefreshTokens(ctx context.Context, userID uuid.UUID) error
}

type userRepo struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) UserRepository { return &userRepo{db: db} }

func (r *userRepo) CreateWorkspace(ctx context.Context, ws *domain.Workspace) error {
	return r.db.WithContext(ctx).Create(ws).Error
}

func (r *userRepo) Create(ctx context.Context, u *domain.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *userRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).Preload("Workspace").Where("id = ?", id).First(&u).Error
	return &u, err
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).Preload("Workspace").Where("email = ?", email).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) GetByGoogleID(ctx context.Context, googleID string) (*domain.User, error) {
	var u domain.User
	err := r.db.WithContext(ctx).Where("google_id = ?", googleID).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]domain.User, error) {
	var users []domain.User
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND is_active = true", wsID).Find(&users).Error
	return users, err
}

func (r *userRepo) Update(ctx context.Context, u *domain.User) error {
	return r.db.WithContext(ctx).Save(u).Error
}

func (r *userRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", id).Update("is_active", false).Error
}

func (r *userRepo) SaveRefreshToken(ctx context.Context, rt *domain.RefreshToken) error {
	return r.db.WithContext(ctx).Create(rt).Error
}

func (r *userRepo) GetRefreshToken(ctx context.Context, token string) (*domain.RefreshToken, error) {
	var rt domain.RefreshToken
	err := r.db.WithContext(ctx).Where("token = ?", token).First(&rt).Error
	return &rt, err
}

func (r *userRepo) DeleteRefreshToken(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Where("token = ?", token).Delete(&domain.RefreshToken{}).Error
}

func (r *userRepo) DeleteAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&domain.RefreshToken{}).Error
}

// ─── Pipeline Repository ──────────────────────────────────────────────────────

type PipelineRepository interface {
	Create(ctx context.Context, p *domain.Pipeline) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Pipeline, error)
	List(ctx context.Context, wsID uuid.UUID) ([]domain.Pipeline, error)
	Update(ctx context.Context, p *domain.Pipeline) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type pipelineRepo struct{ db *gorm.DB }

func NewPipelineRepository(db *gorm.DB) PipelineRepository { return &pipelineRepo{db: db} }

func (r *pipelineRepo) Create(ctx context.Context, p *domain.Pipeline) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *pipelineRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Pipeline, error) {
	var p domain.Pipeline
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", wsID, id).First(&p).Error
	return &p, err
}

func (r *pipelineRepo) List(ctx context.Context, wsID uuid.UUID) ([]domain.Pipeline, error) {
	var pipelines []domain.Pipeline
	err := r.db.WithContext(ctx).Where("workspace_id = ?", wsID).Order("created_at ASC").Find(&pipelines).Error
	return pipelines, err
}

func (r *pipelineRepo) Update(ctx context.Context, p *domain.Pipeline) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *pipelineRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", wsID, id).Delete(&domain.Pipeline{}).Error
}

// ─── Webhook Repository ───────────────────────────────────────────────────────

type WebhookRepository interface {
	Create(ctx context.Context, w *domain.Webhook) error
	GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Webhook, error)
	List(ctx context.Context, wsID uuid.UUID) ([]domain.Webhook, error)
	Update(ctx context.Context, w *domain.Webhook) error
	Delete(ctx context.Context, wsID, id uuid.UUID) error
	FindByEvent(ctx context.Context, wsID uuid.UUID, event string) ([]domain.Webhook, error)
	SaveDelivery(ctx context.Context, d *domain.WebhookDelivery) error
	ListDeliveries(ctx context.Context, webhookID uuid.UUID, pg Page) ([]domain.WebhookDelivery, int64, error)
}

type webhookRepo struct{ db *gorm.DB }

func NewWebhookRepository(db *gorm.DB) WebhookRepository { return &webhookRepo{db: db} }

func (r *webhookRepo) Create(ctx context.Context, w *domain.Webhook) error {
	return r.db.WithContext(ctx).Create(w).Error
}

func (r *webhookRepo) GetByID(ctx context.Context, wsID, id uuid.UUID) (*domain.Webhook, error) {
	var w domain.Webhook
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", wsID, id).First(&w).Error
	return &w, err
}

func (r *webhookRepo) List(ctx context.Context, wsID uuid.UUID) ([]domain.Webhook, error) {
	var hooks []domain.Webhook
	err := r.db.WithContext(ctx).Where("workspace_id = ?", wsID).Find(&hooks).Error
	return hooks, err
}

func (r *webhookRepo) Update(ctx context.Context, w *domain.Webhook) error {
	return r.db.WithContext(ctx).Save(w).Error
}

func (r *webhookRepo) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("workspace_id = ? AND id = ?", wsID, id).Delete(&domain.Webhook{}).Error
}

func (r *webhookRepo) FindByEvent(ctx context.Context, wsID uuid.UUID, event string) ([]domain.Webhook, error) {
	var hooks []domain.Webhook
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND is_active = true AND events @> ?", wsID, `["`+event+`"]`).
		Find(&hooks).Error
	return hooks, err
}

func (r *webhookRepo) SaveDelivery(ctx context.Context, d *domain.WebhookDelivery) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *webhookRepo) ListDeliveries(ctx context.Context, webhookID uuid.UUID, pg Page) ([]domain.WebhookDelivery, int64, error) {
	var total int64
	r.db.WithContext(ctx).Model(&domain.WebhookDelivery{}).Where("webhook_id = ?", webhookID).Count(&total)
	var deliveries []domain.WebhookDelivery
	err := r.db.WithContext(ctx).Where("webhook_id = ?", webhookID).
		Order("created_at DESC").Limit(pg.Limit()).Offset(pg.Offset()).Find(&deliveries).Error
	return deliveries, total, err
}

// ─── Audit Log Repository ─────────────────────────────────────────────────────

type AuditRepository interface {
	Log(ctx context.Context, entry *domain.AuditLog) error
	List(ctx context.Context, wsID uuid.UUID, entity string, pg Page) ([]domain.AuditLog, int64, error)
}

type auditRepo struct{ db *gorm.DB }

func NewAuditRepository(db *gorm.DB) AuditRepository { return &auditRepo{db: db} }

func (r *auditRepo) Log(ctx context.Context, entry *domain.AuditLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *auditRepo) List(ctx context.Context, wsID uuid.UUID, entity string, pg Page) ([]domain.AuditLog, int64, error) {
	q := r.db.WithContext(ctx).Where("workspace_id = ?", wsID)
	if entity != "" {
		q = q.Where("entity = ?", entity)
	}
	var total int64
	q.Model(&domain.AuditLog{}).Count(&total)
	var logs []domain.AuditLog
	err := q.Preload("Actor").Order("created_at DESC").Limit(pg.Limit()).Offset(pg.Offset()).Find(&logs).Error
	return logs, total, err
}

// ─── Notification Repository ──────────────────────────────────────────────────

type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	List(ctx context.Context, userID uuid.UUID, pg Page) ([]domain.Notification, int64, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
}

type notifRepo struct{ db *gorm.DB }

func NewNotificationRepository(db *gorm.DB) NotificationRepository { return &notifRepo{db: db} }

func (r *notifRepo) Create(ctx context.Context, n *domain.Notification) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *notifRepo) List(ctx context.Context, userID uuid.UUID, pg Page) ([]domain.Notification, int64, error) {
	var total int64
	r.db.WithContext(ctx).Model(&domain.Notification{}).Where("user_id = ?", userID).Count(&total)
	var notifs []domain.Notification
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC").Limit(pg.Limit()).Offset(pg.Offset()).Find(&notifs).Error
	return notifs, total, err
}

func (r *notifRepo) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&domain.Notification{}).
		Where("user_id = ? AND id = ?", userID, id).Update("read_at", time.Now()).Error
}

func (r *notifRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&domain.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).Update("read_at", time.Now()).Error
}
