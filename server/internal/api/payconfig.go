package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"modelhub/internal/config"
	"modelhub/internal/pay"
	"modelhub/internal/store"
)

// P3-2: runtime-editable payment channel configuration.
//
// The PAY_* environment variables are only the initial default: on first
// start the api layer seeds pay_settings / pay_channels from env. After that
// the database is the single source of truth and the admin endpoints
// (GET/PUT /admin/pay-config) edit it with hot reload — no restart needed.

// payChannelIDs is the fixed set of channel IDs, in display order.
var payChannelIDs = []string{"yipay", "alipay", "wechat"}

// payChannelFields is the credential field set per channel (API/JSON names).
var payChannelFields = map[string][]string{
	"yipay":  {"mapi_url", "pid", "key"},
	"alipay": {"app_id", "private_key", "public_key"},
	"wechat": {"mch_id", "app_id", "api_v3_key", "merchant_serial", "private_key", "platform_key"},
}

// paySecretFields are masked in GET responses. On PUT, a masked or empty
// value keeps the stored value; any other value replaces it.
var paySecretFields = map[string]bool{
	"key":          true,
	"private_key":  true,
	"api_v3_key":   true,
	"platform_key": true,
}

const payMaskedValue = "********"

// payChannelStatus reports whether a channel could be built from its config.
type payChannelStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// payConfigState is the in-memory, hot-swappable payment configuration.
// Reloads replace the whole struct; readers see a consistent snapshot.
type payConfigState struct {
	cfg      pay.Config
	channels map[string]pay.Channel
	status   map[string]payChannelStatus
}

// storedChannel is a DB channel row parsed into editable fields.
type storedChannel struct {
	enabled bool
	fields  map[string]string
}

// paySnapshot returns a consistent copy of the in-memory payment state.
func (s *Server) paySnapshot() payConfigState {
	s.payMu.RLock()
	defer s.payMu.RUnlock()
	return s.payState
}

// envPayState builds the initial in-memory state from the environment
// (used at construction, before the DB seed, and as the fallback if the DB
// seed fails).
func envPayState(cfg *config.Config) payConfigState {
	settings := store.PaySettings{CNYPerUSD: cfg.CNYPerUSD, PublicURL: cfg.PublicURL}
	stored := map[string]storedChannel{}
	if _, _, chs := pay.EnvDefaults(cfg); settings.CNYPerUSD > 0 {
		for id, f := range chs {
			if completePayFields(id, f) {
				stored[id] = storedChannel{enabled: true, fields: f}
			}
		}
	}
	return buildPayState(settings, stored)
}

// completePayFields reports whether every credential field is non-empty.
func completePayFields(id string, f map[string]string) bool {
	for _, name := range payChannelFields[id] {
		if strings.TrimSpace(f[name]) == "" {
			return false
		}
	}
	return true
}

// missingPayFields lists the required credential fields that are empty.
func missingPayFields(id string, f map[string]string) []string {
	out := []string{}
	for _, name := range payChannelFields[id] {
		if strings.TrimSpace(f[name]) == "" {
			out = append(out, name)
		}
	}
	return out
}

// buildPayState assembles the in-memory payment state. Channels that are
// disabled, incomplete, or carry invalid keys are reported in status and
// skipped — a bad credential never takes the server down.
func buildPayState(settings store.PaySettings, stored map[string]storedChannel) payConfigState {
	out := payConfigState{
		cfg:      pay.Config{CNYPerUSD: settings.CNYPerUSD, PublicURL: settings.PublicURL},
		channels: map[string]pay.Channel{},
		status:   map[string]payChannelStatus{},
	}
	for _, id := range payChannelIDs {
		out.status[id] = payChannelStatus{Error: "disabled"}
	}
	if out.cfg.CNYPerUSD <= 0 {
		for _, id := range payChannelIDs {
			out.status[id] = payChannelStatus{Error: "recharge disabled (cny_per_usd = 0)"}
		}
		return out
	}
	for _, id := range payChannelIDs {
		sc, ok := stored[id]
		if !ok || !sc.enabled {
			continue
		}
		if missing := missingPayFields(id, sc.fields); len(missing) > 0 {
			out.status[id] = payChannelStatus{Error: "missing: " + strings.Join(missing, ", ")}
			continue
		}
		f := sc.fields
		switch id {
		case "yipay":
			out.channels["yipay"] = pay.NewYiPay(f["mapi_url"], f["pid"], f["key"])
			out.status["yipay"] = payChannelStatus{OK: true}
		case "alipay":
			ch, err := pay.NewAlipay(f["app_id"], f["private_key"], f["public_key"])
			if err != nil {
				out.status["alipay"] = payChannelStatus{Error: err.Error()}
				break
			}
			out.channels["alipay"] = ch
			out.status["alipay"] = payChannelStatus{OK: true}
		case "wechat":
			ch, err := pay.NewWechat(f["mch_id"], f["app_id"], f["api_v3_key"], f["merchant_serial"],
				f["private_key"], f["platform_key"])
			if err != nil {
				out.status["wechat"] = payChannelStatus{Error: err.Error()}
				break
			}
			out.channels["wechat"] = ch
			out.status["wechat"] = payChannelStatus{OK: true}
		}
	}
	return out
}

// ReloadPayConfig seeds the database from env on first start (settings row
// missing) and then loads the payment state from the database. Call it after
// construction; on error the env-based state is kept.
func (s *Server) ReloadPayConfig(ctx context.Context) error {
	exists, err := s.store.PaySettingsExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		rate, url, chs := pay.EnvDefaults(s.cfg)
		if err := s.store.SetPaySettings(ctx, store.PaySettings{CNYPerUSD: rate, PublicURL: url}); err != nil {
			return err
		}
		rows, err := s.store.ListPayChannels(ctx)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, r := range rows {
			have[r.ChannelID] = true
		}
		for _, id := range payChannelIDs {
			if have[id] {
				continue
			}
			f := chs[id]
			blob, _ := json.Marshal(f)
			if err := s.store.SetPayChannel(ctx, &store.PayChannel{
				ChannelID: id,
				Enabled:   completePayFields(id, f),
				Config:    blob,
			}); err != nil {
				return err
			}
		}
		slog.Info("pay config: seeded database from environment defaults")
	}
	return s.reloadPayConfigFromDB(ctx)
}

// reloadPayConfigFromDB re-reads the payment configuration and hot-swaps the
// in-memory state.
func (s *Server) reloadPayConfigFromDB(ctx context.Context) error {
	settings, err := s.store.GetPaySettings(ctx)
	if err != nil {
		return err
	}
	rows, err := s.store.ListPayChannels(ctx)
	if err != nil {
		return err
	}
	stored := map[string]storedChannel{}
	for _, r := range rows {
		f := map[string]string{}
		if len(r.Config) > 0 {
			if err := json.Unmarshal(r.Config, &f); err != nil {
				slog.Warn("pay config: bad channel config json", "channel", r.ChannelID, "error", err)
				continue
			}
		}
		stored[r.ChannelID] = storedChannel{enabled: r.Enabled, fields: f}
	}
	state := buildPayState(settings, stored)
	s.payMu.Lock()
	s.payState = state
	s.payMu.Unlock()
	return nil
}

// payConfigJSON renders the admin view: secrets masked, per-channel build status.
func (s *Server) payConfigJSON(ctx context.Context) (gin.H, error) {
	settings, err := s.store.GetPaySettings(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListPayChannels(ctx)
	if err != nil {
		return nil, err
	}
	rowByID := map[string]store.PayChannel{}
	for _, r := range rows {
		rowByID[r.ChannelID] = r
	}
	st := s.paySnapshot()
	channels := gin.H{}
	for _, id := range payChannelIDs {
		fields := map[string]string{}
		enabled := false
		if row, ok := rowByID[id]; ok {
			enabled = row.Enabled
			if len(row.Config) > 0 {
				_ = json.Unmarshal(row.Config, &fields)
			}
		}
		cfgJSON := gin.H{}
		for _, name := range payChannelFields[id] {
			v := fields[name]
			if paySecretFields[name] && v != "" {
				cfgJSON[name] = payMaskedValue
			} else {
				cfgJSON[name] = v
			}
		}
		status := st.status[id]
		channels[id] = gin.H{
			"enabled": enabled,
			"config":  cfgJSON,
			"status":  status,
		}
	}
	return gin.H{
		"cny_per_usd": settings.CNYPerUSD,
		"public_url":  settings.PublicURL,
		"channels":    channels,
	}, nil
}

// handleGetPayConfig returns the payment configuration (admin).
func (s *Server) handleGetPayConfig(c *gin.Context) {
	out, err := s.payConfigJSON(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

type payChannelReq struct {
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

type payConfigReq struct {
	CNYPerUSD *float64          `json:"cny_per_usd"`
	PublicURL *string           `json:"public_url"`
	Channels  map[string]payChannelReq `json:"channels"`
}

// mergePayFields applies update semantics per credential field:
// omitted, empty, or masked -> keep stored; anything else replaces.
func mergePayFields(id string, stored map[string]string, in map[string]any) (map[string]string, error) {
	out := map[string]string{}
	for _, name := range payChannelFields[id] {
		old := stored[name]
		v, present := in[name]
		if !present {
			out[name] = old
			continue
		}
		str, ok := v.(string)
		if !ok {
			return nil, errPayConfigFieldNotString(id, name)
		}
		str = strings.TrimSpace(str)
		if str == "" || str == payMaskedValue {
			out[name] = old
		} else {
			out[name] = str
		}
	}
	return out, nil
}

func errPayConfigFieldNotString(id, name string) error {
	return &payConfigFieldError{channel: id, field: name}
}

type payConfigFieldError struct {
	channel string
	field   string
}

func (e *payConfigFieldError) Error() string {
	return "config." + e.channel + "." + e.field + " must be a string"
}

// handlePutPayConfig updates the payment configuration (admin) and hot-swaps
// the in-memory state. Responds with the fresh (masked) configuration.
func (s *Server) handlePutPayConfig(c *gin.Context) {
	ctx := c.Request.Context()
	var req payConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "invalid body")
		return
	}
	if req.CNYPerUSD != nil && (*req.CNYPerUSD < 0 || *req.CNYPerUSD > 100) {
		abortWith(c, http.StatusBadRequest, "invalid_rate", "cny_per_usd must be between 0 and 100 (0 disables recharge)")
		return
	}
	if req.CNYPerUSD != nil || req.PublicURL != nil {
		settings, err := s.store.GetPaySettings(ctx)
		if err != nil {
			httpErr(c, err)
			return
		}
		if req.CNYPerUSD != nil {
			settings.CNYPerUSD = *req.CNYPerUSD
		}
		if req.PublicURL != nil {
			settings.PublicURL = strings.TrimSpace(*req.PublicURL)
		}
		if err := s.store.SetPaySettings(ctx, settings); err != nil {
			httpErr(c, err)
			return
		}
	}
	if len(req.Channels) > 0 {
		rows, err := s.store.ListPayChannels(ctx)
		if err != nil {
			httpErr(c, err)
			return
		}
		rowByID := map[string]store.PayChannel{}
		for _, r := range rows {
			rowByID[r.ChannelID] = r
		}
		for id, ch := range req.Channels {
			if _, known := payChannelFields[id]; !known {
				abortWith(c, http.StatusBadRequest, "unknown_channel", "unknown payment channel: "+id)
				return
			}
			stored := map[string]string{}
			if row, ok := rowByID[id]; ok && len(row.Config) > 0 {
				_ = json.Unmarshal(row.Config, &stored)
			}
			if ch.Config != nil {
				merged, err := mergePayFields(id, stored, ch.Config)
				if err != nil {
					abortWith(c, http.StatusBadRequest, "invalid_config", err.Error())
					return
				}
				stored = merged
			}
			enabled := false
			if row, ok := rowByID[id]; ok {
				enabled = row.Enabled
			}
			if ch.Enabled != nil {
				enabled = *ch.Enabled
			}
			blob, _ := json.Marshal(stored)
			if err := s.store.SetPayChannel(ctx, &store.PayChannel{
				ChannelID: id,
				Enabled:   enabled,
				Config:    blob,
			}); err != nil {
				httpErr(c, err)
				return
			}
		}
	}
	if err := s.reloadPayConfigFromDB(ctx); err != nil {
		httpErr(c, err)
		return
	}
	out, err := s.payConfigJSON(ctx)
	if err != nil {
		httpErr(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}