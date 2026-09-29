package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"modelhub/internal/billing"
	"modelhub/internal/gateway"
	"modelhub/internal/store"
)

// DramaTimeout bounds a whole drama (storyboard call + every shot).
const DramaTimeout = 30 * time.Minute

// Shot is one storyboard panel in a drama. The serialized shots array is the
// exportable asset pack (the frontend's "download JSON").
type Shot struct {
	ShotNo      int    `json:"shot_no"`
	Scene       string `json:"scene"`
	Dialogue    string `json:"dialogue"`
	ImagePrompt string `json:"image_prompt"`
	Status      string `json:"status"` // pending | running | succeeded | failed
	ImageURL    string `json:"image_url,omitempty"`
	AudioURL    string `json:"audio_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DramaWorker runs queued dramas: storyboard the script (LLM or naive split),
// then generate each shot's image (and optional TTS) through the M4 adapters.
// Billing is all-or-nothing: success settles the whole hold, any failure
// releases it. The drama UUID is the ledger request ID. After settlement the
// worker composes the shots into one MP4 (P2-1b; best effort — a missing
// ffmpeg or a failed render only leaves video_url empty).
type DramaWorker struct {
	store    *store.Store
	provider *gateway.Provider
	composer *Composer
}

// NewDramaWorker builds a drama worker.
func NewDramaWorker(st *store.Store, p *gateway.Provider, composer *Composer) *DramaWorker {
	return &DramaWorker{store: st, provider: p, composer: composer}
}

// Run blocks until ctx is cancelled; it first recovers dramas a dead worker
// left running (fail + release so funds are never frozen by a crash).
func (w *DramaWorker) Run(ctx context.Context) {
	w.recoverStale(ctx)
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.processNext(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("process drama", "error", err)
			}
		}
	}
}

func (w *DramaWorker) recoverStale(ctx context.Context) {
	dramas, err := w.store.ListRunningDramas(ctx, 1000)
	if err != nil {
		slog.Warn("recover stale dramas", "error", err)
		return
	}
	for i := range dramas {
		d := &dramas[i]
		w.failAndRelease(ctx, d, "worker restarted; drama interrupted")
		slog.Warn("recovered stale drama", "drama", d.DramaUUID)
	}
}

func (w *DramaWorker) processNext(ctx context.Context) error {
	d, err := w.store.ClaimNextQueuedDrama(ctx)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	w.runDrama(ctx, d)
	return nil
}

func (w *DramaWorker) runDrama(ctx context.Context, d *store.Drama) {
	reason := "drama:" + d.ImageModel
	runCtx, cancel := context.WithTimeout(ctx, DramaTimeout)
	defer cancel()

	model, err := w.store.GetModel(runCtx, d.ImageModel)
	if err != nil || !model.Enabled {
		w.failAndRelease(ctx, d, "image model no longer available: "+d.ImageModel)
		return
	}
	ch, err := w.store.PickChannel(runCtx, d.ImageModel)
	if err != nil {
		w.failAndRelease(ctx, d, "no enabled channel for image model "+d.ImageModel)
		return
	}
	ad := adapterFor(ch.Provider, "image")
	if ad == nil {
		w.failAndRelease(ctx, d, fmt.Sprintf("provider %q does not support image generation", ch.Provider))
		return
	}

	var ttsModel *store.Model
	var chTTS *store.Channel
	var adTTS Adapter
	if d.TTSModel != nil && *d.TTSModel != "" {
		ttsModel, err = w.store.GetModel(runCtx, *d.TTSModel)
		if err != nil || !ttsModel.Enabled {
			w.failAndRelease(ctx, d, "tts model no longer available: "+*d.TTSModel)
			return
		}
		chTTS, err = w.store.PickChannel(runCtx, *d.TTSModel)
		if err != nil {
			w.failAndRelease(ctx, d, "no enabled channel for tts model "+*d.TTSModel)
			return
		}
		adTTS = adapterFor(chTTS.Provider, "tts")
		if adTTS == nil {
			w.failAndRelease(ctx, d, fmt.Sprintf("provider %q does not support tts", chTTS.Provider))
			return
		}
	}

	shots, err := w.storyboard(runCtx, d)
	if err != nil {
		w.failAndRelease(ctx, d, "storyboard failed: "+err.Error())
		return
	}
	w.saveProgress(ctx, d, shots)

	for i := range shots {
		if runCtx.Err() != nil {
			w.failAndRelease(ctx, d, "drama timed out")
			return
		}
		shot := &shots[i]
		shot.Status = "running"
		w.saveProgress(ctx, d, shots)

		imgPayload, _ := json.Marshal(map[string]string{"prompt": shot.ImagePrompt, "size": "1024x1024"})
		imgCtx, imgCancel := context.WithTimeout(runCtx, TaskTimeout)
		urls, err := ad.Run(imgCtx, w.provider, ch, model, imgPayload)
		imgCancel()
		if err != nil {
			shot.Status = "failed"
			shot.Error = err.Error()
			w.saveProgress(ctx, d, shots)
			w.failAndRelease(ctx, d, fmt.Sprintf("shot %d image failed: %s", shot.ShotNo, err.Error()))
			return
		}
		if len(urls) == 0 {
			shot.Status = "failed"
			shot.Error = "upstream returned no image url"
			w.saveProgress(ctx, d, shots)
			w.failAndRelease(ctx, d, fmt.Sprintf("shot %d image returned no url", shot.ShotNo))
			return
		}
		shot.ImageURL = urls[0]

		if adTTS != nil && strings.TrimSpace(shot.Dialogue) != "" {
			ttsPayload, _ := json.Marshal(map[string]string{"text": shot.Dialogue})
			ttsCtx, ttsCancel := context.WithTimeout(runCtx, TaskTimeout)
			audioURLs, err := adTTS.Run(ttsCtx, w.provider, chTTS, ttsModel, ttsPayload)
			ttsCancel()
			if err != nil {
				shot.Status = "failed"
				shot.Error = err.Error()
				w.saveProgress(ctx, d, shots)
				w.failAndRelease(ctx, d, fmt.Sprintf("shot %d tts failed: %s", shot.ShotNo, err.Error()))
				return
			}
			if len(audioURLs) > 0 {
				shot.AudioURL = audioURLs[0]
			}
		}

		shot.Status = "succeeded"
		w.saveProgress(ctx, d, shots)
	}

	sc, cancel2 := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel2()
	if err := w.store.CompleteDrama(sc, d.DramaUUID, billing.USD(d.HoldMicro)); err != nil {
		slog.Error("complete drama", "drama", d.DramaUUID, "error", err)
		return
	}
	if err := w.store.SettleFunds(sc, d.KeyUserID, d.HoldMicro, d.HoldMicro, d.DramaUUID, reason, d.APIKeyID); err != nil {
		slog.Error("settle drama funds", "drama", d.DramaUUID, "error", err)
	}

	// P2-1b: compose the MP4 after billing is closed — a render failure can
	// never affect money, and upstream result URLs expire, so we do it while
	// they are still fresh.
	w.composeVideo(ctx, d, shots)
}

// composeVideo renders the final MP4 and records video_url (or the reason it
// is missing). Never fails the drama: the asset pack already succeeded.
func (w *DramaWorker) composeVideo(ctx context.Context, d *store.Drama, shots []Shot) {
	if w.composer == nil || !w.composer.Available() {
		w.setVideo(ctx, d.DramaUUID, "", "ffmpeg not installed on the server host")
		return
	}
	composeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ComposeTimeout)
	defer cancel()
	outPath := w.composer.VideoPath(d.DramaUUID)
	slog.Info("composing drama video", "drama", d.DramaUUID, "shots", len(shots))
	if err := w.composer.Compose(composeCtx, shots, outPath); err != nil {
		slog.Warn("compose drama video", "drama", d.DramaUUID, "error", err)
		w.setVideo(ctx, d.DramaUUID, "", err.Error())
		return
	}
	w.setVideo(ctx, d.DramaUUID, "/media/dramas/"+d.DramaUUID+".mp4", "")
	slog.Info("drama video ready", "drama", d.DramaUUID)
}

func (w *DramaWorker) setVideo(ctx context.Context, dramaUUID, videoURL, videoErr string) {
	sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.store.SetDramaVideo(sc, dramaUUID, videoURL, videoErr); err != nil {
		slog.Warn("set drama video", "drama", dramaUUID, "error", err)
	}
}

// failAndRelease marks the drama failed and releases the whole hold
// (all-or-nothing: a partial drama is never billed).
func (w *DramaWorker) failAndRelease(ctx context.Context, d *store.Drama, msg string) {
	sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := w.store.FailDrama(sc, d.DramaUUID, msg); err != nil {
		if err != store.ErrNotFound {
			slog.Error("fail drama", "drama", d.DramaUUID, "error", err)
		}
		return
	}
	if d.KeyUserID != nil && d.HoldMicro > 0 {
		if err := w.store.ReleaseFunds(sc, *d.KeyUserID, d.HoldMicro, d.DramaUUID, "drama:"+d.ImageModel); err != nil {
			slog.Error("release drama funds", "drama", d.DramaUUID, "error", err)
		}
	}
}

func (w *DramaWorker) saveProgress(ctx context.Context, d *store.Drama, shots []Shot) {
	sc, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	shotsJSON, err := json.Marshal(shots)
	if err != nil {
		return
	}
	if err := w.store.UpdateDramaShots(sc, d.DramaUUID, shotsJSON); err != nil && err != store.ErrNotFound {
		slog.Warn("save drama progress", "drama", d.DramaUUID, "error", err)
	}
}

// --- storyboard ---

type storyboardShot struct {
	Scene       string `json:"scene"`
	Dialogue    string `json:"dialogue"`
	ImagePrompt string `json:"image_prompt"`
}

func storyboardSystemPrompt() string {
	return "你是漫剧分镜师。把用户给的剧本拆分成指定数量的镜头，只输出 JSON，不要输出任何 JSON 以外的文字。" +
		"格式：{\"shots\":[{\"scene\":\"画面描述（一句话）\",\"dialogue\":\"该镜头的台词或旁白（一句话，可为空字符串）\"," +
		"\"image_prompt\":\"该镜头的英文图像生成提示词（包含风格、主体、动作、光影、构图，不要包含引号）\"}]}"
}

// storyboard turns the script into shots, either via the configured chat
// model (strict JSON) or a naive sentence split when none was given.
func (w *DramaWorker) storyboard(ctx context.Context, d *store.Drama) ([]Shot, error) {
	var raw []storyboardShot
	if d.StoryboardModel != "" {
		m, err := w.store.GetModel(ctx, d.StoryboardModel)
		if err != nil || !m.Enabled {
			return nil, fmt.Errorf("storyboard model unavailable: %s", d.StoryboardModel)
		}
		ch, err := w.store.PickChannel(ctx, d.StoryboardModel)
		if err != nil {
			return nil, fmt.Errorf("no channel for storyboard model %s", d.StoryboardModel)
		}
		body, _ := json.Marshal(map[string]any{
			"model":       m.UpstreamModel,
			"stream":      false,
			"temperature": 0.7,
			"messages": []map[string]string{
				{"role": "system", "content": storyboardSystemPrompt()},
				{"role": "user", "content": fmt.Sprintf("风格：%s\n镜头数：%d\n剧本：\n%s", d.Style, d.ShotsPlanned, d.Script)},
			},
		})
		status, resp, err := w.provider.DoJSON(ctx, ch, http.MethodPost, "/chat/completions", body, nil)
		if err != nil {
			return nil, fmt.Errorf("storyboard upstream call: %w", err)
		}
		if status >= 400 {
			return nil, fmt.Errorf("storyboard upstream HTTP %d: %s", status, truncate(resp, 300))
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(resp, &parsed); err != nil || len(parsed.Choices) == 0 {
			return nil, fmt.Errorf("storyboard response not parseable")
		}
		rawJSON := extractJSON(parsed.Choices[0].Message.Content)
		if rawJSON == nil {
			return nil, fmt.Errorf("no JSON object found in storyboard response")
		}
		if err := json.Unmarshal(rawJSON, &raw); err != nil {
			return nil, fmt.Errorf("storyboard is not valid JSON: %w", err)
		}
	} else {
		raw = naiveSplit(d.Script, d.Style, d.ShotsPlanned)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("no shots generated from script")
	}
	if len(raw) > d.ShotsPlanned {
		raw = raw[:d.ShotsPlanned]
	}
	out := make([]Shot, 0, len(raw))
	for i, s := range raw {
		if strings.TrimSpace(s.ImagePrompt) == "" {
			return nil, fmt.Errorf("shot %d has an empty image_prompt", i+1)
		}
		out = append(out, Shot{
			ShotNo:      i + 1,
			Scene:       s.Scene,
			Dialogue:    s.Dialogue,
			ImagePrompt: s.ImagePrompt,
			Status:      "pending",
		})
	}
	return out, nil
}

// extractJSON pulls the first JSON object out of an LLM reply, tolerating
// markdown code fences and surrounding prose.
func extractJSON(content string) []byte {
	s := strings.TrimSpace(content)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+len("```"):]
		if j := strings.Index(rest, "\n"); j >= 0 {
			rest = rest[j+1:]
		}
		if k := strings.Index(rest, "```"); k >= 0 {
			rest = rest[:k]
		}
		s = strings.TrimSpace(rest)
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return nil
	}
	return []byte(s[start : end+1])
}

// naiveSplit is the fallback storyboarder when no chat model is configured:
// sentences/paragraphs become shots, and each shot's image prompt is the
// style plus the scene text.
func naiveSplit(script, style string, n int) []storyboardShot {
	var parts []string
	for _, p := range strings.FieldsFunc(script, func(r rune) bool {
		return r == '\n' || r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?'
	}) {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	if n <= 0 {
		n = len(parts)
	}
	if len(parts) > n {
		per := (len(parts) + n - 1) / n
		merged := make([]string, 0, n)
		for i := 0; i < len(parts); i += per {
			end := i + per
			if end > len(parts) {
				end = len(parts)
			}
			merged = append(merged, strings.Join(parts[i:end], ""))
		}
		parts = merged
	}
	prefix := ""
	if style != "" {
		prefix = style + ", "
	}
	out := make([]storyboardShot, 0, len(parts))
	for _, p := range parts {
		out = append(out, storyboardShot{Scene: p, Dialogue: p, ImagePrompt: prefix + p})
	}
	return out
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}