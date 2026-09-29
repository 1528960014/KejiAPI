package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// P3-3: multi-tenancy — organizations.
//
// An org is a tenant with a shared wallet, members (owner/admin/member) and
// org API keys that bill the org wallet at list price. Members manage the
// org through /api/me/orgs/* (JWT); the platform admin audits orgs and
// credits their wallets through /admin/organizations.

func orgJSON(o *store.Organization) gin.H {
	return gin.H{
		"id":             o.ID,
		"name":           o.Name,
		"owner_user_id":  o.OwnerUserID,
		"balance_micro":  o.BalanceMicro,
		"balance_usd":    billing.USD(o.BalanceMicro),
		"created_at":     o.CreatedAt,
	}
}

func orgMemberJSON(m *store.OrgMember) gin.H {
	return gin.H{
		"user_id":   m.UserID,
		"email":     m.Email,
		"role":      m.Role,
		"joined_at": m.JoinedAt,
	}
}

func orgKeyJSON(k *store.APIKey) gin.H {
	item := gin.H{
		"id":             k.ID,
		"name":           k.Name,
		"allowed_models": k.AllowedModels,
		"spend_micro":    k.Spend,
		"spend_usd":      billing.USD(k.Spend),
		"created_at":     k.CreatedAt,
	}
	if k.Quota != nil {
		item["quota_usd"] = billing.USD(*k.Quota)
	}
	if k.ExpiresAt != nil {
		item["expires_at"] = *k.ExpiresAt
	}
	return item
}

// requireOrgMember resolves the :id org and the caller's membership,
// aborting with 404 when either is missing.
func (s *Server) requireOrgMember(c *gin.Context) (*store.Organization, *store.OrgMember, bool) {
	u := userFrom(c)
	orgID, err := parseIDParam(c)
	if err != nil {
		return nil, nil, false
	}
	org, err := s.store.GetOrganization(c.Request.Context(), orgID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "org_not_found", "organization not found")
		return nil, nil, false
	}
	if err != nil {
		httpErr(c, err)
		return nil, nil, false
	}
	m, err := s.store.GetOrgMember(c.Request.Context(), orgID, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "org_not_found", "organization not found")
		return nil, nil, false
	}
	if err != nil {
		httpErr(c, err)
		return nil, nil, false
	}
	return org, m, true
}

// requireOrgAdmin is requireOrgMember plus an admin-or-owner role check.
func (s *Server) requireOrgAdmin(c *gin.Context) (*store.Organization, *store.OrgMember, bool) {
	org, m, ok := s.requireOrgMember(c)
	if !ok {
		return nil, nil, false
	}
	if m.Role != store.OrgRoleOwner && m.Role != store.OrgRoleAdmin {
		abortWith(c, http.StatusForbidden, "forbidden", "org admin role required")
		return nil, nil, false
	}
	return org, m, true
}

// --- member-facing ---

// handleListMyOrgs lists the caller's orgs with their role.
func (s *Server) handleListMyOrgs(c *gin.Context) {
	u := userFrom(c)
	list, err := s.store.ListOrganizationsByUser(c.Request.Context(), u.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		item := orgJSON(&list[i].Organization)
		item["role"] = list[i].Role
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleCreateOrg creates an org; the caller becomes its owner.
func (s *Server) handleCreateOrg(c *gin.Context) {
	u := userFrom(c)
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Name) < 2 || len(req.Name) > 64 {
		abortWith(c, http.StatusBadRequest, "invalid_name", "name must be 2-64 characters")
		return
	}
	org, err := s.store.CreateOrganization(c.Request.Context(), req.Name, u.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, orgJSON(org))
}

// handleGetOrg returns one org plus its member list (members only).
func (s *Server) handleGetOrg(c *gin.Context) {
	org, _, ok := s.requireOrgMember(c)
	if !ok {
		return
	}
	members, err := s.store.ListOrgMembers(c.Request.Context(), org.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(members))
	for i := range members {
		data = append(data, orgMemberJSON(&members[i]))
	}
	out := orgJSON(org)
	out["members"] = data
	c.JSON(http.StatusOK, out)
}

// handleAddOrgMember invites a user by email (admin+).
func (s *Server) handleAddOrgMember(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}
	if req.Role == "" {
		req.Role = store.OrgRoleMember
	}
	if req.Role != store.OrgRoleAdmin && req.Role != store.OrgRoleMember {
		abortWith(c, http.StatusBadRequest, "invalid_role", "role must be admin or member")
		return
	}
	m, err := s.store.AddOrgMember(c.Request.Context(), org.ID, req.Email, req.Role)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusBadRequest, "user_not_found", "no user with that email")
		return
	}
	if errors.Is(err, store.ErrOrgMemberExists) {
		abortWith(c, http.StatusConflict, "member_exists", "user is already a member")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, orgMemberJSON(m))
}

// handleSetOrgMemberRole changes a member's role (admin+).
func (s *Server) handleSetOrgMemberRole(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	targetID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_id", "user_id must be an integer")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "role is required")
		return
	}
	if req.Role != store.OrgRoleAdmin && req.Role != store.OrgRoleMember {
		abortWith(c, http.StatusBadRequest, "invalid_role", "role must be admin or member")
		return
	}
	if err := s.store.SetOrgMemberRole(c.Request.Context(), org.ID, targetID, req.Role); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			abortWith(c, http.StatusNotFound, "member_not_found", "member not found")
			return
		}
		abortWith(c, http.StatusBadRequest, "invalid_role", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleRemoveOrgMember removes a member (admin+, or a member leaving
// themselves; the owner can never be removed).
func (s *Server) handleRemoveOrgMember(c *gin.Context) {
	org, me, ok := s.requireOrgMember(c)
	if !ok {
		return
	}
	targetID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_id", "user_id must be an integer")
		return
	}
	isSelf := targetID == me.UserID
	if !isSelf && me.Role != store.OrgRoleOwner && me.Role != store.OrgRoleAdmin {
		abortWith(c, http.StatusForbidden, "forbidden", "org admin role required")
		return
	}
	if err := s.store.RemoveOrgMember(c.Request.Context(), org.ID, targetID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			abortWith(c, http.StatusNotFound, "member_not_found", "member not found")
			return
		}
		abortWith(c, http.StatusBadRequest, "cannot_remove", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleListOrgKeys lists the org's keys (any member).
func (s *Server) handleListOrgKeys(c *gin.Context) {
	org, _, ok := s.requireOrgMember(c)
	if !ok {
		return
	}
	keys, err := s.store.ListOrgKeys(c.Request.Context(), org.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for i := range keys {
		data = append(data, orgKeyJSON(&keys[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleCreateOrgKey creates an org key (admin+); the plain value is
// returned exactly once.
func (s *Server) handleCreateOrgKey(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	var req struct {
		Name          string   `json:"name"`
		AllowedModels []string `json:"allowed_models"`
		QuotaUSD      *float64 `json:"quota_usd"`
		ExpiresAt     *string  `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var quota *int64
	if req.QuotaUSD != nil && *req.QuotaUSD > 0 {
		q := int64(*req.QuotaUSD * 1_000_000)
		quota = &q
	}
	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_expires", "expires_at must be RFC3339")
			return
		}
		expiresAt = &t
	}
	plain, key, err := s.store.CreateOrgKey(c.Request.Context(), org.ID, req.Name, req.AllowedModels, quota, expiresAt)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"key":  plain,
		"org":  orgKeyJSON(key),
	})
}

// handleDeleteOrgKey removes an org key (admin+).
func (s *Server) handleDeleteOrgKey(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	keyID, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteOrgKey(c.Request.Context(), org.ID, keyID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			abortWith(c, http.StatusNotFound, "key_not_found", "org key not found")
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleOrgUsage returns the org's usage summary + recent rows (admin+).
func (s *Server) handleOrgUsage(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	limit, _ := parsePagination(c, 50, 200)
	summary, err := s.store.OrgUsageSummary(c.Request.Context(), org.ID)
	if err != nil {
		httpErr(c, err)
		return
	}
	rows, err := s.store.ListOrgUsage(c.Request.Context(), org.ID, limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		data = append(data, gin.H{
			"id":                  r.ID,
			"api_key_id":          r.APIKeyID,
			"key_name":            r.KeyName,
			"model_id":            r.ModelID,
			"provider":            r.Provider,
			"prompt_tokens":       r.PromptTokens,
			"completion_tokens":   r.CompletionTokens,
			"cost_usd":            r.Cost,
			"status":              r.Status,
			"created_at":          r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"requests":          summary.Requests,
			"prompt_tokens":     summary.PromptTokens,
			"completion_tokens": summary.CompletionTokens,
			"cost_usd":          summary.CostUSD,
		},
		"data": data,
	})
}

// handleOrgLedger returns the org wallet ledger (admin+).
func (s *Server) handleOrgLedger(c *gin.Context) {
	org, _, ok := s.requireOrgAdmin(c)
	if !ok {
		return
	}
	limit, _ := parsePagination(c, 50, 200)
	list, err := s.store.ListOrgLedger(c.Request.Context(), org.ID, limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		e := &list[i]
		item := gin.H{
			"id":         e.ID,
			"kind":       e.Kind,
			"amount":     e.Amount,
			"amount_usd": billing.USD(e.Amount),
			"reason":     e.Reason,
			"created_at": e.CreatedAt,
		}
		if e.RequestID != nil {
			item["request_id"] = *e.RequestID
		}
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// --- admin ---

// handleAdminListOrgs lists all orgs (platform admin).
func (s *Server) handleAdminListOrgs(c *gin.Context) {
	limit, _ := parsePagination(c, 50, 200)
	list, err := s.store.AdminListOrganizations(c.Request.Context(), limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		item := orgJSON(&list[i].Organization)
		item["owner_email"] = list[i].OwnerEmail
		item["member_count"] = list[i].MemberCount
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleAdminCreditOrg adds balance to an org wallet (platform admin).
func (s *Server) handleAdminCreditOrg(c *gin.Context) {
	orgID, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req struct {
		AmountUSD float64 `json:"amount_usd"`
		Reason    string  `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AmountUSD <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "amount_usd > 0 is required")
		return
	}
	if req.Reason == "" {
		req.Reason = "admin credit"
	}
	amountMicro := int64(req.AmountUSD * 1_000_000)
	org, err := s.store.CreditOrg(c.Request.Context(), orgID, amountMicro, req.Reason)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, orgJSON(org))
}