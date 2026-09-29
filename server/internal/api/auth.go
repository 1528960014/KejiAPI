package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"modelhub/internal/auth"
	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// ctxKeyUser is the gin context key holding the logged-in user.
const ctxKeyUser = "user"

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type credentialsReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

// tokensFor mints an access token and a stored single-use refresh token.
func (s *Server) tokensFor(ctx context.Context, u *store.User) (gin.H, error) {
	access, err := s.tokens.SignAccess(u.ID, u.Email)
	if err != nil {
		return nil, err
	}
	refresh, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateRefreshToken(ctx, u.ID, refresh, auth.RefreshTokenTTL); err != nil {
		return nil, err
	}
	return gin.H{
		"access_token":            access,
		"refresh_token":           refresh,
		"token_type":              "Bearer",
		"access_token_expires_in": int64(auth.AccessTokenTTL.Seconds()),
	}, nil
}

// handleRegister creates an account and returns a token pair.
func (s *Server) handleRegister(c *gin.Context) {
	var req credentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !emailRe.MatchString(req.Email) {
		abortWith(c, http.StatusBadRequest, "invalid_email", "email is invalid")
		return
	}
	if len(req.Password) < 8 {
		abortWith(c, http.StatusBadRequest, "weak_password", "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpErr(c, err)
		return
	}
	u, err := s.store.CreateAccount(c.Request.Context(), req.Email, hash)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			abortWith(c, http.StatusConflict, "email_exists", "user with this email already exists")
			return
		}
		httpErr(c, err)
		return
	}
	out, err := s.tokensFor(c.Request.Context(), u)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

// handleLogin verifies credentials and returns a token pair.
func (s *Server) handleLogin(c *gin.Context) {
	var req credentialsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	u, err := s.store.GetUserByEmail(c.Request.Context(), req.Email)
	if err != nil || u.PasswordHash == nil || !u.Enabled ||
		!auth.VerifyPassword(*u.PasswordHash, req.Password) {
		abortWith(c, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
		return
	}
	out, err := s.tokensFor(c.Request.Context(), u)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// handleRefresh exchanges a single-use refresh token for a new pair.
func (s *Server) handleRefresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	userID, err := s.store.RedeemRefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		abortWith(c, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
		return
	}
	u, err := s.store.GetUser(c.Request.Context(), userID)
	if err != nil || !u.Enabled {
		abortWith(c, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
		return
	}
	out, err := s.tokensFor(c.Request.Context(), u)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// handleLogout revokes a refresh token (idempotent).
func (s *Server) handleLogout(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	if err := s.store.RevokeRefreshToken(c.Request.Context(), req.RefreshToken); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// authJWT protects /api/me/* with a Bearer access token.
func (s *Server) authJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			abortWith(c, http.StatusUnauthorized, "invalid_token", "missing bearer token")
			return
		}
		claims, err := s.tokens.VerifyAccess(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			abortWith(c, http.StatusUnauthorized, "invalid_token", "token is invalid or expired")
			return
		}
		u, err := s.store.GetUser(c.Request.Context(), claims.UserID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				abortWith(c, http.StatusUnauthorized, "invalid_token", "user is not available")
				return
			}
			httpErr(c, err)
			return
		}
		if !u.Enabled {
			abortWith(c, http.StatusUnauthorized, "invalid_token", "user is disabled")
			return
		}
		c.Set(ctxKeyUser, u)
		c.Next()
	}
}

func userFrom(c *gin.Context) *store.User {
	if v, ok := c.Get(ctxKeyUser); ok {
		if u, ok := v.(*store.User); ok {
			return u
		}
	}
	return nil
}

// ledgerJSON renders ledger entries with USD conversions.
func ledgerJSON(entries []store.LedgerEntry) []gin.H {
	data := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		entry := gin.H{
			"id":         e.ID,
			"kind":       e.Kind,
			"amount":     e.Amount,
			"amount_usd": billing.USD(e.Amount),
			"reason":     e.Reason,
			"created_at": e.CreatedAt,
		}
		if e.RequestID != nil {
			entry["request_id"] = *e.RequestID
		}
		data = append(data, entry)
	}
	return data
}

// handleMe returns the logged-in user's profile.
func (s *Server) handleMe(c *gin.Context) {
	c.JSON(http.StatusOK, userJSON(userFrom(c)))
}

// handleMyLedger returns the user's own wallet history, newest first.
func (s *Server) handleMyLedger(c *gin.Context) {
	u := userFrom(c)
	limit, offset := parsePagination(c, 50, 200)
	entries, err := s.store.ListLedger(c.Request.Context(), u.ID, limit, offset)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ledgerJSON(entries)})
}

// handleMyKeys lists the user's API keys.
func (s *Server) handleMyKeys(c *gin.Context) {
	u := userFrom(c)
	keys, err := s.store.ListAPIKeysByUser(c.Request.Context(), u.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for i := range keys {
		data = append(data, keyJSON(&keys[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleCreateMyKey creates a key bound to the user.
func (s *Server) handleCreateMyKey(c *gin.Context) {
	u := userFrom(c)
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	plain, key, err := s.store.CreateAPIKeyForUser(c.Request.Context(), u.ID, req.Name)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": plain, "id": key.ID, "name": key.Name})
}

// handleDeleteMyKey deletes one of the user's own keys.
func (s *Server) handleDeleteMyKey(c *gin.Context) {
	u := userFrom(c)
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteAPIKeyByUser(c.Request.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
