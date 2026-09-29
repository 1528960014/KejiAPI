package billing

import (
	"math"

	"modelhub/internal/store"
)

// Cost computes the estimated cost of a chat completion in USD from the
// per-1K-token prices and observed token counts.
func Cost(m *store.Model, promptTokens, completionTokens int) float64 {
	if m == nil {
		return 0
	}
	return (float64(promptTokens)/1000.0)*m.InputPricePer1k +
		(float64(completionTokens)/1000.0)*m.OutputPricePer1k
}

// ApplyRate discounts a micro-USD amount by an agent's wholesale rate
// (0 < rate <= 1), rounding up. A nil rate or rate >= 1 is the identity.
// A positive input never discounts to zero (minimum 1 micro-USD) so a
// non-free request stays billable.
func ApplyRate(micro int64, rate *float64) int64 {
	if micro == 0 || rate == nil || *rate >= 1 {
		return micro
	}
	v := int64(math.Ceil(float64(micro) * *rate))
	if v < 1 {
		v = 1
	}
	return v
}
