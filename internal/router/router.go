package router

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/yourorg/crm/internal/config"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/internal/handler"
	"github.com/yourorg/crm/internal/middleware"
	"github.com/yourorg/crm/internal/repository"
	"github.com/yourorg/crm/internal/service"
	"github.com/yourorg/crm/internal/worker"
	cloudinaryutil "github.com/yourorg/crm/pkg/cloudinary"
	"github.com/yourorg/crm/pkg/mailer"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client, workerClient *worker.Client, logger *zap.Logger) *gin.Engine {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	//  middleware 
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.RequestLogger(logger))
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.RateLimit(rdb, 300, time.Minute))


	cldClient, err := cloudinaryutil.New(
		cfg.CloudinaryCloudName,
		cfg.CloudinaryAPIKey,
		cfg.CloudinaryAPISecret,
	)
	if err != nil {
		logger.Warn("cloudinary not configured – file uploads disabled", zap.Error(err))
	}

	m := mailer.New(cfg.MailjetAPIKeyPublic, cfg.MailjetAPIKeyPrivate, cfg.MailFromEmail, cfg.MailFromName)

	//  Repositories 
	userRepo := repository.NewUserRepository(db)
	contactRepo := repository.NewContactRepository(db)
	companyRepo := repository.NewCompanyRepository(db)
	dealRepo := repository.NewDealRepository(db)
	activityRepo := repository.NewActivityRepository(db)
	taskRepo := repository.NewTaskRepository(db)
	pipelineRepo := repository.NewPipelineRepository(db)
	webhookRepo := repository.NewWebhookRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	notifRepo := repository.NewNotificationRepository(db)

	_ = companyRepo
	_ = pipelineRepo
	_ = webhookRepo
	_ = notifRepo

	//  Services 
	authSvc := service.NewAuthService(userRepo, cfg, m)
	contactSvc := service.NewContactService(contactRepo, auditRepo, workerClient, webhookRepo)
	dealSvc := service.NewDealService(dealRepo, auditRepo)
	activitySvc := service.NewActivityService(activityRepo)
	taskSvc := service.NewTaskService(taskRepo)

	var uploadSvc service.UploadService
	if cldClient != nil {
		uploadSvc = service.NewUploadService(cldClient)
	}

	//  Handlers 
	authH := handler.NewAuthHandler(authSvc)
	contactH := handler.NewContactHandler(contactSvc, uploadSvc)
	dealH := handler.NewDealHandler(dealSvc)
	activityH := handler.NewActivityHandler(activitySvc)
	taskH := handler.NewTaskHandler(taskSvc)
	var uploadH *handler.UploadHandler
	if uploadSvc != nil {
		uploadH = handler.NewUploadHandler(uploadSvc)
	}

	//  Health 
	r.GET("/health", handler.HealthHandler)
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pinggggg") })

	//  API v1 
	v1 := r.Group("/api/v1")

	// Auth (public)
	auth := v1.Group("/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
		auth.POST("/refresh", authH.Refresh)
		auth.POST("/forgot-password", authH.ForgotPassword)
		auth.POST("/reset-password", authH.ResetPassword)
	}

	// Protected routes
	protected := v1.Group("/")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))
	protected.Use(middleware.LoadUser(db))

	// Auth (protected)
	protectedAuth := protected.Group("/auth")
	{
		protectedAuth.POST("/logout", authH.Logout)
		protectedAuth.GET("/me", authH.Me)
		protectedAuth.POST("/change-password", authH.ChangePassword)
	}

	// Contacts
	importH := handler.NewImportHandler(workerClient)
	contacts := protected.Group("/contacts")
	{
		contacts.GET("", contactH.List)
		contacts.POST("", contactH.Create)
		contacts.POST("/import", importH.ImportContacts)
		contacts.GET("/:id", contactH.Get)
		contacts.PATCH("/:id", contactH.Update)
		contacts.DELETE("/:id", middleware.RequireRole(domain.RoleManager), contactH.Delete)
		if uploadH != nil {
			contacts.POST("/:id/avatar", contactH.UploadAvatar)
		}
	}
	// Deals
	deals := protected.Group("/deals")
	{
		deals.GET("", dealH.List)
		deals.POST("", dealH.Create)
		deals.GET("/:id", dealH.Get)
		deals.PATCH("/:id", dealH.Update)
		deals.DELETE("/:id", middleware.RequireRole(domain.RoleManager), dealH.Delete)
		deals.PATCH("/:id/stage", dealH.MoveStage)
		deals.POST("/:id/won", dealH.MarkWon)
		deals.POST("/:id/lost", dealH.MarkLost)
		deals.POST("/:id/contacts/:contact_id", dealH.AddContact)
		deals.DELETE("/:id/contacts/:contact_id", dealH.RemoveContact)
	}

	// Activities
	activities := protected.Group("/activities")
	{
		activities.GET("", activityH.List)
		activities.POST("", activityH.Create)
		activities.PUT("/:id", activityH.Update)
		activities.DELETE("/:id", activityH.Delete)
	}

	// Tasks
	tasks := protected.Group("/tasks")
	{
		tasks.GET("", taskH.List)
		tasks.POST("", taskH.Create)
		tasks.PUT("/:id", taskH.Update)
		tasks.POST("/:id/complete", taskH.Complete)
		tasks.DELETE("/:id", taskH.Delete)
	}



	// Pipelines
	pipelines := protected.Group("/pipelines")
	{
		pipelines.GET("", func(c *gin.Context) {
			wsID := middleware.WorkspaceIDFromCtx(c)
			var pipelines []domain.Pipeline
			db.Where("workspace_id = ?", wsID).Find(&pipelines)
			c.JSON(200, gin.H{"success": true, "data": pipelines})
		})
		pipelines.POST("", func(c *gin.Context) {
			wsID := middleware.WorkspaceIDFromCtx(c)
			var body struct {
				Name      string          `json:"name"`
				Stages    json.RawMessage `json:"stages"`
				IsDefault bool            `json:"is_default"`
			}
			c.ShouldBindJSON(&body)
			p := domain.Pipeline{
				ID:          uuid.New(),
				WorkspaceID: wsID,
				Name:        body.Name,
				Stages:      body.Stages,
				IsDefault:   body.IsDefault,
			}
			db.Create(&p)
			c.JSON(201, gin.H{"success": true, "data": p})
		})
		pipelines.GET("/:id", func(c *gin.Context) {
			wsID := middleware.WorkspaceIDFromCtx(c)
			var p domain.Pipeline
			db.Where("workspace_id = ? AND id = ?", wsID, c.Param("id")).First(&p)
			c.JSON(200, gin.H{"success": true, "data": p})
		})
		pipelines.DELETE("/:id", func(c *gin.Context) {
			wsID := middleware.WorkspaceIDFromCtx(c)
			db.Where("workspace_id = ? AND id = ?", wsID, c.Param("id")).Delete(&domain.Pipeline{})
			c.Status(204)
		})
	}

	if uploadH != nil {
		uploads := protected.Group("/uploads")
		{
			uploads.POST("/attachment", uploadH.UploadAttachment)
		}

		workspace := protected.Group("/workspace")
		{
			workspace.POST("/logo", middleware.RequireRole(domain.RoleAdmin), uploadH.UploadWorkspaceLogo)
		}
	}

	return r
}
