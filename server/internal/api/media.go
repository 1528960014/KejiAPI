package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// mediaTypes lists the supported generation types.
var mediaTypes = map[string]bool{"image": true, "video": true, "music": true, "tts": true}

// inferMediaType returns the model's single non-chat capability, if any.
func inferMediaType(capabilities []string) string {
	media := []string{}
	for _, c := range capabilities {
		if c != "chat" && mediaTypes[c] {
			media = append(media, c)
		}
	}
	if len(media) == 1 {
		return media[0]
	}
	return ""
}

func hasCapability(capabilities []string, cap string) bool {
	for _, c := range capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// newTaskUUID builds a unique task ID (32 hex chars).
func newTaskUUID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// taskJSON renders a task for API responses.
func taskJSON(t *store.Task) gin.H {
	return gin.H{
		"task_id":     t.TaskUUID,
		"status":      t.Status,
		"type":        t.Type,
		"model":       t.ModelID,
		"result_urls": t.ResultURLs,
		"cost_usd":    t.Cost,
		"error":       t.ErrorMsg,
		"created_at":  t.CreatedAt,
		"updated_at":  t.UpdatedAt,
	}
}

// handleMediaGenerate submits an async media generation task.
//
//	POST /v1/media/generate
//	{"model": "flux-1", "type": "image", "prompt": "...", "n": 1, "size": "1024x1024"}
//
// The media type may be omitted when the model has exactly one media
// capability. Cost = unit_price x n is held from the key's billing user
// before the task is queued; the worker settles on success or releases on
// failure.
func (s *Server) handleMediaGenerate(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}
	var req struct {
		Model string `json:"model"`
		Type  string `json:"type"`
		N     int    `json:"n"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		abortWith(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Model == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	model, err := s.store.GetModel(ctx, req.Model)
	if errors.Is(err, store.ErrNotFound) || (model != nil && !model.Enabled) {
		abortWith(c, http.StatusNotFound, "model_not_found", "model not found or disabled: "+req.Model)
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if !modelAllowed(key, req.Model) {
		abortWith(c, http.StatusForbidden, "model_not_allowed", "API key is not allowed to use model "+req.Model)
		return
	}

	mediaType := req.Type
	if mediaType == "" {
		mediaType = inferMediaType(model.Capabilities)
	}
	if mediaType == "" {
		abortWith(c, http.StatusBadRequest, "invalid_request", "type is required (image/video/music/tts); model "+req.Model+" has no single media capability")
		return
	}
	if !hasCapability(model.Capabilities, mediaType) {
		abortWith(c, http.StatusBadRequest, "invalid_request", "model "+req.Model+" does not support type "+mediaType)
		return
	}

	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	estMicro := billing.MediaCostMicro(model, n)
	if estMicro == 0 && model.PriceUnit == "token" {
		// A token-priced model has no media price configured.
		abortWith(c, http.StatusBadRequest, "invalid_request", "model "+req.Model+" has no media price configured (set unit_price)")
		return
	}
	reason := "media:" + req.Model + ":" + mediaType

	held := false
	userID := keyUserID(key)
	// The task UUID doubles as the ledger request ID, so the hold entry and
	// the worker's settle/release stay auditable against the same ID.
	taskUUID := newTaskUUID()
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
		if err := s.store.HoldFunds(ctx, *userID, estMicro, taskUUID, reason); err != nil {
			if errors.Is(err, store.ErrInsufficientBalance) {
				abortWith(c, http.StatusPaymentRequired, "insufficient_balance", "insufficient balance; top up this user via the admin API")
				return
			}
			httpErr(c, err)
			return
		}
		held = true
	}

	t := &store.Task{
		TaskUUID:  taskUUID,
		Type:      mediaType,
		ModelID:   req.Model,
		Payload:   body,
		HoldMicro: estMicro,
	}
	if key != nil {
		t.APIKeyID = &key.ID
	}
	if err := s.store.CreateTask(ctx, t); err != nil {
		if held {
			s.releaseHeld(ctx, *userID, estMicro, t.TaskUUID, reason)
		}
		httpErr(c, err)
		return
	}
	slog.Info("media task queued", "task", t.TaskUUID, "type", mediaType, "model", req.Model)
	c.JSON(http.StatusAccepted, gin.H{"task_id": t.TaskUUID, "status": "queued"})
}

// releaseHeld best-effort releases a hold after a failed task submission.
func (s *Server) releaseHeld(ctx context.Context, userID, holdMicro int64, requestID, reason string) {
	sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.store.ReleaseFunds(sc, userID, holdMicro, requestID, reason); err != nil {
		slog.Warn("release held funds", "error", err, "request_id", requestID)
	}
}

// handleMediaStatus reports one task. Only the owning API key may read it.
func (s *Server) handleMediaStatus(c *gin.Context) {
	ctx := c.Request.Context()
	key := s.apiKeyFrom(c)
	t, err := s.store.GetTaskByUUID(ctx, c.Param("task_id"))
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusNotFound, "task_not_found", "task not found")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if key != nil && t.APIKeyID != nil && *t.APIKeyID != key.ID {
		abortWith(c, http.StatusForbidden, "forbidden", "this task belongs to another API key")
		return
	}
	c.JSON(http.StatusOK, taskJSON(t))
}

// handleListTasks is the admin view of recent tasks (newest first).
func (s *Server) handleListTasks(c *gin.Context) {
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
	tasks, err := s.store.ListTasks(ctx, keyID, limit)
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(tasks))
	for i := range tasks {
		data = append(data, taskJSON(&tasks[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}