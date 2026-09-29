package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
	"modelhub/internal/task"
)

// drama shot-count bounds (clamped server-side).
const (
	minDramaShots     = 2
	maxDramaShots     = 24
	defaultDramaShots = 8
)

// handleDramaGenerate submits an async comic-drama generation.
//
//	POST /v1/drama/generate
//	{"script": "...", "style": "水墨", "shots": 8,
//	 "storyboard_model": "gpt-4o-mini", "image_model": "flux-1", "tts_model": "cosyvoice-v1"}
//
// The whole drama is held (shots x (image unit + tts unit)) before queuing;
// the worker settles on full success or releases on any failure.
func (s *Server) handleDramaGenerate(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}
	var req struct {
		Script          string `json:"script"`
		Style           string `json:"style"`
		Shots           int    `json:"shots"`
		StoryboardModel string `json:"storyboard_model"`
		ImageModel      string `json:"image_model"`
		TTSModel        string `json:"tts_model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Script) == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "script is required")
		return
	}
	if req.ImageModel == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "image_model is required")
		return
	}
	if req.Shots <= 0 {
		req.Shots = defaultDramaShots
	}
	if req.Shots < minDramaShots {
		req.Shots = minDramaShots
	}
	if req.Shots > maxDramaShots {
		req.Shots = maxDramaShots
	}

	// image model: enabled, image capability, per-item price, key-allowed
	imageModel, err := s.store.GetModel(ctx, req.ImageModel)
	if errors.Is(err, store.ErrNotFound) || (imageModel != nil && !imageModel.Enabled) {
		abortWith(c, http.StatusNotFound, "model_not_found", "image model not found or disabled: "+req.ImageModel)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if !hasCapability(imageModel.Capabilities, "image") {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model "+req.ImageModel+" does not support image")
		return
	}
	if imageModel.UnitPrice <= 0 {
		abortWith(c, http.StatusBadRequest, "invalid_request", "image model has no unit_price configured")
		return
	}
	if !modelAllowed(key, req.ImageModel) {
		abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+req.ImageModel)
		return
	}

	// tts model (optional)
	var ttsModel *store.Model
	if req.TTSModel != "" {
		ttsModel, err = s.store.GetModel(ctx, req.TTSModel)
		if errors.Is(err, store.ErrNotFound) || (ttsModel != nil && !ttsModel.Enabled) {
			abortWith(c, http.StatusNotFound, "model_not_found", "tts model not found or disabled: "+req.TTSModel)
			return
		}
		if err != nil {
			httpErr(c, err)
			return
		}
		if !hasCapability(ttsModel.Capabilities, "tts") || ttsModel.UnitPrice <= 0 {
			abortWith(c, http.StatusBadRequest, "invalid_request", "tts model does not support tts or has no unit_price: "+req.TTSModel)
			return
		}
		if !modelAllowed(key, req.TTSModel) {
			abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+req.TTSModel)
			return
		}
	}

	// storyboard model (optional; when omitted the script is split naively)
	if req.StoryboardModel != "" {
		sbModel, err := s.store.GetModel(ctx, req.StoryboardModel)
		if errors.Is(err, store.ErrNotFound) || (sbModel != nil && !sbModel.Enabled) {
			abortWith(c, http.StatusNotFound, "model_not_found", "storyboard model not found or disabled: "+req.StoryboardModel)
			return
		}
		if err != nil {
			httpErr(c, err)
			return
		}
		if !hasCapability(sbModel.Capabilities, "chat") {
			abortWith(c, http.StatusBadRequest, "invalid_request", "storyboard model does not support chat: "+req.StoryboardModel)
			return
		}
		if !modelAllowed(key, req.StoryboardModel) {
			abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+req.StoryboardModel)
			return
		}
	}

	estMicro := billing.DramaCostMicro(imageModel, ttsModel, req.Shots)
	reason := "drama:" + req.ImageModel

	// The drama UUID doubles as the ledger request ID (hold/settle/release
	// stay auditable against the same ID).
	dramaUUID := newTaskUUID()
	held := false
	userID := keyUserID(key)
	if userID != nil && estMicro > 0 {
		user, err := s.store.GetUser(ctx, *userID)
		if errors.Is(err, store.ErrNotFound) {
			abortWith(c, http.StatusForbidden, "user_not_found", "billing user of this API key no longer exists")
			return
		}
		if err != nil {
			httpErr(c, err)
			return
		}
		if !user.Enabled {
			abortWith(c, http.StatusForbidden, "user_disabled", "billing user is disabled")
			return
		}
		// P2-3: agent wholesale rate discounts the frozen amount.
		estMicro = billing.ApplyRate(estMicro, user.AgentRate)
	}
	if key != nil && key.Quota != nil && key.Spend+estMicro > *key.Quota {
		abortWith(c, http.StatusTooManyRequests, "quota_exceeded", "API key quota exhausted; ask the admin to raise the quota")
		return
	}
	if userID != nil && estMicro > 0 {
		if err := s.store.HoldFunds(ctx, *userID, estMicro, dramaUUID, reason); err != nil {
			if errors.Is(err, store.ErrInsufficientBalance) {
				abortWith(c, http.StatusPaymentRequired, "insufficient_balance", "insufficient balance; top up this user via the admin API")
				return
			}
			httpErr(c, err)
			return
		}
		held = true
	}

	d := &store.Drama{
		DramaUUID:       dramaUUID,
		Title:           firstLine(req.Script),
		Script:          req.Script,
		Style:           req.Style,
		StoryboardModel: req.StoryboardModel,
		ImageModel:      req.ImageModel,
		ShotsPlanned:    req.Shots,
		HoldMicro:       estMicro,
		Shots:           []byte("[]"),
	}
	if req.TTSModel != "" {
		tts := req.TTSModel
		d.TTSModel = &tts
	}
	if key != nil {
		d.APIKeyID = &key.ID
	}
	if err := s.store.CreateDrama(ctx, d); err != nil {
		if held {
			s.releaseHeld(ctx, *userID, estMicro, dramaUUID, reason)
		}
		httpErr(c, err)
		return
	}
	slog.Info("drama queued", "drama", d.DramaUUID, "shots", d.ShotsPlanned, "image", d.ImageModel)
	c.JSON(http.StatusAccepted, gin.H{"drama_id": d.DramaUUID, "status": "queued"})
}

// dramaJSON renders a drama for API responses; shots (the asset pack) is
// embedded verbatim.
func dramaJSON(d *store.Drama) gin.H {
	var shots any
	if len(d.Shots) > 0 {
		_ = json.Unmarshal(d.Shots, &shots)
	}
	if shots == nil {
		shots = []any{}
	}
	return gin.H{
		"drama_id":         d.DramaUUID,
		"status":           d.Status,
		"title":            d.Title,
		"style":            d.Style,
		"storyboard_model": d.StoryboardModel,
		"image_model":      d.ImageModel,
		"tts_model":        d.TTSModel,
		"shots_planned":    d.ShotsPlanned,
		"shots":            shots,
		"cost_usd":         d.Cost,
		"error":            d.ErrorMsg,
		"video_url":        d.VideoURL,
		"video_error":      d.VideoError,
		"created_at":       d.CreatedAt,
		"updated_at":       d.UpdatedAt,
	}
}

// handleDramaStatus reports one drama. Only the owning API key may read it.
func (s *Server) handleDramaStatus(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)
	d, err := s.store.GetDramaByUUID(ctx, c.Param("drama_id"))
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "drama_not_found", "drama not found")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if key != nil && d.APIKeyID != nil && *d.APIKeyID != key.ID {
		abortWith(c, http.StatusForbidden, "forbidden", "this drama belongs to another API key")
		return
	}
	c.JSON(http.StatusOK, dramaJSON(d))
}

// handleListDramas is the admin view of recent dramas (newest first).
func (s *Server) handleListDramas(c *gin.Context) {
	ctx := c.Request.Context()
	var keyID *int64
	if v := c.Query("api_key_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			abortWith(c, http.StatusBadRequest, "invalid_request", "api_key_id must be a number")
			return
		}
		keyID = &id
	}
	limit, _ := parsePagination(c, 50, 200)
	dramas, err := s.store.ListDramas(ctx, keyID, limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(dramas))
	for i := range dramas {
		data = append(data, dramaJSON(&dramas[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// handleDramaVideo serves a composed drama MP4 (P2-1b). The 32-hex drama
// UUID is the only credential; the file must have been written by the
// composer for that exact UUID.
func (s *Server) handleDramaVideo(c *gin.Context) {
	uuid := c.Param("uuid")
	if !task.IsValidDramaUUID(uuid) {
		abortWith(c, http.StatusNotFound, "not_found", "not found")
		return
	}
	c.File(filepath.Join(s.cfg.MediaDir, "dramas", uuid+".mp4"))
}

// firstLine derives a short display title from the script.
func firstLine(script string) string {
	for _, line := range strings.Split(script, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 60 {
				return line[:60] + "…"
			}
			return line
		}
	}
	return "untitled"
}
