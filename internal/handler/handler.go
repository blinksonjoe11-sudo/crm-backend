package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/middleware"
	"github.com/yourorg/crm/internal/repository"
	"github.com/yourorg/crm/internal/service"
)

// ─── response helpers ─────────────────────────────────────────────────────────

type Meta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": data})
}

func list(c *gin.Context, data interface{}, total int64, pg repository.Page) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"meta":    Meta{Page: pg.Page, PerPage: pg.PerPage, Total: total},
	})
}

func noContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func handleErr(c *gin.Context, err error) {
	switch err {
	case service.ErrNotFound:
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case service.ErrDuplicate:
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case service.ErrUnauthorized, service.ErrInvalidCredentials:
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case service.ErrForbidden:
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	case service.ErrWeakPassword:
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{"message": err.Error()}})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{"message": "internal server error"}})
	}
}

func bindErr(c *gin.Context, err error) {
	c.JSON(http.StatusUnprocessableEntity, gin.H{
		"success": false,
		"error":   gin.H{"code": "VALIDATION_ERROR", "message": err.Error()},
	})
}

// ─── Auth Handler ─────────────────────────────────────────────────────────────

type AuthHandler struct{ svc service.AuthService }

func NewAuthHandler(svc service.AuthService) *AuthHandler { return &AuthHandler{svc: svc} }

func (h *AuthHandler) Register(c *gin.Context) {
	var in service.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	result, refresh, err := h.svc.Register(c.Request.Context(), in)
	if err != nil {
		handleErr(c, err)
		return
	}
	setRefreshCookie(c, refresh)
	created(c, result)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var in service.LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	result, refresh, err := h.svc.Login(c.Request.Context(), in)
	if err != nil {
		handleErr(c, err)
		return
	}
	setRefreshCookie(c, refresh)
	ok(c, result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	token, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"message": "missing refresh token"}})
		return
	}
	result, newRefresh, err := h.svc.RefreshTokens(c.Request.Context(), token)
	if err != nil {
		handleErr(c, err)
		return
	}
	setRefreshCookie(c, newRefresh)
	ok(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token, _ := c.Cookie("refresh_token")
	if token != "" {
		_ = h.svc.Logout(c.Request.Context(), token)
	}
	c.SetCookie("refresh_token", "", -1, "/", "", true, true)
	noContent(c)
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID := middleware.UserIDFromCtx(c)
	user, err := h.svc.Me(c.Request.Context(), userID)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, user)
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var in struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	_ = h.svc.ForgotPassword(c.Request.Context(), in.Email)
	ok(c, gin.H{"message": "if this email exists, a reset link has been sent"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var in struct {
		Token    string `json:"token"    binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), in.Token, in.Password); err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"message": "password reset successful"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var in struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	userID := middleware.UserIDFromCtx(c)
	if err := h.svc.ChangePassword(c.Request.Context(), userID, in.OldPassword, in.NewPassword); err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"message": "password changed"})
}

func setRefreshCookie(c *gin.Context, token string) {
	c.SetCookie("refresh_token", token, 30*24*3600, "/api/v1/auth", "", true, true)
}

// ─── Contact Handler ──────────────────────────────────────────────────────────

type ContactHandler struct {
	svc    service.ContactService
	upload service.UploadService
}

func NewContactHandler(svc service.ContactService, upload service.UploadService) *ContactHandler {
	return &ContactHandler{svc: svc, upload: upload}
}

func (h *ContactHandler) List(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	var pg repository.Page
	_ = c.ShouldBindQuery(&pg)

	f := repository.ContactFilter{
		Search: c.Query("search"),
		Page:   pg,
	}
	if cid := c.Query("company_id"); cid != "" {
		id, _ := uuid.Parse(cid)
		f.CompanyID = &id
	}
	if oid := c.Query("owner_id"); oid != "" {
		id, _ := uuid.Parse(oid)
		f.OwnerID = &id
	}

	contacts, total, err := h.svc.List(c.Request.Context(), wsID, f)
	if err != nil {
		handleErr(c, err)
		return
	}
	list(c, contacts, total, pg)
}

func (h *ContactHandler) Get(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "invalid id"}})
		return
	}
	contact, err := h.svc.Get(c.Request.Context(), wsID, id)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, contact)
}

func (h *ContactHandler) Create(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	var in service.CreateContactInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	contact, err := h.svc.Create(c.Request.Context(), wsID, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	created(c, contact)
}

func (h *ContactHandler) Update(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in service.UpdateContactInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	contact, err := h.svc.Update(c.Request.Context(), wsID, id, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, contact)
}

func (h *ContactHandler) Delete(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), wsID, id); err != nil {
		handleErr(c, err)
		return
	}
	noContent(c)
}

// UploadAvatar — POST /contacts/:id/avatar  (multipart/form-data, field: "avatar")
func (h *ContactHandler) UploadAvatar(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id := c.Param("id")

	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "avatar file required"}})
		return
	}
	defer file.Close()

	url, err := h.upload.UploadContactAvatar(c.Request.Context(), wsID, id, file, header)
	if err != nil {
		handleErr(c, err)
		return
	}

	contactID, _ := uuid.Parse(id)
	if err := h.svc.UpdateAvatar(c.Request.Context(), wsID, contactID, url); err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"avatar_url": url})
}

// ─── Deal Handler ─────────────────────────────────────────────────────────────

type DealHandler struct{ svc service.DealService }

func NewDealHandler(svc service.DealService) *DealHandler { return &DealHandler{svc: svc} }

func (h *DealHandler) List(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	var pg repository.Page
	_ = c.ShouldBindQuery(&pg)

	f := repository.DealFilter{
		Search:  c.Query("search"),
		StageID: c.Query("stage_id"),
		Status:  domain.DealStatus(c.Query("status")),
		Page:    pg,
	}
	if pid := c.Query("pipeline_id"); pid != "" {
		id, _ := uuid.Parse(pid)
		f.PipelineID = &id
	}
	if oid := c.Query("owner_id"); oid != "" {
		id, _ := uuid.Parse(oid)
		f.OwnerID = &id
	}

	deals, total, err := h.svc.List(c.Request.Context(), wsID, f)
	if err != nil {
		handleErr(c, err)
		return
	}
	list(c, deals, total, pg)
}

func (h *DealHandler) Get(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	deal, err := h.svc.Get(c.Request.Context(), wsID, id)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, deal)
}

func (h *DealHandler) Create(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	userID := middleware.UserIDFromCtx(c)
	var in service.CreateDealInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	if in.OwnerID == uuid.Nil {
		in.OwnerID = userID
	}
	deal, err := h.svc.Create(c.Request.Context(), wsID, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	created(c, deal)
}

func (h *DealHandler) Update(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in service.UpdateDealInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	deal, err := h.svc.Update(c.Request.Context(), wsID, id, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, deal)
}

func (h *DealHandler) Delete(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), wsID, id); err != nil {
		handleErr(c, err)
		return
	}
	noContent(c)
}

func (h *DealHandler) MoveStage(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in struct {
		StageID string `json:"stage_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	deal, err := h.svc.MoveStage(c.Request.Context(), wsID, id, in.StageID)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, deal)
}

func (h *DealHandler) MarkWon(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	deal, err := h.svc.MarkWon(c.Request.Context(), wsID, id)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, deal)
}

func (h *DealHandler) MarkLost(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&in)
	deal, err := h.svc.MarkLost(c.Request.Context(), wsID, id, in.Reason)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, deal)
}

func (h *DealHandler) AddContact(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	dealID, _ := uuid.Parse(c.Param("id"))
	contactID, _ := uuid.Parse(c.Param("contact_id"))
	if err := h.svc.AddContact(c.Request.Context(), wsID, dealID, contactID); err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"message": "contact linked"})
}

func (h *DealHandler) RemoveContact(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	dealID, _ := uuid.Parse(c.Param("id"))
	contactID, _ := uuid.Parse(c.Param("contact_id"))
	if err := h.svc.RemoveContact(c.Request.Context(), wsID, dealID, contactID); err != nil {
		handleErr(c, err)
		return
	}
	noContent(c)
}

// ─── Activity Handler ─────────────────────────────────────────────────────────

type ActivityHandler struct{ svc service.ActivityService }

func NewActivityHandler(svc service.ActivityService) *ActivityHandler {
	return &ActivityHandler{svc: svc}
}

func (h *ActivityHandler) List(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	var pg repository.Page
	_ = c.ShouldBindQuery(&pg)
	f := repository.ActivityFilter{Page: pg}
	if cid := c.Query("contact_id"); cid != "" {
		id, _ := uuid.Parse(cid)
		f.ContactID = &id
	}
	if did := c.Query("deal_id"); did != "" {
		id, _ := uuid.Parse(did)
		f.DealID = &id
	}
	if t := c.Query("type"); t != "" {
		f.Type = domain.ActivityType(t)
	}
	activities, total, err := h.svc.List(c.Request.Context(), wsID, f)
	if err != nil {
		handleErr(c, err)
		return
	}
	list(c, activities, total, pg)
}

func (h *ActivityHandler) Create(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	userID := middleware.UserIDFromCtx(c)
	var in service.CreateActivityInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	a, err := h.svc.Create(c.Request.Context(), wsID, userID, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	created(c, a)
}

func (h *ActivityHandler) Update(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in service.CreateActivityInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	a, err := h.svc.Update(c.Request.Context(), wsID, id, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, a)
}

func (h *ActivityHandler) Delete(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), wsID, id); err != nil {
		handleErr(c, err)
		return
	}
	noContent(c)
}

// ─── Task Handler ─────────────────────────────────────────────────────────────

type TaskHandler struct{ svc service.TaskService }

func NewTaskHandler(svc service.TaskService) *TaskHandler { return &TaskHandler{svc: svc} }

func (h *TaskHandler) List(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	var pg repository.Page
	_ = c.ShouldBindQuery(&pg)
	f := repository.TaskFilter{Page: pg}
	if aid := c.Query("assignee_id"); aid != "" {
		id, _ := uuid.Parse(aid)
		f.AssigneeID = &id
	}
	if c.Query("overdue") == "true" {
		f.OverdueOnly = true
	}
	if comp := c.Query("completed"); comp != "" {
		b := comp == "true"
		f.Completed = &b
	}
	tasks, total, err := h.svc.List(c.Request.Context(), wsID, f)
	if err != nil {
		handleErr(c, err)
		return
	}
	list(c, tasks, total, pg)
}

func (h *TaskHandler) Create(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	userID := middleware.UserIDFromCtx(c)
	var in service.CreateTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	t, err := h.svc.Create(c.Request.Context(), wsID, userID, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	created(c, t)
}

func (h *TaskHandler) Update(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	var in service.CreateTaskInput
	if err := c.ShouldBindJSON(&in); err != nil {
		bindErr(c, err)
		return
	}
	t, err := h.svc.Update(c.Request.Context(), wsID, id, in)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, t)
}

func (h *TaskHandler) Complete(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	t, err := h.svc.Complete(c.Request.Context(), wsID, id)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, t)
}

func (h *TaskHandler) Delete(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	id, _ := uuid.Parse(c.Param("id"))
	if err := h.svc.Delete(c.Request.Context(), wsID, id); err != nil {
		handleErr(c, err)
		return
	}
	noContent(c)
}

// ─── Upload Handler ───────────────────────────────────────────────────────────

type UploadHandler struct{ svc service.UploadService }

func NewUploadHandler(svc service.UploadService) *UploadHandler { return &UploadHandler{svc: svc} }

// UploadAttachment — POST /uploads/attachment?entity_id=<uuid>
func (h *UploadHandler) UploadAttachment(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	entityID := c.Query("entity_id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "file field required"}})
		return
	}
	defer file.Close()

	url, err := h.svc.UploadAttachment(c.Request.Context(), wsID, entityID, file, header)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"url": url, "filename": header.Filename, "size": header.Size})
}

// UploadWorkspaceLogo — POST /workspace/logo
func (h *UploadHandler) UploadWorkspaceLogo(c *gin.Context) {
	wsID := middleware.WorkspaceIDFromCtx(c)
	file, header, err := c.Request.FormFile("logo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"message": "logo file required"}})
		return
	}
	defer file.Close()

	url, err := h.svc.UploadWorkspaceLogo(c.Request.Context(), wsID, file, header)
	if err != nil {
		handleErr(c, err)
		return
	}
	ok(c, gin.H{"logo_url": url})
}

// ─── Health Handler ───────────────────────────────────────────────────────────

func HealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
	})
}
