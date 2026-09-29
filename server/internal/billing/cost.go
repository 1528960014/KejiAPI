package billing

import "modelhub/internal/store"

// Cost computes the estimated cost of a chat completion in USD from the
// per-1K-token prices and observed token counts.
func Cost(m *store.Model, promptTokens, completionTokens int) float64 {
	if m == nil {
		return 0
	}
	return (float64(promptTokens)/1000.0)*m.InputPricePer1k +
		(float64(completionTokens)/1000.0)*m.OutputPricePer1k
}