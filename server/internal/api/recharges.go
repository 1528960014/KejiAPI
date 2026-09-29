package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/pay"
	"modelhub/internal/store"
)

// P2-4: online recharge (CNY payment -> USD balance).
//
// User flow: POST /api/me/recharges {amount_cny (fen), method} -> the order
// is created with a fixed credit (micro-USD) and the channel returns a QR
// code and/or payment URL. The channel's async notify (public, signature
// verified) flips the order to paid and credits the balance in one
// transaction (idempotent).

const (
	minRechargeCNY = 100     // ¥1
	maxRechargeCNY = 1000000 // ¥10,000
)

func rechargeJSON(r *store.Recharge) gin.H {
	out := gin.H{
		"id":           r.ID,
		"order_no":     r.OrderNo,
		"method":       r.Method,
		"amount_cny":   r.AmountCNY,
		"amount_yuan":  float64(r.AmountCNY) / 100.0,
		"credit_micro": r.CreditMicro,
		"credit_usd":   billing.USD(r.CreditMicro),
		"status":       r.Status,
		"created_at":   r.CreatedAt,
		"paid_at":      nil,
	}
	if r.PaidAt != nil {
		out["paid_at"] = *r.PaidAt
	}
	return out
}

// handleRechargeConfig exposes which channels are enabled and the rate.
func (s *Server) handleRechargeConfig(c *gin.Context) {
	st := s.paySnapshot()
	c.JSON(http.StatusOK, gin.H{
		"enabled":     st.cfg.CNYPerUSD > 0 && len(st.channels) > 0,
		"cny_per_usd": st.cfg.CNYPerUSD,
		"methods":     pay.Methods(st.channels),
		"min_cny":     minRechargeCNY,
		"max_cny":     maxRechargeCNY,
	})
}

type createRechargeReq struct {
	AmountCNY int64  `json:"amount_cny"` // fen
	Method    string `json:"method"`
	SubType   string `json:"sub_type"` // yipay: "alipay" | "wxpay"
}

// handleCreateRecharge creates a pending order and asks the channel for a
// QR code / payment URL.
func (s *Server) handleCreateRecharge(c *gin.Context) {
	u := userFrom(c)
	var req createRechargeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "amount_cny (fen) and method are required")
		return
	}
	if req.AmountCNY < minRechargeCNY || req.AmountCNY > maxRechargeCNY {
		abortWith(c, http.StatusBadRequest, "invalid_amount", "amount_cny must be between 100 and 1000000 fen")
		return
	}
	st := s.paySnapshot()
	if st.cfg.CNYPerUSD <= 0 {
		abortWith(c, http.StatusServiceUnavailable, "recharge_disabled", "online recharge is not configured")
		return
	}
	ch, ok := st.channels[req.Method]
	if !ok {
		abortWith(c, http.StatusBadRequest, "invalid_method", "payment method is not enabled")
		return
	}
	credit := pay.CreditMicro(req.AmountCNY, st.cfg.CNYPerUSD)
	if credit <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_amount", "amount too small to credit any balance")
		return
	}
	order, err := s.store.CreateRecharge(c.Request.Context(), u.ID, ch.ID(), req.AmountCNY, credit)
	if err != nil {
		httpErr(c, err)
		return
	}

	base := s.publicBaseURL(c.Request)
	payment, err := ch.CreateOrder(c.Request.Context(), pay.Order{
		OutTradeNo:   order.OrderNo,
		AmountCNYFen: order.AmountCNY,
		Name:         "ModelHub 余额充值",
		NotifyURL:    base + "/pay/notify/" + ch.ID(),
		ReturnURL:    base + "/recharge?order=" + order.OrderNo,
		ClientIP:     c.ClientIP(),
		SubType:      req.SubType,
	})
	if err != nil {
		_ = s.store.FailRecharge(c.Request.Context(), order.OrderNo)
		abortWith(c, http.StatusBadGateway, "payment_channel_error", "payment channel order failed: "+err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"order":   rechargeJSON(order),
		"payment": gin.H{"qr_code": payment.QRCode, "pay_url": payment.PayURL},
	})
}

// publicBaseURL resolves the externally reachable base URL for notify URLs.
func (s *Server) publicBaseURL(r *http.Request) string {
	if s.paySnapshot().cfg.PublicURL != "" {
		return s.paySnapshot().cfg.PublicURL
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return scheme + "://" + r.Host
}

// handleMyRecharges lists the user's recent orders.
func (s *Server) handleMyRecharges(c *gin.Context) {
	u := userFrom(c)
	limit, _ := parsePagination(c, 20, 100)
	list, err := s.store.ListRecharges(c.Request.Context(), u.ID, limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		data = append(data, rechargeJSON(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleGetMyRecharge fetches one of the user's orders (status polling).
func (s *Server) handleGetMyRecharge(c *gin.Context) {
	u := userFrom(c)
	id, err := parseIDParam(c)
	if err != nil {
		return
	}
	r, err := s.store.GetRecharge(c.Request.Context(), id, u.ID)
	if errors.Is(err, store.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, rechargeJSON(r))
}

// handlePayNotify is the public async callback for one channel. The channel
// verifies its own signature; we then match the order and credit in one
// transaction. Duplicate notifies are safe (idempotent).
func (s *Server) handlePayNotify(method string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ch, ok := s.paySnapshot().channels[method]
		if !ok {
			c.String(http.StatusNotFound, "channel disabled")
			return
		}
		res, err := ch.ParseNotify(c.Request)
		if err != nil {
			c.String(http.StatusBadRequest, "verify failed")
			return
		}
		order, err := s.store.GetRechargeByOrderNo(c.Request.Context(), res.OutTradeNo)
		if errors.Is(err, store.ErrNotFound) {
			c.String(http.StatusNotFound, "order not found")
			return
		}
		if err != nil {
			httpErr(c, err)
			return
		}
		if order.Method != ch.ID() {
			c.String(http.StatusBadRequest, "method mismatch")
			return
		}
		if order.AmountCNY != res.AmountCNYFen {
			c.String(http.StatusBadRequest, "amount mismatch")
			return
		}
		if _, err := s.store.MarkRechargePaid(c.Request.Context(), res.OutTradeNo, res.ChannelTradeNo, order.CreditMicro); err != nil {
			httpErr(c, err)
			return
		}
		if ch.ID() == "wechat" {
			c.JSON(http.StatusOK, gin.H{"code": "SUCCESS", "message": "成功"})
			return
		}
		c.String(http.StatusOK, "success")
	}
}

// handleAdminRecharges lists orders across users (admin).
func (s *Server) handleAdminRecharges(c *gin.Context) {
	limit, offset := parsePagination(c, 50, 200)
	list, err := s.store.AdminListRecharges(c.Request.Context(), limit, offset)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(list))
	for i := range list {
		item := rechargeJSON(&list[i])
		item["user_id"] = list[i].UserID
		item["email"] = list[i].Email
		data = append(data, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
