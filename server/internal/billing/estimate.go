package billing

import (
	"encoding/json"
	"math"

	"modelhub/internal/store"
)

// MicrosPerUSD is the ledger unit: all balances and amounts are integer
// micro-USD.
const MicrosPerUSD = 1_000_000

// DefaultCompletionTokens is assumed when the request pins no max_tokens, and
// MaxCompletionTokens caps the estimate so a missing cap cannot freeze a
// user's whole balance.
const (
	DefaultCompletionTokens = 1024
	MaxCompletionTokens     = 16384
)

// EstimateTokens derives rough token counts from the raw request body:
// ~4 bytes per token for prompt text (conservative over-estimate for
// CJK, which is fine because settlement uses real usage), and the pinned
// max_tokens (or the default) for the completion.
func EstimateTokens(body []byte) (prompt, completion int) {
	completion = DefaultCompletionTokens
	var req struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		MaxTokens           *int `json:"max_tokens"`
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0, completion
	}
	var bytes int
	for _, msg := range req.Messages {
		bytes += contentBytes(msg.Content)
	}
	prompt = (bytes + 3) / 4
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		completion = *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil && *req.MaxCompletionTokens > 0 {
		completion = *req.MaxCompletionTokens
	}
	if completion > MaxCompletionTokens {
		completion = MaxCompletionTokens
	}
	return prompt, completion
}

// contentBytes counts the characters carried by a message content field,
// which may be a plain string or an array of typed parts.
func contentBytes(raw json.RawMessage) int {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return len(s)
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		n := 0
		for _, p := range parts {
			n += len(p.Text)
		}
		return n
	}
	return 0
}

// CostMicro returns the exact cost of the given token counts in micro-USD,
// rounded up.
func CostMicro(m *store.Model, prompt, completion int) int64 {
	if m == nil {
		return 0
	}
	usd := float64(prompt)/1000.0*m.InputPricePer1k +
		float64(completion)/1000.0*m.OutputPricePer1k
	return int64(math.Ceil(usd * MicrosPerUSD))
}

// MediaCostMicro returns the charge for generating n media items with a
// per-unit priced model, rounded up. Token-priced models return 0: media
// generation is always billed per item via unit_price.
func MediaCostMicro(m *store.Model, n int) int64 {
	if m == nil || n <= 0 {
		return 0
	}
	return int64(math.Ceil(m.UnitPrice * float64(n) * MicrosPerUSD))
}

// DramaCostMicro returns the frozen amount for a comic drama: each planned
// shot costs one image item plus, when a TTS model is given, one TTS item.
// The drama is billed all-or-nothing (any failed shot releases the hold).
func DramaCostMicro(imageModel, ttsModel *store.Model, shots int) int64 {
	if imageModel == nil || shots <= 0 {
		return 0
	}
	per := MediaCostMicro(imageModel, 1)
	if ttsModel != nil {
		per += MediaCostMicro(ttsModel, 1)
	}
	return per * int64(shots)
}

// USD converts micro-USD to a display float.
func USD(micro int64) float64 { return float64(micro) / MicrosPerUSD }

// ToMicro converts a USD float to micro-USD, rounded up.
func ToMicro(usd float64) int64 { return int64(math.Ceil(usd * MicrosPerUSD)) }
