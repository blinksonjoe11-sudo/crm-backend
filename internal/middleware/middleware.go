package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/yourorg/crm/internal/domain"
	"github.com/yourorg/crm/pkg/jwtutil"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	KeyUserID      = "userID"
	KeyWorkspaceID = "workspaceID"
	KeyRole        = "role"
	KeyUser        = "user"
	KeyWorkspace   = "workspace"
)

// ─── Context helpers 

func UserIDFromCtx(c *gin.Context) uuid.UUID {
	id, _ := c.Get(KeyUserID)
	return id.(uuid.UUID)
}

func WorkspaceIDFromCtx(c *gin.Context) uuid.UUID {
	id, _ := c.Get(KeyWorkspaceID)
	return id.(uuid.UUID)
}

func RoleFromCtx(c *gin.Context) domain.Role {
	r, _ := c.Get(KeyRole)
	return r.(domain.Role)
}

func UserFromCtx(c *gin.Context) *domain.User {
	u, _ := c.Get(KeyUser)
	return u.(*domain.User)
}

// ─── JWT Auth ─────────────────────────────────────────────────────────────────

func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("missing authorization header"))
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("invalid authorization format"))
			return
		}

		claims, err := jwtutil.ParseAccess(parts[1], secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp(err.Error()))
			return
		}

		c.Set(KeyUserID, claims.UserID)
		c.Set(KeyWorkspaceID, claims.WorkspaceID)
		c.Set(KeyRole, claims.Role)
		c.Next()
	}
}

// ─── Load Workspace & User ────────────────────────────────────────────────────

func LoadUser(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := UserIDFromCtx(c)
		var user domain.User
		if err := db.Preload("Workspace").Where("id = ? AND is_active = true", userID).First(&user).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("user not found or inactive"))
			return
		}
		c.Set(KeyUser, &user)
		c.Next()
	}
}

// ─── RBAC ─────────────────────────────────────────────────────────────────────

// roleRank maps each role to a numeric level. Higher = more permissions.
var roleRank = map[domain.Role]int{
	domain.RoleOwner:    100,
	domain.RoleAdmin:    80,
	domain.RoleManager:  60,
	domain.RoleSalesRep: 40,
	domain.RoleReadOnly: 20,
}

// RequireRole allows access only if the user's role rank >= the minimum required role.
func RequireRole(minimum domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := RoleFromCtx(c)
		if roleRank[role] < roleRank[minimum] {
			c.AbortWithStatusJSON(http.StatusForbidden, errResp("insufficient permissions"))
			return
		}
		c.Next()
	}
}

// ─── Rate Limiter ─────────────────────────────────────────────────────────────

func RateLimit(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "rl:" + c.ClientIP()
		ctx := c.Request.Context()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			c.Next()
			return
		}
		if count == 1 {
			rdb.Expire(ctx, key, window)
		}
		if int(count) > limit {
			c.Header("Retry-After", window.String())
			c.AbortWithStatusJSON(http.StatusTooManyRequests, errResp("rate limit exceeded"))
			return
		}
		c.Next()
	}
}

// ─── CORS ─────────────────────────────────────────────────────────────────────

func CORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := false
		for _, o := range allowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ─── Request Logger ───────────────────────────────────────────────────────────

func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// ─── Recovery ────────────────────────────────────────────────────────────────

func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered", zap.Any("error", r))
				c.AbortWithStatusJSON(http.StatusInternalServerError, errResp("internal server error"))
			}
		}()
		c.Next()
	}
}

// ─── Security Headers ─────────────────────────────────────────────────────────

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func errResp(msg string) gin.H {
	return gin.H{"success": false, "error": gin.H{"message": msg}}
}
