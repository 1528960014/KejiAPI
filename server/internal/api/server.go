package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"modelhub/internal/auth"
	"modelhub/internal/config"
	"modelhub/internal/gateway"
	"modelhub/internal/store"
)

// ctxKeyAPIKey is the gin context key holding the resolved API key.
const ctxKeyAPIKey = "apiKey"

// Server bundles HTTP handlers with their dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	provider *gateway.Provider
	tokens   *auth.TokenService
}

// New builds the API server.
func New(cfg *config.Config, st *store.Store, p *gateway.Provider) *Server {
	return &Server{cfg: cfg, store: st, provider: p, tokens: auth.NewTokenService(cfg.MasterKey)}
}

// Engine wires all gin routes.
func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger())
	r.GET("/healthz", s.handleHealth)

	v1 := r.Group("/v1")
	v1.Use(s.authAPIKey())
	{
		v1.GET("/models", s.handleModels)
		v1.POST("/chat/completions", s.handleChatCompletions)
		v1.POST("/media/generate", s.handleMediaGenerate)
		v1.GET("/media/status/:task_id", s.handleMediaStatus)
	}

	admin := r.Group("/admin")
	admin.Use(s.authMasterKey())
	{
		admin.GET("/models", s.handleListModels)
		admin.POST("/models", s.handleCreateModel)
		admin.DELETE("/models/:id", s.handleDeleteModel)
		admin.GET("/channels", s.handleListChannels)
		admin.POST("/channels", s.handleCreateChannel)
		admin.DELETE("/channels/:id", s.handleDeleteChannel)
		admin.GET("/api-keys", s.handleListKeys)
		admin.POST("/api-keys", s.handleCreateKey)
		admin.PATCH("/api-keys/:id", s.handleUpdateKey)
		admin.DELETE("/api-keys/:id", s.handleDeleteKey)

		admin.GET("/users", s.handleListUsers)
		admin.POST("/users", s.handleCreateUser)
		admin.GET("/users/:id", s.handleGetUser)
		admin.POST("/users/:id/credit", s.handleCreditUser)
		admin.GET("/users/:id/ledger", s.handleUserLedger)

		admin.GET("/usage", s.handleListUsage)
		admin.GET("/usage/summary", s.handleUsageSummary)
		admin.GET("/tasks", s.handleListTasks)
	}

	api := r.Group("/api")
	{
		api.GET("/models", s.handlePublicModels)
	}

	authAPI := r.Group("/api/auth")
	{
		authAPI.POST("/register", s.handleRegister)
		authAPI.POST("/login", s.handleLogin)
		authAPI.POST("/refresh", s.handleRefresh)
		authAPI.POST("/logout", s.handleLogout)
	}

	me := r.Group("/api")
	me.Use(s.authJWT())
	{
		me.GET("/me", s.handleMe)
		me.GET("/me/ledger", s.handleMyLedger)
		me.GET("/me/keys", s.handleMyKeys)
		me.POST("/me/keys", s.handleCreateMyKey)
		me.DELETE("/me/keys/:id", s.handleDeleteMyKey)
	}
	return r
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start).String(),
			"client", c.ClientIP(),
		)
	}
}

func (s *Server) authAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer sk-") {
			abortWith(c, http.StatusUnauthorized, "invalid_api_key", "missing or malformed API key")
			return
		}
		plain := strings.TrimPrefix(header, "Bearer ")
		record, err := s.store.GetAPIKeyByHash(c.Request.Context(), store.HashKey(plain))
		if errors.Is(err, store.ErrNotFound) {
			abortWith(c, http.StatusUnauthorized, "invalid_api_key", "API key not recognized")
			return
		}
		if err != nil {
			httpErr(c, err)
			return
		}
		if record.ExpiresAt != nil && !record.ExpiresAt.After(time.Now()) {
			abortWith(c, http.StatusUnauthorized, "expired_api_key", "API key expired")
			return
		}
		c.Set(ctxKeyAPIKey, record)
		c.Next()
	}
}

func (s *Server) authMasterKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+s.cfg.MasterKey {
			abortWith(c, http.StatusForbidden, "forbidden", "invalid master key")
			return
		}
		c.Next()
	}
}

func (s *Server) apiKeyFrom(c *gin.Context) *store.APIKey {
	if v, ok := c.Get(ctxKeyAPIKey); ok {
		if k, ok := v.(*store.APIKey); ok {
			return k
		}
	}
	return nil
}

func abortWith(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": message, "type": code, "code": code}})
}

func httpErr(c *gin.Context, err error) {
	slog.Error("handler error", "path", c.Request.URL.Path, "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "internal error", "type": "server_error"}})
}
