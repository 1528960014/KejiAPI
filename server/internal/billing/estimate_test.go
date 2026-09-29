package billing

import (
	"testing"

	"modelhub/internal/store"
)

func TestEstimateTokensSimple(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"max_tokens":50}`)
	prompt, completion := EstimateTokens(body)
	if prompt != 1 { // 2 bytes -> ceil(2/4) = 1
		t.Errorf("prompt = %d, want 1", prompt)
	}
	if completion != 50 {
		t.Errorf("completion = %d, want 50", completion)
	}
}

func TestEstimateTokensDefaultsAndCap(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hello"}]}`)
	_, completion := EstimateTokens(body)
	if completion != DefaultCompletionTokens {
		t.Errorf("completion = %d, want %d", completion, DefaultCompletionTokens)
	}

	body = []byte(`{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":999999}`)
	_, completion = EstimateTokens(body)
	if completion != MaxCompletionTokens {
		t.Errorf("completion = %d, want %d", completion, MaxCompletionTokens)
	}
}

func TestEstimateTokensContentParts(t *testing.T) {
	// 5 + 5 = 10 bytes -> ceil(10/4) = 3
	body := []byte(`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}],"max_completion_tokens":7}`)
	prompt, completion := EstimateTokens(body)
	if prompt != 3 {
		t.Errorf("prompt = %d, want 3", prompt)
	}
	if completion != 7 {
		t.Errorf("completion = %d, want 7", completion)
	}
}

func TestEstimateTokensInvalidBody(t *testing.T) {
	prompt, completion := EstimateTokens([]byte(`not json`))
	if prompt != 0 || completion != DefaultCompletionTokens {
		t.Errorf("got (%d, %d), want (0, %d)", prompt, completion, DefaultCompletionTokens)
	}
}

func TestCostMicro(t *testing.T) {
	m := &store.Model{InputPricePer1k: 0.15, OutputPricePer1k: 0.6}
	if got := CostMicro(m, 1000, 2000); got != 1_350_000 {
		t.Errorf("CostMicro = %d, want 1350000", got)
	}
	// rounds up: 1 prompt token @ 0.15/1k + 1 completion @ 0.6/1k = 0.00075 USD
	if got := CostMicro(m, 1, 1); got != 750 {
		t.Errorf("CostMicro = %d, want 750", got)
	}
	// sub-micro costs round up to at least 1 micro when nonzero
	micro := &store.Model{InputPricePer1k: 0.000001, OutputPricePer1k: 0}
	if got := CostMicro(micro, 1, 0); got != 1 {
		t.Errorf("CostMicro = %d, want 1", got)
	}
	if got := CostMicro(nil, 10, 10); got != 0 {
		t.Errorf("CostMicro(nil) = %d, want 0", got)
	}
}

func TestUSDConversions(t *testing.T) {
	if USD(1_350_000) != 1.35 {
		t.Errorf("USD(1350000) = %v, want 1.35", USD(1_350_000))
	}
	if ToMicro(10.5) != 10_500_000 {
		t.Errorf("ToMicro(10.5) = %d, want 10500000", ToMicro(10.5))
	}
}

func TestMediaCostMicro(t *testing.T) {
	m := &store.Model{PriceUnit: "image", UnitPrice: 0.02}
	if got := MediaCostMicro(m, 2); got != 40_000 {
		t.Errorf("MediaCostMicro = %d, want 40000", got)
	}
	// rounds up: 0.0000005 USD -> 1 micro
	cheap := &store.Model{PriceUnit: "tts", UnitPrice: 0.0000005}
	if got := MediaCostMicro(cheap, 1); got != 1 {
		t.Errorf("MediaCostMicro(cheap) = %d, want 1", got)
	}
	// token-priced models are not billed per item
	token := &store.Model{PriceUnit: "token", InputPricePer1k: 0.15}
	if got := MediaCostMicro(token, 3); got != 0 {
		t.Errorf("MediaCostMicro(token model) = %d, want 0", got)
	}
	if got := MediaCostMicro(nil, 3); got != 0 {
		t.Errorf("MediaCostMicro(nil) = %d, want 0", got)
	}
	if got := MediaCostMicro(m, 0); got != 0 {
		t.Errorf("MediaCostMicro(n=0) = %d, want 0", got)
	}
}

func TestDramaCostMicro(t *testing.T) {
	image := &store.Model{PriceUnit: "image", UnitPrice: 0.02}
	tts := &store.Model{PriceUnit: "tts", UnitPrice: 0.005}
	// 8 shots x (0.02 + 0.005) = 0.2 USD = 200000 micro
	if got := DramaCostMicro(image, tts, 8); got != 200_000 {
		t.Errorf("DramaCostMicro(image+tts, 8) = %d, want 200000", got)
	}
	// no TTS: 8 x 0.02 = 160000 micro
	if got := DramaCostMicro(image, nil, 8); got != 160_000 {
		t.Errorf("DramaCostMicro(image only, 8) = %d, want 160000", got)
	}
	if got := DramaCostMicro(nil, tts, 8); got != 0 {
		t.Errorf("DramaCostMicro(nil image) = %d, want 0", got)
	}
	if got := DramaCostMicro(image, tts, 0); got != 0 {
		t.Errorf("DramaCostMicro(shots=0) = %d, want 0", got)
	}
}
