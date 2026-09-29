package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"modelhub/internal/billing"
	"modelhub/internal/store"
)

// P4-1: bidirectional real-time voice — a pass-through proxy for the OpenAI
// Realtime API (WebSocket). Clients speak the standard OpenAI WS protocol:
//
//	POST /v1/realtime
//	Sec-WebSocket-Protocol: openai-insecure-api-key.<modelhub sk-...>
//
// The gateway validates the ModelHub key, waits for the client's first
// session.update to name a model (capability "realtime"), dials the channel
// for that model with the upstream key subprotocol, then pipes frames in both
// directions. Usage is captured from response.done events and settled when
// the session ends. There is no pre-hold (a session can last minutes); a
// crash mid-session loses at most that session's charge — documented.

const (
	realtimeProtoPrefix = "openai-insecure-api-key."
	realtimeReadLimit   = 8 << 20
	maxBufferedFrames   = 16
	maxBufferedBytes    = 1 << 20
)

// --- pure helpers (unit-tested) ---

// realtimeKeyFromProtocols finds the ModelHub API key in the WS subprotocol
// list ("Sec-Websocket-Protocol" header, comma separated).
func realtimeKeyFromProtocols(protocols []string) string {
	for _, p := range protocols {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, realtimeProtoPrefix) {
			return strings.TrimPrefix(p, realtimeProtoPrefix)
		}
	}
	return ""
}

// realtimeSessionModel extracts the model from a client session.update frame.
func realtimeSessionModel(frame []byte) (string, bool) {
	var ev struct {
		Type    string `json:"type"`
		Session struct {
			Model string `json:"model"`
		} `json:"session"`
	}
	if err := json.Unmarshal(frame, &ev); err != nil || ev.Type != "session.update" {
		return "", false
	}
	return ev.Session.Model, ev.Session.Model != ""
}

// realtimeUsageFromFrame extracts usage from a response.done frame. Upstream
// usually reports cost directly; otherwise the token counts are used as a
// fallback against the model's per-1k prices.
func realtimeUsageFromFrame(frame []byte) (costUSD float64, inTokens, outTokens int, ok bool) {
	var ev struct {
		Type     string `json:"type"`
		Response struct {
			Usage struct {
				Cost             float64 `json:"cost"`
				InputTokens      int     `json:"input_tokens"`
				OutputTokens     int     `json:"output_tokens"`
				InputTokenCount  int     `json:"input_token_count"`
				OutputTokenCount int     `json:"output_token_count"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(frame, &ev); err != nil || ev.Type != "response.done" {
		return 0, 0, 0, false
	}
	u := ev.Response.Usage
	in, out := u.InputTokens, u.OutputTokens
	if in == 0 && out == 0 {
		in, out = u.InputTokenCount, u.OutputTokenCount
	}
	if u.Cost > 0 {
		return u.Cost, in, out, true
	}
	if in > 0 || out > 0 {
		return 0, in, out, true
	}
	return 0, 0, 0, false
}

// realtimeUpstreamURL rewrites an http(s) channel base URL to its
// ws(s) .../realtime endpoint.
func realtimeUpstreamURL(base string) string {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/realtime"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// --- session state ---

type bufferedFrame struct {
	msgType int
	data    []byte
}

type realtimeSession struct {
	s        *Server
	key      *store.APIKey
	client   *websocket.Conn
	clientMu sync.Mutex // serializes writes to the client conn

	mu        sync.Mutex
	upstream  *websocket.Conn
	model     *store.Model
	pending   []bufferedFrame
	pendSize  int
	costUSD   float64
	inTokens  int
	outTokens int
	settled   bool
}

// run pumps both directions until either side drops, then settles. When one
// pump exits it tears down the connections so the other pump unblocks.
func (rt *realtimeSession) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{}, 2)
	go func() { rt.clientPump(ctx); done <- struct{}{} }()
	go func() { rt.upstreamPump(ctx); done <- struct{}{} }()
	<-done
	cancel()
	rt.closeUpstream()
	rt.closeClient()
	<-done
	rt.settle()
}

// clientPump forwards client frames. No read deadline: a user can be silent
// for minutes during a voice call; dead TCP is detected by the OS.
func (rt *realtimeSession) clientPump(ctx context.Context) {
	rt.client.SetReadLimit(realtimeReadLimit)
	for {
		if ctx.Err() != nil {
			return
		}
		msgType, data, err := rt.client.ReadMessage()
		if err != nil {
			return
		}
		rt.handleClientFrame(msgType, data)
	}
}

// handleClientFrame forwards to the upstream once connected; before that it
// looks for the session.update that names the model and buffers early frames.
func (rt *realtimeSession) handleClientFrame(msgType int, data []byte) {
	for {
		rt.mu.Lock()
		up := rt.upstream
		rt.mu.Unlock()
		if up != nil {
			if err := up.WriteMessage(msgType, data); err != nil {
				rt.failSession("upstream write failed: " + err.Error())
			}
			return
		}
		modelID, isUpdate := "", false
		if msgType == websocket.TextMessage {
			modelID, isUpdate = realtimeSessionModel(data)
		}
		if isUpdate {
			if err := rt.connectUpstream(modelID); err != nil {
				rt.failSession(err.Error())
				return
			}
			continue
		}
		rt.mu.Lock()
		if len(rt.pending) >= maxBufferedFrames || rt.pendSize+len(data) > maxBufferedBytes {
			rt.mu.Unlock()
			rt.failSession("session.update with a model was not received in time")
			return
		}
		cp := make([]byte, len(data))
		copy(cp, data)
		rt.pending = append(rt.pending, bufferedFrame{msgType, cp})
		rt.pendSize += len(data)
		rt.mu.Unlock()
		return
	}
}

// connectUpstream resolves and validates the model, dials the channel's WS
// endpoint with the upstream key subprotocol, and flushes buffered frames.
func (rt *realtimeSession) connectUpstream(modelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	model, err := rt.s.store.GetModel(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) || (model != nil && !model.Enabled) {
		return fmt.Errorf("model not found or disabled: %s", modelID)
	}
	if err != nil {
		return err
	}
	if !hasCapability(model.Capabilities, "realtime") {
		return fmt.Errorf("model %s does not support realtime (needs the realtime capability)", modelID)
	}
	if !modelAllowed(rt.key, modelID) {
		return fmt.Errorf("API key is not allowed to use model %s", modelID)
	}
	ch, err := rt.s.store.PickChannel(ctx, modelID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no enabled channel for model %s", modelID)
	}
	if err != nil {
		return err
	}
	wsURL := realtimeUpstreamURL(ch.BaseURL)
	if wsURL == "" {
		return fmt.Errorf("channel %s has an invalid base url for websocket", ch.Name)
	}
	header := http.Header{"Sec-Websocket-Protocol": []string{realtimeProtoPrefix + ch.APIKey}}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("upstream websocket: %v (%s)", err, resp.Status)
		}
		return fmt.Errorf("upstream websocket: %w", err)
	}
	rt.mu.Lock()
	rt.upstream = conn
	rt.model = model
	pending := rt.pending
	rt.pending = nil
	rt.pendSize = 0
	rt.mu.Unlock()
	for _, f := range pending {
		if werr := conn.WriteMessage(f.msgType, f.data); werr != nil {
			return werr
		}
	}
	slog.Info("realtime session connected", "key", rt.key.ID, "model", modelID)
	return nil
}

// upstreamPump forwards upstream frames to the client and accumulates usage
// from response.done events. Upstream pings (every ~15s) keep the read
// deadline alive.
func (rt *realtimeSession) upstreamPump(ctx context.Context) {
	var up *websocket.Conn
	for {
		rt.mu.Lock()
		up = rt.upstream
		rt.mu.Unlock()
		if up != nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	up.SetReadLimit(realtimeReadLimit)
	_ = up.SetReadDeadline(time.Now().Add(realtimePongTimeout()))
	up.SetPongHandler(func(string) error {
		return up.SetReadDeadline(time.Now().Add(realtimePongTimeout()))
	})
	for {
		if ctx.Err() != nil {
			return
		}
		msgType, data, err := up.ReadMessage()
		if err != nil {
			rt.sendClientError("realtime_upstream_closed", "upstream connection closed")
			return
		}
		_ = up.SetReadDeadline(time.Now().Add(realtimePongTimeout()))
		if msgType == websocket.TextMessage {
			if cost, in, out, ok := realtimeUsageFromFrame(data); ok {
				rt.mu.Lock()
				rt.costUSD += cost
				rt.inTokens += in
				rt.outTokens += out
				rt.mu.Unlock()
			}
		}
		if err := rt.writeClient(msgType, data); err != nil {
			return
		}
	}
}

func realtimePongTimeout() time.Duration { return 60 * time.Second }

func (rt *realtimeSession) writeClient(msgType int, data []byte) error {
	rt.clientMu.Lock()
	defer rt.clientMu.Unlock()
	return rt.client.WriteMessage(msgType, data)
}

func (rt *realtimeSession) sendClientError(code, msg string) {
	payload, _ := json.Marshal(gin.H{"type": "error", "error": gin.H{"code": code, "message": msg}})
	_ = rt.writeClient(websocket.TextMessage, payload)
}

func (rt *realtimeSession) failSession(msg string) {
	rt.sendClientError("realtime_error", msg)
	rt.clientMu.Lock()
	_ = rt.client.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseInternalServerErr, msg), time.Now().Add(2*time.Second))
	_ = rt.client.Close()
	rt.clientMu.Unlock()
}

func (rt *realtimeSession) closeClient() {
	rt.clientMu.Lock()
	defer rt.clientMu.Unlock()
	_ = rt.client.Close()
}

func (rt *realtimeSession) closeUpstream() {
	rt.mu.Lock()
	up := rt.upstream
	rt.upstream = nil
	rt.mu.Unlock()
	if up != nil {
		_ = up.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(2*time.Second))
		_ = up.Close()
	}
}

// settle charges the actual session usage at session end (no pre-hold).
func (rt *realtimeSession) settle() {
	if rt.key == nil {
		return
	}
	rt.mu.Lock()
	if rt.settled {
		rt.mu.Unlock()
		return
	}
	rt.settled = true
	model, cost, inT, outT := rt.model, rt.costUSD, rt.inTokens, rt.outTokens
	rt.mu.Unlock()
	if model == nil {
		slog.Warn("realtime session ended without a model; not billed", "key", rt.key.ID)
		return
	}
	var actualMicro int64
	if cost > 0 {
		actualMicro = billing.ToMicro(cost)
	} else if inT > 0 || outT > 0 {
		actualMicro = billing.CostMicro(model, inT, outT)
	}
	if actualMicro <= 0 {
		return // no usage reported: nothing to bill
	}
	sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reqID := newRequestID()
	reason := "realtime:" + model.ModelID
	var err error
	if rt.key.OrgID != nil {
		err = rt.s.store.SettleOrgFunds(sc, rt.key.OrgID, 0, actualMicro, reqID, reason, &rt.key.ID)
	} else {
		err = rt.s.store.SettleFunds(sc, keyUserID(rt.key), 0, actualMicro, reqID, reason, &rt.key.ID)
	}
	if err != nil {
		slog.Warn("settle realtime funds", "error", err, "request_id", reqID)
		return
	}
	rt.s.limiter.AddTokens(rt.key.ID, inT+outT)
	rt.s.logUsage(sc, rt.key, model, false, inT, outT, "ok", "", billing.USD(actualMicro))
	slog.Info("realtime session settled", "key", rt.key.ID, "model", model.ModelID, "cost_usd", billing.USD(actualMicro))
}

// --- handler ---

// handleRealtime upgrades POST /v1/realtime to a WebSocket. Auth uses the
// OpenAI WS subprotocol (not the Bearer header), so this route sits outside
// the v1 group's authAPIKey middleware.
func (s *Server) handleRealtime(c *gin.Context) {
	plain := realtimeKeyFromProtocols(c.Request.Header["Sec-Websocket-Protocol"])
	if plain == "" {
		abortWith(c, http.StatusUnauthorized, "invalid_api_key",
			"realtime auth: use Sec-WebSocket-Protocol openai-insecure-api-key.<key>")
		return
	}
	key, err := s.store.GetAPIKeyByHash(c.Request.Context(), store.HashKey(plain))
	if errors.Is(err, store.ErrNotFound) {
		abortWith(c, http.StatusUnauthorized, "invalid_api_key", "API key not recognized")
		return
	}
	if err != nil {
		httpErr(c, err)
		return
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now()) {
		abortWith(c, http.StatusUnauthorized, "expired_api_key", "API key expired")
		return
	}
	if s.limiter.Enabled() {
		if ok, which := s.limiter.AllowRequest(key.ID); !ok {
			abortWith(c, http.StatusTooManyRequests, "rate_limited", rateLimitMessage(which))
			return
		}
	}

	upgrader := websocket.Upgrader{ReadBufferSize: 8192, WriteBufferSize: 8192}
	// Echo the auth subprotocol so the OpenAI SDK handshake completes.
	upResp := http.Header{}
	upResp.Set("Sec-WebSocket-Protocol", realtimeProtoPrefix+plain)
	client, err := upgrader.Upgrade(c.Writer, c.Request, upResp)
	if err != nil {
		slog.Warn("realtime upgrade", "error", err)
		return
	}
	defer client.Close()

	rt := &realtimeSession{s: s, key: key, client: client}
	rt.run(c.Request.Context())
}
