package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// userJSON renders a user without secrets; balances in both micro-USD and USD.
func userJSON(u *store.User) gin.H {
	return gin.H{
		"id":            u.ID,
		"email":         u.Email,
		"balance_micro": u.Balance,
		"balance_usd":   billing.USD(u.Balance),
		"enabled":       u.Enabled,
		"created_at":    u.CreatedAt,
	}
}

type createUserReq struct {
	Email             string  `json:"email"`
	InitialBalanceUSD float64 `json:"initial_balance_usd"`
}

// handleCreateUser creates a billing user, optionally funded on creation.
func (s *Server) handleCreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}
	if req.InitialBalanceUSD < 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "initial_balance_usd must be >= 0")
		return
	}
	u, err := s.store.CreateUser(c.Request.Context(), req.Email, billing.ToMicro(req.InitialBalanceUSD))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			abortWith(c, http.StatusConflict, "email_exists", "user with this email already exists")
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, userJSON(u))
}

// handleListUsers lists all billing users.
func (s *Server) handleListUsers(c *gin.Context) {
	users, err := s.store.ListUsers(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(users))
	for i := range users {
		data = append(data, userJSON(&users[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleGetUser fetches one user.
func (s *Server) handleGetUser(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	u, err := s.store.GetUser(c.Request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, userJSON(u))
}

type creditUserReq struct {
	AmountUSD float64 `json:"amount_usd"`
	Reason    string  `json:"reason"`
}

// handleCreditUser manually tops up a user's balance (ledger credit).
func (s *Server) handleCreditUser(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req creditUserReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AmountUSD <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "amount_usd > 0 is required")
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "manual top-up"
	}
	u, err := s.store.CreditUser(c.Request.Context(), id, billing.ToMicro(req.AmountUSD), reason)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, userJSON(u))
}

// handleUserLedger returns the wallet history for one user, newest first.
func (s *Server) handleUserLedger(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	limit, offset := parsePagination(c, 50, 200)
	entries, err := s.store.ListLedger(c.Request.Context(), id, limit, offset)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ledgerJSON(entries)})
}
