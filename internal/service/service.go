package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/yourorg/crm/internal/config"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/repository"
	"github.com/yourorg/crm/internal/worker"
	"github.com/yourorg/crm/pkg/jwtutil"
	"github.com/yourorg/crm/pkg/mailer"
	"golang.org/x/crypto/bcrypt"
)

// ─── Errors ───────────────────────────────────────────────────────────────────

var (
	ErrNotFound           = errors.New("not found")
	ErrDuplicate          = errors.New("already exists")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrWeakPassword       = errors.New("password must be at least 8 characters with upper, lower, digit")
)

// ─── Auth Service ─────────────────────────────────────────────────────────────

type RegisterInput struct {
	WorkspaceName string `json:"workspace_name" binding:"required,min=2,max=100"`
	FirstName     string `json:"first_name"     binding:"required,min=1,max=100"`
	LastName      string `json:"last_name"      binding:"omitempty,max=100"`
	Email         string `json:"email"          binding:"required,email"`
	Password      string `json:"password"       binding:"required,min=8"`
}

type LoginInput struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResult struct {
	User        *domain.User `json:"user"`
	AccessToken string       `json:"access_token"`
	ExpiresIn   int64        `json:"expires_in"`
	// Refresh token is set as httpOnly cookie by handler
}

type AuthService interface {
	Register(ctx context.Context, in RegisterInput) (*AuthResult, string, error)
	Login(ctx context.Context, in LoginInput) (*AuthResult, string, error)
	RefreshTokens(ctx context.Context, refreshToken string) (*AuthResult, string, error)
	Logout(ctx context.Context, refreshToken string) error
	Me(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID uuid.UUID, oldPass, newPass string) error
}

type authService struct {
	users  repository.UserRepository
	cfg    *config.Config
	mailer *mailer.Mailer
}

func NewAuthService(users repository.UserRepository, cfg *config.Config, m *mailer.Mailer) AuthService {
	return &authService{users: users, cfg: cfg, mailer: m}
}

func (s *authService) Register(ctx context.Context, in RegisterInput) (*AuthResult, string, error) {
	if err := validatePassword(in.Password); err != nil {
		return nil, "", err
	}

	// no error + existing user found = duplicate
	existing, _ := s.users.GetByEmail(ctx, strings.ToLower(in.Email))
	if existing != nil {
		return nil, "", ErrDuplicate
	}

	// Save workspace first so ws.ID is populated by GORM
	ws := &domain.Workspace{
		ID:   uuid.New(),
		Name: in.WorkspaceName,
		Slug: slugify(in.WorkspaceName),
		Plan: domain.PlanFree,
	}
	if err := s.users.CreateWorkspace(ctx, ws); err != nil {
		return nil, "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return nil, "", err
	}

	user := &domain.User{
		ID:           uuid.New(),
		WorkspaceID:  ws.ID,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Email:        strings.ToLower(in.Email),
		PasswordHash: string(hash),
		Role:         domain.RoleOwner,
		IsActive:     true,
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, "", err
	}

	user.Workspace = ws
	return s.issueTokens(ctx, user)
}

func (s *authService) Login(ctx context.Context, in LoginInput) (*AuthResult, string, error) {
	user, err := s.users.GetByEmail(ctx, strings.ToLower(in.Email))
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, "", ErrForbidden
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	now := time.Now()
	user.LastLoginAt = &now
	_ = s.users.Update(ctx, user)

	return s.issueTokens(ctx, user)
}

func (s *authService) RefreshTokens(ctx context.Context, refreshToken string) (*AuthResult, string, error) {
	rt, err := s.users.GetRefreshToken(ctx, refreshToken)
	if err != nil || rt.ExpiresAt.Before(time.Now()) {
		return nil, "", ErrInvalidToken
	}

	user, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, "", ErrInvalidToken
	}

	// Rotate refresh token
	_ = s.users.DeleteRefreshToken(ctx, refreshToken)
	return s.issueTokens(ctx, user)
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	return s.users.DeleteRefreshToken(ctx, refreshToken)
}

func (s *authService) Me(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	return s.users.GetByID(ctx, userID)
}

func (s *authService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.users.GetByEmail(ctx, strings.ToLower(email))
	if err != nil {
		return nil // silently succeed for security
	}

	token, _ := generateSecureToken(32)
	rt := &domain.RefreshToken{
		UserID:    user.ID,
		Token:     "reset_" + token,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.users.SaveRefreshToken(ctx, rt); err != nil {
		return err
	}

	body, _ := mailer.RenderTemplate(mailer.PasswordResetTemplate, map[string]string{
		"ResetURL": s.cfg.AppURL + "/reset-password?token=" + token,
	})
	_ = s.mailer.Send(mailer.Mail{To: user.Email, ToName: user.FullName(), Subject: "Reset your CRM password", Body: body})
	return nil
}

func (s *authService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	rt, err := s.users.GetRefreshToken(ctx, "reset_"+token)
	if err != nil || rt.ExpiresAt.Before(time.Now()) {
		return ErrInvalidToken
	}

	user, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return ErrNotFound
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	user.PasswordHash = string(hash)
	_ = s.users.Update(ctx, user)
	_ = s.users.DeleteRefreshToken(ctx, rt.Token)
	_ = s.users.DeleteAllRefreshTokens(ctx, user.ID)
	return nil
}

func (s *authService) ChangePassword(ctx context.Context, userID uuid.UUID, oldPass, newPass string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return ErrNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPass)); err != nil {
		return ErrInvalidCredentials
	}
	if err := validatePassword(newPass); err != nil {
		return err
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(newPass), 12)
	user.PasswordHash = string(hash)
	return s.users.Update(ctx, user)
}

func (s *authService) issueTokens(ctx context.Context, user *domain.User) (*AuthResult, string, error) {
	access, err := jwtutil.GenerateAccess(user, s.cfg.JWTSecret, s.cfg.JWTExpiry)
	if err != nil {
		return nil, "", err
	}

	rawRefresh, _ := generateSecureToken(48)
	rt := &domain.RefreshToken{
		UserID:    user.ID,
		Token:     rawRefresh,
		ExpiresAt: time.Now().Add(s.cfg.JWTRefreshExpiry),
	}
	if err := s.users.SaveRefreshToken(ctx, rt); err != nil {
		return nil, "", err
	}

	return &AuthResult{
		User:        user,
		AccessToken: access,
		ExpiresIn:   int64(s.cfg.JWTExpiry.Seconds()),
	}, rawRefresh, nil
}

// ─── Contact Service

type CreateContactInput struct {
	FirstName   string                 `json:"first_name" binding:"required,min=1,max=100"`
	LastName    string                 `json:"last_name"  binding:"omitempty,max=100"`
	Email       string                 `json:"email"      binding:"omitempty,email"`
	Phone       string                 `json:"phone"      binding:"omitempty"`
	Title       string                 `json:"title"      binding:"omitempty,max=200"`
	LinkedInURL string                 `json:"linkedin_url"`
	CompanyID   *uuid.UUID             `json:"company_id"`
	OwnerID     *uuid.UUID             `json:"owner_id"`
	Source      string                 `json:"source"`
	Tags        []string               `json:"tags"`
	CustomData  map[string]interface{} `json:"custom_data"`
}

type UpdateContactInput struct {
	FirstName   *string                `json:"first_name"`
	LastName    *string                `json:"last_name"`
	Email       *string                `json:"email"`
	Phone       *string                `json:"phone"`
	Title       *string                `json:"title"`
	LinkedInURL *string                `json:"linkedin_url"`
	CompanyID   *uuid.UUID             `json:"company_id"`
	OwnerID     *uuid.UUID             `json:"owner_id"`
	Tags        []string               `json:"tags"`
	CustomData  map[string]interface{} `json:"custom_data"`
}

type ContactService interface {
	Create(ctx context.Context, wsID uuid.UUID, in CreateContactInput) (*domain.Contact, error)
	Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Contact, error)
	List(ctx context.Context, wsID uuid.UUID, f repository.ContactFilter) ([]domain.Contact, int64, error)
	Update(ctx context.Context, wsID, id uuid.UUID, in UpdateContactInput) (*domain.Contact, error)
	Delete(ctx context.Context, wsID, id uuid.UUID) error
	UpdateAvatar(ctx context.Context, wsID, id uuid.UUID, avatarURL string) error
}

type contactService struct {
	repo         repository.ContactRepository
	auditor      repository.AuditRepository
	workerClient *worker.Client
	webhookRepo  repository.WebhookRepository
}

func NewContactService(
	repo repository.ContactRepository,
	auditor repository.AuditRepository,
	workerClient *worker.Client,
	webhookRepo repository.WebhookRepository,
) ContactService {
	return &contactService{
		repo:         repo,
		auditor:      auditor,
		workerClient: workerClient,
		webhookRepo:  webhookRepo,
	}
}

func (s *contactService) Create(ctx context.Context, wsID uuid.UUID, in CreateContactInput) (*domain.Contact, error) {
	if in.Email != "" {
		if existing, _ := s.repo.GetByEmail(ctx, wsID, strings.ToLower(in.Email)); existing != nil {
			return nil, ErrDuplicate
		}
	}
	c := &domain.Contact{
		WorkspaceID: wsID,
		FirstName:   in.FirstName,
		LastName:    in.LastName,
		Email:       strings.ToLower(in.Email),
		Phone:       in.Phone,
		Title:       in.Title,
		LinkedInURL: in.LinkedInURL,
		CompanyID:   in.CompanyID,
		OwnerID:     in.OwnerID,
		Source:      in.Source,
		Tags:        in.Tags,
	}
	if in.CustomData != nil {
		c.CustomData = domain.JSONB(in.CustomData)
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	
	if s.workerClient != nil {
		worker.FireEvent(ctx, s.workerClient, s.webhookRepo, wsID, "contact.created", c)
	}

	return c, nil
}

func (s *contactService) Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Contact, error) {
	c, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return c, nil
}

func (s *contactService) List(ctx context.Context, wsID uuid.UUID, f repository.ContactFilter) ([]domain.Contact, int64, error) {
	return s.repo.List(ctx, wsID, f)
}

func (s *contactService) Update(ctx context.Context, wsID, id uuid.UUID, in UpdateContactInput) (*domain.Contact, error) {
	c, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if in.FirstName != nil {
		c.FirstName = *in.FirstName
	}
	if in.LastName != nil {
		c.LastName = *in.LastName
	}
	if in.Email != nil {
		c.Email = strings.ToLower(*in.Email)
	}
	if in.Phone != nil {
		c.Phone = *in.Phone
	}
	if in.Title != nil {
		c.Title = *in.Title
	}
	if in.LinkedInURL != nil {
		c.LinkedInURL = *in.LinkedInURL
	}
	if in.CompanyID != nil {
		c.CompanyID = in.CompanyID
	}
	if in.OwnerID != nil {
		c.OwnerID = in.OwnerID
	}
	if in.Tags != nil {
		c.Tags = in.Tags
	}
	if in.CustomData != nil {
		c.CustomData = domain.JSONB(in.CustomData)
	}
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *contactService) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, wsID, id); err != nil {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, wsID, id)
}

func (s *contactService) UpdateAvatar(ctx context.Context, wsID, id uuid.UUID, avatarURL string) error {
	c, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return ErrNotFound
	}
	if c.CustomData == nil {
		c.CustomData = domain.JSONB{}
	}
	c.CustomData["avatar_url"] = avatarURL
	return s.repo.Update(ctx, c)
}

// ─── Deal Service ─────────────────────────────────────────────────────────────

type CreateDealInput struct {
	Title         string      `json:"title"          binding:"required,min=1,max=300"`
	PipelineID    uuid.UUID   `json:"pipeline_id"    binding:"required"`
	StageID       string      `json:"stage_id"       binding:"required"`
	Value         float64     `json:"value"`
	Currency      string      `json:"currency"`
	Probability   float64     `json:"probability"`
	ExpectedClose *time.Time  `json:"expected_close"`
	OwnerID       uuid.UUID   `json:"owner_id"`
	CompanyID     *uuid.UUID  `json:"company_id"`
	ContactIDs    []uuid.UUID `json:"contact_ids"`
}

type UpdateDealInput struct {
	Title         *string                `json:"title"`
	StageID       *string                `json:"stage_id"`
	Value         *float64               `json:"value"`
	Probability   *float64               `json:"probability"`
	ExpectedClose *time.Time             `json:"expected_close"`
	OwnerID       *uuid.UUID             `json:"owner_id"`
	CompanyID     *uuid.UUID             `json:"company_id"`
	CustomData    map[string]interface{} `json:"custom_data"`
}

type DealService interface {
	Create(ctx context.Context, wsID uuid.UUID, in CreateDealInput) (*domain.Deal, error)
	Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error)
	List(ctx context.Context, wsID uuid.UUID, f repository.DealFilter) ([]domain.Deal, int64, error)
	Update(ctx context.Context, wsID, id uuid.UUID, in UpdateDealInput) (*domain.Deal, error)
	Delete(ctx context.Context, wsID, id uuid.UUID) error
	MoveStage(ctx context.Context, wsID, id uuid.UUID, stageID string) (*domain.Deal, error)
	MarkWon(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error)
	MarkLost(ctx context.Context, wsID, id uuid.UUID, reason string) (*domain.Deal, error)
	AddContact(ctx context.Context, wsID, dealID, contactID uuid.UUID) error
	RemoveContact(ctx context.Context, wsID, dealID, contactID uuid.UUID) error
}

type dealService struct {
	repo    repository.DealRepository
	auditor repository.AuditRepository
}

func NewDealService(repo repository.DealRepository, auditor repository.AuditRepository) DealService {
	return &dealService{repo: repo, auditor: auditor}
}

func (s *dealService) Create(ctx context.Context, wsID uuid.UUID, in CreateDealInput) (*domain.Deal, error) {
	deal := &domain.Deal{
		WorkspaceID:   wsID,
		Title:         in.Title,
		PipelineID:    in.PipelineID,
		StageID:       in.StageID,
		Value:         in.Value,
		Currency:      in.Currency,
		Probability:   in.Probability,
		ExpectedClose: in.ExpectedClose,
		OwnerID:       in.OwnerID,
		CompanyID:     in.CompanyID,
		Status:        domain.DealStatusOpen,
	}
	if deal.Currency == "" {
		deal.Currency = "USD"
	}
	if err := s.repo.Create(ctx, deal); err != nil {
		return nil, err
	}
	for _, cID := range in.ContactIDs {
		_ = s.repo.AddContact(ctx, deal.ID, cID)
	}
	return deal, nil
}

func (s *dealService) Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error) {
	d, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return d, nil
}

func (s *dealService) List(ctx context.Context, wsID uuid.UUID, f repository.DealFilter) ([]domain.Deal, int64, error) {
	return s.repo.List(ctx, wsID, f)
}

func (s *dealService) Update(ctx context.Context, wsID, id uuid.UUID, in UpdateDealInput) (*domain.Deal, error) {
	d, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	if in.Title != nil {
		d.Title = *in.Title
	}
	if in.StageID != nil {
		d.StageID = *in.StageID
	}
	if in.Value != nil {
		d.Value = *in.Value
	}
	if in.Probability != nil {
		d.Probability = *in.Probability
	}
	if in.ExpectedClose != nil {
		d.ExpectedClose = in.ExpectedClose
	}
	if in.OwnerID != nil {
		d.OwnerID = *in.OwnerID
	}
	if in.CompanyID != nil {
		d.CompanyID = in.CompanyID
	}
	if in.CustomData != nil {
		d.CustomData = domain.JSONB(in.CustomData)
	}
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *dealService) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, wsID, id); err != nil {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, wsID, id)
}

func (s *dealService) MoveStage(ctx context.Context, wsID, id uuid.UUID, stageID string) (*domain.Deal, error) {
	d, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	d.StageID = stageID
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *dealService) MarkWon(ctx context.Context, wsID, id uuid.UUID) (*domain.Deal, error) {
	d, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	d.Status = domain.DealStatusWon
	d.ClosedAt = &now
	d.Probability = 100
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *dealService) MarkLost(ctx context.Context, wsID, id uuid.UUID, reason string) (*domain.Deal, error) {
	d, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	d.Status = domain.DealStatusLost
	d.ClosedAt = &now
	d.LostReason = reason
	d.Probability = 0
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *dealService) AddContact(ctx context.Context, wsID, dealID, contactID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, wsID, dealID); err != nil {
		return ErrNotFound
	}
	return s.repo.AddContact(ctx, dealID, contactID)
}

func (s *dealService) RemoveContact(ctx context.Context, wsID, dealID, contactID uuid.UUID) error {
	return s.repo.RemoveContact(ctx, dealID, contactID)
}

// ─── Activity Service ─────────────────────────────────────────────────────────

type CreateActivityInput struct {
	Type       domain.ActivityType `json:"type"        binding:"required"`
	Subject    string              `json:"subject"`
	Body       string              `json:"body"`
	ContactID  *uuid.UUID          `json:"contact_id"`
	DealID     *uuid.UUID          `json:"deal_id"`
	CompanyID  *uuid.UUID          `json:"company_id"`
	OccurredAt *time.Time          `json:"occurred_at"`
	Duration   int                 `json:"duration"`
}

type ActivityService interface {
	Create(ctx context.Context, wsID, userID uuid.UUID, in CreateActivityInput) (*domain.Activity, error)
	Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Activity, error)
	List(ctx context.Context, wsID uuid.UUID, f repository.ActivityFilter) ([]domain.Activity, int64, error)
	Update(ctx context.Context, wsID, id uuid.UUID, in CreateActivityInput) (*domain.Activity, error)
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type activityService struct{ repo repository.ActivityRepository }

func NewActivityService(repo repository.ActivityRepository) ActivityService {
	return &activityService{repo: repo}
}

func (s *activityService) Create(ctx context.Context, wsID, userID uuid.UUID, in CreateActivityInput) (*domain.Activity, error) {
	occurredAt := time.Now()
	if in.OccurredAt != nil {
		occurredAt = *in.OccurredAt
	}
	a := &domain.Activity{
		WorkspaceID: wsID,
		UserID:      userID,
		Type:        in.Type,
		Subject:     in.Subject,
		Body:        in.Body,
		ContactID:   in.ContactID,
		DealID:      in.DealID,
		CompanyID:   in.CompanyID,
		OccurredAt:  occurredAt,
		Duration:    in.Duration,
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *activityService) Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Activity, error) {
	a, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return a, nil
}

func (s *activityService) List(ctx context.Context, wsID uuid.UUID, f repository.ActivityFilter) ([]domain.Activity, int64, error) {
	return s.repo.List(ctx, wsID, f)
}

func (s *activityService) Update(ctx context.Context, wsID, id uuid.UUID, in CreateActivityInput) (*domain.Activity, error) {
	a, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	a.Type = in.Type
	a.Subject = in.Subject
	a.Body = in.Body
	a.Duration = in.Duration
	if in.OccurredAt != nil {
		a.OccurredAt = *in.OccurredAt
	}
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *activityService) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return s.repo.Delete(ctx, wsID, id)
}

// ─── Task Service ─────────────────────────────────────────────────────────────

type CreateTaskInput struct {
	Title       string              `json:"title"       binding:"required,min=1,max=500"`
	Description string              `json:"description"`
	Priority    domain.TaskPriority `json:"priority"`
	DueAt       *time.Time          `json:"due_at"`
	AssigneeID  uuid.UUID           `json:"assignee_id" binding:"required"`
	ContactID   *uuid.UUID          `json:"contact_id"`
	DealID      *uuid.UUID          `json:"deal_id"`
}

type TaskService interface {
	Create(ctx context.Context, wsID, createdBy uuid.UUID, in CreateTaskInput) (*domain.Task, error)
	Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error)
	List(ctx context.Context, wsID uuid.UUID, f repository.TaskFilter) ([]domain.Task, int64, error)
	Update(ctx context.Context, wsID, id uuid.UUID, in CreateTaskInput) (*domain.Task, error)
	Complete(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error)
	Delete(ctx context.Context, wsID, id uuid.UUID) error
}

type taskService struct{ repo repository.TaskRepository }

func NewTaskService(repo repository.TaskRepository) TaskService {
	return &taskService{repo: repo}
}

func (s *taskService) Create(ctx context.Context, wsID, createdBy uuid.UUID, in CreateTaskInput) (*domain.Task, error) {
	priority := in.Priority
	if priority == "" {
		priority = domain.TaskPriorityMedium
	}
	t := &domain.Task{
		WorkspaceID: wsID,
		Title:       in.Title,
		Description: in.Description,
		Priority:    priority,
		DueAt:       in.DueAt,
		AssigneeID:  in.AssigneeID,
		CreatedByID: createdBy,
		ContactID:   in.ContactID,
		DealID:      in.DealID,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *taskService) Get(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error) {
	t, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *taskService) List(ctx context.Context, wsID uuid.UUID, f repository.TaskFilter) ([]domain.Task, int64, error) {
	return s.repo.List(ctx, wsID, f)
}

func (s *taskService) Update(ctx context.Context, wsID, id uuid.UUID, in CreateTaskInput) (*domain.Task, error) {
	t, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	t.Title = in.Title
	t.Description = in.Description
	t.Priority = in.Priority
	t.DueAt = in.DueAt
	t.AssigneeID = in.AssigneeID
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *taskService) Complete(ctx context.Context, wsID, id uuid.UUID) (*domain.Task, error) {
	t, err := s.repo.GetByID(ctx, wsID, id)
	if err != nil {
		return nil, ErrNotFound
	}
	now := time.Now()
	t.CompletedAt = &now
	if err := s.repo.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *taskService) Delete(ctx context.Context, wsID, id uuid.UUID) error {
	return s.repo.Delete(ctx, wsID, id)
}

// ─── Upload Service ───────────────────────────────────────────────────────────
// Wraps Cloudinary and enforces workspace-scoped folder naming.

type UploadService interface {
	// UploadContactAvatar uploads an avatar for a contact and returns the secure URL.
	UploadContactAvatar(ctx context.Context, wsID uuid.UUID, contactID string, file interface{}, header interface{}) (string, error)
	// UploadCompanyLogo uploads a logo for a company and returns the secure URL.
	UploadCompanyLogo(ctx context.Context, wsID uuid.UUID, companyID string, file interface{}, header interface{}) (string, error)
	// UploadAttachment uploads any file attachment linked to a deal/contact.
	UploadAttachment(ctx context.Context, wsID uuid.UUID, entityID string, file interface{}, header interface{}) (string, error)
	// UploadWorkspaceLogo uploads a workspace logo.
	UploadWorkspaceLogo(ctx context.Context, wsID uuid.UUID, file interface{}, header interface{}) (string, error)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func validatePassword(p string) error {
	if len(p) < 8 {
		return ErrWeakPassword
	}
	var hasUpper, hasLower, hasDigit bool
	for _, ch := range p {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('-')
		}
	}
	slug := strings.Trim(b.String(), "-")
	token, _ := generateSecureToken(4)
	return slug + "-" + token
}

func generateSecureToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
