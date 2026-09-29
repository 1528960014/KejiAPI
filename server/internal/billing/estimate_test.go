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
