package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"kejiapi/internal/billing"
	"kejiapi/internal/store"
)

// P0 batch endpoints: terms, announcements, redeem codes, promo codes,
// subscription plans, recharge stats and refunds.

// ---------- terms ----------

func (s *Server) handleGetTerms(c *gin.Context) {
	t, err := s.store.GetTerms(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": t.Content, "updated_at": t.UpdatedAt})
}

func (s *Server) handleAdminGetTerms(c *gin.Context) {
	t, err := s.store.GetTerms(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": t.Content, "updated_at": t.UpdatedAt})
}

type putTermsReq struct {
	Content string `json:"content"`
}

func (s *Server) handleAdminPutTerms(c *gin.Context) {
	var req putTermsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "content is required")
		return
	}
	t, err := s.store.PutTerms(c.Request.Context(), req.Content)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": t.Content, "updated_at": t.UpdatedAt})
}

// ---------- announcements ----------

func announcementJSON(a *store.Announcement) gin.H {
	return gin.H{
		"id":         a.ID,
		"title":      a.Title,
		"content":    a.Content,
		"style":      a.Style,
		"active":     a.Active,
		"views":      a.Views,
		"created_at": a.CreatedAt,
	}
}

func (s *Server) handlePublicAnnouncements(c *gin.Context) {
	list, err := s.store.ListActiveAnnouncements(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, announcementJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type announcementReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Style   string `json:"style"`
	Active  *bool  `json:"active"`
}

func normalizeAnnouncementStyle(style string) string {
	if style == "banner" {
		return "banner"
	}
	return "popup"
}

func (s *Server) handleAdminListAnnouncements(c *gin.Context) {
	list, err := s.store.ListAnnouncements(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, announcementJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (s *Server) handleAdminCreateAnnouncement(c *gin.Context) {
	var req announcementReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	a, err := s.store.CreateAnnouncement(c.Request.Context(), req.Title, req.Content, normalizeAnnouncementStyle(req.Style))
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, announcementJSON(a))
}

func (s *Server) handleAdminUpdateAnnouncement(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req announcementReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	if err := s.store.UpdateAnnouncement(c.Request.Context(), id, req.Title, req.Content, normalizeAnnouncementStyle(req.Style), active); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminDeleteAnnouncement(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteAnnouncement(c.Request.Context(), id); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleAnnouncementView records a view for a public announcement.
func (s *Server) handleAnnouncementView(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.BumpAnnouncementView(c.Request.Context(), id); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- redeem codes ----------

func redeemCodeJSON(r *store.RedeemCode) gin.H {
	return gin.H{
		"id":           r.ID,
		"code":         r.Code,
		"credit_micro": r.CreditMicro,
		"credit_usd":   billing.USD(r.CreditMicro),
		"used_by":      r.UsedBy,
		"used_at":      r.UsedAt,
		"created_at":   r.CreatedAt,
	}
}

func (s *Server) handleAdminListRedeemCodes(c *gin.Context) {
	list, err := s.store.ListRedeemCodes(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, redeemCodeJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type createRedeemCodesReq struct {
	Count        int     `json:"count"`
	CreditUSD    float64 `json:"credit_usd"`
}

func (s *Server) handleAdminCreateRedeemCodes(c *gin.Context) {
	var req createRedeemCodesReq
	if err := c.ShouldBindJSON(&req); err != nil || req.CreditUSD <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "credit_usd > 0 is required")
		return
	}
	list, err := s.store.CreateRedeemCodes(c.Request.Context(), req.Count, billing.ToMicro(req.CreditUSD))
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, redeemCodeJSON(&list[i]))
	}
	c.JSON(http.StatusCreated, gin.H{"data": data})
}

func (s *Server) handleAdminDeleteRedeemCode(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeleteRedeemCode(c.Request.Context(), id); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type redeemReq struct {
	Code string `json:"code"`
}

// handleMyRedeem applies a redeem code to the user's balance.
func (s *Server) handleMyRedeem(c *gin.Context) {
	u := userFrom(c)
	var req redeemReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}
	err := s.store.RedeemCode(c.Request.Context(), req.Code, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "redeem_not_found", "redeem code does not exist")
		return
	}
	if errors.Is(err, store.ErrRedeemUsed) {
		abortWith(c, http.StatusConflict, "redeem_used", "redeem code already used")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- promo codes ----------

func promoCodeJSON(p *store.PromoCode) gin.H {
	out := gin.H{
		"id":          p.ID,
		"code":        p.Code,
		"kind":        p.Kind,
		"value":       p.Value,
		"min_cny":     p.MinCNYFen,
		"max_uses":    p.MaxUses,
		"used_count":  p.UsedCount,
		"enabled":     p.Enabled,
		"expires_at":  p.ExpiresAt,
		"created_at":  p.CreatedAt,
	}
	return out
}

func (s *Server) handleAdminListPromos(c *gin.Context) {
	list, err := s.store.ListPromoCodes(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, promoCodeJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type promoCodeReq struct {
	Code      string  `json:"code"`
	Kind      string  `json:"kind"`
	Value     int64   `json:"value"`
	MinCNYFen int64   `json:"min_cny_fen"`
	MaxUses   int     `json:"max_uses"`
	ExpiresAt *string `json:"expires_at"`
	Enabled   *bool   `json:"enabled"`
}

func (s *Server) handleAdminCreatePromo(c *gin.Context) {
	var req promoCodeReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Code == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}
	if req.Kind != "percent" && req.Kind != "amount_off" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "kind must be percent or amount_off")
		return
	}
	if req.Value <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "value > 0 is required")
		return
	}
	if req.Kind == "percent" && req.Value >= 100 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "percent must be 1..99")
		return
	}
	expiresAt, err := parseOptionalTime(req.ExpiresAt)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "expires_at must be RFC3339")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := &store.PromoCode{
		Code:      req.Code,
		Kind:      req.Kind,
		Value:     req.Value,
		MinCNYFen: req.MinCNYFen,
		MaxUses:   req.MaxUses,
		ExpiresAt: expiresAt,
		Enabled:   enabled,
	}
	if err := s.store.CreatePromoCode(c.Request.Context(), p); err != nil {
		httpErr(c, err)
		return
	}
	created, _ := s.store.GetPromoCode(c.Request.Context(), req.Code)
	c.JSON(http.StatusCreated, promoCodeJSON(created))
}

func (s *Server) handleAdminUpdatePromo(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	list, err := s.store.ListPromoCodes(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	var target *store.PromoCode
	for i := range list {
		if list[i].ID == id {
			target = &list[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req promoCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Kind != "" {
		if req.Kind != "percent" && req.Kind != "amount_off" {
			abortWith(c, http.StatusBadRequest, "invalid_request", "kind must be percent or amount_off")
			return
		}
		target.Kind = req.Kind
	}
	if req.Value > 0 {
		target.Value = req.Value
	}
	if req.Kind == "percent" && target.Value >= 100 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "percent must be 1..99")
		return
	}
	if req.MinCNYFen >= 0 {
		target.MinCNYFen = req.MinCNYFen
	}
	if req.MaxUses >= 0 {
		target.MaxUses = req.MaxUses
	}
	if req.ExpiresAt != nil {
		t, err := parseOptionalTime(req.ExpiresAt)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_request", "expires_at must be RFC3339")
			return
		}
		target.ExpiresAt = t
	}
	if req.Enabled != nil {
		target.Enabled = *req.Enabled
	}
	if err := s.store.UpdatePromoCode(c.Request.Context(), target); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminDeletePromo(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeletePromoCode(c.Request.Context(), id); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------- plans & subscriptions ----------

func planJSON(p *store.Plan) gin.H {
	return gin.H{
		"id":            p.ID,
		"name":          p.Name,
		"quota_micro":   p.QuotaMicro,
		"quota_usd":     billing.USD(p.QuotaMicro),
		"period_days":   p.PeriodDays,
		"price_cny":     p.PriceCNYFen,
		"price_yuan":    float64(p.PriceCNYFen) / 100.0,
		"enabled":       p.Enabled,
		"created_at":    p.CreatedAt,
	}
}

func (s *Server) handleAdminListPlans(c *gin.Context) {
	list, err := s.store.ListPlans(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, planJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

type planReq struct {
	Name        string  `json:"name"`
	QuotaUSD    float64 `json:"quota_usd"`
	PeriodDays  int     `json:"period_days"`
	PriceCNYFen int64   `json:"price_cny_fen"`
	Enabled     *bool   `json:"enabled"`
}

func (s *Server) handleAdminCreatePlan(c *gin.Context) {
	var req planReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.PeriodDays <= 0 {
		req.PeriodDays = 30
	}
	if req.PeriodDays > 365 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "period_days must be 1..365")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := &store.Plan{
		Name:        req.Name,
		QuotaMicro:  billing.ToMicro(req.QuotaUSD),
		PeriodDays:  req.PeriodDays,
		PriceCNYFen: req.PriceCNYFen,
		Enabled:     enabled,
	}
	if err := s.store.CreatePlan(c.Request.Context(), p); err != nil {
		httpErr(c, err)
		return
	}
	created, _ := s.store.GetPlan(c.Request.Context(), p.ID)
	c.JSON(http.StatusCreated, planJSON(created))
}

func (s *Server) handleAdminUpdatePlan(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	p, err := s.store.GetPlan(c.Request.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	var req planReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name != "" {
		p.Name = req.Name
	}
	if req.QuotaUSD > 0 {
		p.QuotaMicro = billing.ToMicro(req.QuotaUSD)
	}
	if req.PeriodDays > 0 {
		p.PeriodDays = req.PeriodDays
	}
	if req.PriceCNYFen >= 0 {
		p.PriceCNYFen = req.PriceCNYFen
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := s.store.UpdatePlan(c.Request.Context(), p); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminDeletePlan(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	if err := s.store.DeletePlan(c.Request.Context(), id); err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func subscriptionJSON(sub *store.Subscription) gin.H {
	return gin.H{
		"plan_id":     sub.PlanID,
		"plan_name":   sub.PlanName,
		"quota_micro": sub.QuotaMicro,
		"quota_usd":   billing.USD(sub.QuotaMicro),
		"used_micro":  sub.UsedMicro,
		"used_usd":    billing.USD(sub.UsedMicro),
		"reset_at":    sub.ResetAt,
		"created_at":  sub.CreatedAt,
	}
}

// handleMySubscription returns the caller's active subscription (404 -> null).
func (s *Server) handleMySubscription(c *gin.Context) {
	u := userFrom(c)
	sub, err := s.store.GetUserSubscription(c.Request.Context(), u.ID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusOK, gin.H{"subscription": nil})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if !sub.ResetAt.After(time.Now()) {
		c.JSON(http.StatusOK, gin.H{"subscription": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": subscriptionJSON(sub)})
}

type setSubscriptionReq struct {
	PlanID int64 `json:"plan_id"`
}

// handleAdminSetUserSubscription assigns (or renews) a plan for a user.
func (s *Server) handleAdminSetUserSubscription(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req setSubscriptionReq
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanID <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "plan_id is required")
		return
	}
	_, err = s.store.GetPlan(c.Request.Context(), req.PlanID)
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "plan_not_found", "plan does not exist")
		return
	}
	sub, err := s.store.SetUserSubscription(c.Request.Context(), id, req.PlanID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, subscriptionJSON(sub))
}

func (s *Server) handleAdminListSubscriptions(c *gin.Context) {
	list, err := s.store.ListSubscriptions(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		item := subscriptionJSON(&list[i])
		item["user_id"] = list[i].UserID
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ---------- recharge stats & refund ----------

func (s *Server) handleAdminRechargeStats(c *gin.Context) {
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := parseIntQuery(v); err == nil {
			days = n
		}
	}
	st, err := s.store.RechargeStats(c.Request.Context(), days)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := gin.H{
		"total_micro": st.TotalMicro,
		"total_usd":   billing.USD(st.TotalMicro),
		"by_day":      make([]gin.H, 0, len(st.ByDay)),
		"by_method":   make([]gin.H, 0, len(st.ByMethod)),
		"top_users":   make([]gin.H, 0, len(st.TopUsers)),
	}
	for i := range st.ByDay {
		data["by_day"] = append(data["by_day"].([]gin.H), gin.H{"date": st.ByDay[i].Date, "micro": st.ByDay[i].Micro, "usd": billing.USD(st.ByDay[i].Micro)})
	}
	for i := range st.ByMethod {
		data["by_method"] = append(data["by_method"].([]gin.H), gin.H{"method": st.ByMethod[i].Method, "micro": st.ByMethod[i].Micro, "usd": billing.USD(st.ByMethod[i].Micro), "orders": st.ByMethod[i].Orders})
	}
	for i := range st.TopUsers {
		data["top_users"] = append(data["top_users"].([]gin.H), gin.H{"user_id": st.TopUsers[i].UserID, "email": st.TopUsers[i].Email, "micro": st.TopUsers[i].Micro, "usd": billing.USD(st.TopUsers[i].Micro)})
	}
	c.JSON(http.StatusOK, data)
}

type refundReq struct {
	Reason string `json:"reason"`
}

func (s *Server) handleAdminRefundRecharge(c *gin.Context) {
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	var req refundReq
	_ = c.ShouldBindJSON(&req)
	if err := s.store.RefundRecharge(c.Request.Context(), id, req.Reason); err != nil {
		if errors.Is(err, store.ErrRechargeRefund) {
			abortWith(c, http.StatusConflict, "refund_not_allowed", "order is not in a refundable state")
			return
		}
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parseOptionalTime parses an RFC3339 timestamp; nil input -> nil output.
func parseOptionalTime(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func parseIntQuery(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not an int")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}