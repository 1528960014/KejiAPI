package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/auth"
	"kejiapi/internal/config"
	"kejiapi/internal/gateway"
	"kejiapi/internal/store"
)

// ctxKeyAPIKey is the gin context key holding the resolved API key.
const ctxKeyAPIKey = "apiKey"

// Server bundles HTTP handlers with their dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	provider *gateway.Provider
	tokens   *auth.TokenService
	payMu    sync.RWMutex
	payState payConfigState         // P3-2: hot-swappable payment config
	ipMu     sync.RWMutex
	ipRules  []store.IPRule         // gateway IP blacklist/whitelist cache
	limiter  *rateLimiter           // P4-3: per-key RPM/TPM (nil when disabled)
	conc     *gateway.ConcurrencyLimiter // P7-3: per-key / per-channel concurrency (nil when disabled)
	health   *gateway.ChannelHealth // P5-1/P6-1: channel failure cooldown,
	// shared with the task workers so one failure cools down everywhere
	// P5-1: *store.Store in production; swappable in unit tests.
	channelSource channelSource
}

// New builds the API server. The payment state starts from the environment;
// call ReloadPayConfig to seed the database and switch to it (hot-reloadable
// via /admin/pay-config afterwards). health is the shared channel-health
// tracker (also used by the task workers).
func New(cfg *config.Config, st *store.Store, p *gateway.Provider, health *gateway.ChannelHealth) *Server {
	s := &Server{
		cfg:           cfg,
		store:         st,
		provider:      p,
		tokens:        auth.NewTokenService(cfg.MasterKey),
		payState:      envPayState(cfg),
		health:        health,
		channelSource: st,
	}
	if cfg.RPM > 0 || cfg.TPM > 0 {
		s.limiter = newRateLimiter(cfg.RPM, cfg.TPM)
	}
	if cfg.ConcPerKey > 0 || cfg.ConcPerChannel > 0 {
		s.conc = gateway.NewConcurrencyLimiter(cfg.ConcPerKey, cfg.ConcPerChannel)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rules, err := st.ListIPRules(ctx); err == nil {
		s.ipRules = rules
	}
	return s
}

// Engine wires all gin routes.
func (s *Server) Engine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger())
	r.GET("/healthz", s.handleHealth)

	v1 := r.Group("/v1")
	v1.Use(s.ipGate(), s.authAPIKey())
	{
		v1.GET("/models", s.handleModels)
		v1.POST("/chat/completions", s.handleChatCompletions)
		v1.GET("/agents", s.handleListChatAgents)
		v1.POST("/media/generate", s.handleMediaGenerate)
		v1.GET("/media/status/:task_id", s.handleMediaStatus)
		v1.POST("/drama/generate", s.handleDramaGenerate)
		v1.GET("/drama/status/:drama_id", s.handleDramaStatus)

		// P3-4: real-time speech (OpenAI-compatible synchronous TTS).
		v1.POST("/audio/speech", s.handleAudioSpeech)
	}

	// P4-1: bidirectional real-time voice (OpenAI Realtime WS pass-through).
	// Outside the v1 group: auth is via the Sec-WebSocket-Protocol subprotocol,
	// not the Bearer header.
	r.POST("/v1/realtime", s.handleRealtime)

	admin := r.Group("/admin")
	admin.Use(s.authMasterKey(), s.auditLog())
	{
		admin.GET("/ops", s.handleOps)
		admin.GET("/audit-logs", s.handleListAuditLogs)

		// IP access control (blacklist / whitelist).
		admin.GET("/ip-rules", s.handleListIPRules)
		admin.POST("/ip-rules", s.handleCreateIPRule)
		admin.PATCH("/ip-rules/:id", s.handleUpdateIPRule)
		admin.DELETE("/ip-rules/:id", s.handleDeleteIPRule)

		admin.GET("/models", s.handleListModels)
		admin.POST("/models", s.handleCreateModel)
		admin.PATCH("/models/:id", s.handleUpdateModel)
		admin.DELETE("/models/:id", s.handleDeleteModel)
		admin.GET("/channels", s.handleListChannels)
		admin.POST("/channels", s.handleCreateChannel)
		admin.PATCH("/channels/:id", s.handleUpdateChannel)
		admin.POST("/channels/:id/test", s.handleTestChannel)
		admin.POST("/channels/probe-all", s.handleProbeAllChannels)
		admin.DELETE("/channels/:id", s.handleDeleteChannel)
		admin.GET("/api-keys", s.handleListKeys)
		admin.POST("/api-keys", s.handleCreateKey)
		admin.PATCH("/api-keys/:id", s.handleUpdateKey)
		admin.DELETE("/api-keys/:id", s.handleDeleteKey)

		admin.GET("/users", s.handleListUsers)
		admin.POST("/users", s.handleCreateUser)
		admin.PATCH("/users/:id", s.handleUpdateUser)
		admin.GET("/users/:id", s.handleGetUser)
		admin.POST("/users/:id/credit", s.handleCreditUser)
		admin.GET("/users/:id/ledger", s.handleUserLedger)

		admin.GET("/usage", s.handleListUsage)
		admin.GET("/usage/summary", s.handleUsageSummary)
		admin.GET("/usage/daily", s.handleUsageDaily)
		admin.GET("/tasks", s.handleListTasks)
		admin.GET("/dramas", s.handleListDramas)
		admin.GET("/recharges", s.handleAdminRecharges)

		// P2-3: reseller agents (wholesale rate on a billing user).
		admin.GET("/agents", s.handleListAgents)
		admin.POST("/agents", s.handleCreateAgent)
		admin.PUT("/agents/:id", s.handleUpdateAgent)
		admin.DELETE("/agents/:id", s.handleDeleteAgent)

		// P2-2: chat agent templates (predefined assistants).
		admin.GET("/assistants", s.handleListAssistants)
		admin.POST("/assistants", s.handleCreateAssistant)
		admin.PATCH("/assistants/:id", s.handleUpdateAssistant)
		admin.DELETE("/assistants/:id", s.handleDeleteAssistant)

		// P3-2: payment channel configuration (hot reload, no restart).
		admin.GET("/pay-config", s.handleGetPayConfig)
		admin.PUT("/pay-config", s.handlePutPayConfig)

		// P3-3: organizations (platform audit + wallet credit).
		admin.GET("/organizations", s.handleAdminListOrgs)
		admin.POST("/organizations/:id/credit", s.handleAdminCreditOrg)

		// P7-6: external systems (iframe-embedded admin extensions).
		admin.GET("/external-pages", s.handleListExternalPages)
		admin.POST("/external-pages", s.handleCreateExternalPage)
		admin.PATCH("/external-pages/:id", s.handleUpdateExternalPage)
		admin.DELETE("/external-pages/:id", s.handleDeleteExternalPage)

		// P0-1: terms of service.
		admin.GET("/terms", s.handleAdminGetTerms)
		admin.PUT("/terms", s.handleAdminPutTerms)

		// P0-2: announcements.
		admin.GET("/announcements", s.handleAdminListAnnouncements)
		admin.POST("/announcements", s.handleAdminCreateAnnouncement)
		admin.PATCH("/announcements/:id", s.handleAdminUpdateAnnouncement)
		admin.DELETE("/announcements/:id", s.handleAdminDeleteAnnouncement)

		// P0-3: redeem codes.
		admin.GET("/redeem-codes", s.handleAdminListRedeemCodes)
		admin.POST("/redeem-codes", s.handleAdminCreateRedeemCodes)
		admin.DELETE("/redeem-codes/:id", s.handleAdminDeleteRedeemCode)

		// P0-4: promo codes.
		admin.GET("/promo-codes", s.handleAdminListPromos)
		admin.POST("/promo-codes", s.handleAdminCreatePromo)
		admin.PATCH("/promo-codes/:id", s.handleAdminUpdatePromo)
		admin.DELETE("/promo-codes/:id", s.handleAdminDeletePromo)

		// P0-5: subscription plans + per-user subscriptions.
		admin.GET("/plans", s.handleAdminListPlans)
		admin.POST("/plans", s.handleAdminCreatePlan)
		admin.PATCH("/plans/:id", s.handleAdminUpdatePlan)
		admin.DELETE("/plans/:id", s.handleAdminDeletePlan)
		admin.POST("/users/:id/subscription", s.handleAdminSetUserSubscription)
		admin.GET("/subscriptions", s.handleAdminListSubscriptions)

		// P0-6: recharge analytics + refunds.
		admin.GET("/recharge-stats", s.handleAdminRechargeStats)
		admin.POST("/recharges/:id/refund", s.handleAdminRefundRecharge)
	}

	api := r.Group("/api")
	{
		api.GET("/models", s.handlePublicModels)
		api.GET("/rankings", s.handlePublicRankings)
		// P0: public console content.
		api.GET("/terms", s.handleGetTerms)
		api.GET("/announcements", s.handlePublicAnnouncements)
		api.POST("/announcements/:id/view", s.handleAnnouncementView)
	}

	// P2-1b: composed drama videos. Public by design: the drama UUID (32 hex
	// chars) is the unguessable token.
	r.GET("/media/dramas/:uuid", s.handleDramaVideo)

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
		// P2-3: reseller subkeys (agent accounts only).
		me.GET("/me/subkeys", s.handleListSubkeys)
		me.POST("/me/subkeys", s.handleCreateSubkey)
		me.DELETE("/me/subkeys/:id", s.handleDeleteSubkey)

		// P3-3: organizations (multi-tenancy).
		me.GET("/me/orgs", s.handleListMyOrgs)
		me.POST("/me/orgs", s.handleCreateOrg)
		me.GET("/me/orgs/:id", s.handleGetOrg)
		me.POST("/me/orgs/:id/members", s.handleAddOrgMember)
		me.PUT("/me/orgs/:id/members/:user_id", s.handleSetOrgMemberRole)
		me.DELETE("/me/orgs/:id/members/:user_id", s.handleRemoveOrgMember)
		me.GET("/me/orgs/:id/keys", s.handleListOrgKeys)
		me.POST("/me/orgs/:id/keys", s.handleCreateOrgKey)
		me.DELETE("/me/orgs/:id/keys/:key_id", s.handleDeleteOrgKey)
		me.GET("/me/orgs/:id/usage", s.handleOrgUsage)
		me.GET("/me/orgs/:id/ledger", s.handleOrgLedger)
	}

	// P2-4: online recharge (JWT user).
	{
		me.GET("/me/recharge/config", s.handleRechargeConfig)
		me.POST("/me/recharges", s.handleCreateRecharge)
		me.GET("/me/recharges", s.handleMyRecharges)
		me.GET("/me/recharges/:id", s.handleGetMyRecharge)

		// P0: redeem code + subscription (JWT user).
		me.POST("/me/redeem", s.handleMyRedeem)
		me.GET("/me/subscription", s.handleMySubscription)
	}

	// P2-4: public payment channel callbacks (signature-verified, no auth).
	// yipay notify may arrive as GET or POST; alipay/wechat POST.
	for _, m := range []string{"yipay", "alipay", "wechat"} {
		method := m
		r.GET("/pay/notify/"+method, s.handlePayNotify(method))
		r.POST("/pay/notify/"+method, s.handlePayNotify(method))
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
