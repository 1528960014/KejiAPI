package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handlePublicRankings serves the public model rankings page (no auth):
//
//	GET /api/rankings?period=today|week|month|year
//
// Returns top-12 models by tokens with requests/cost/period-over-period
// change, the period total, and a bucket × model matrix for the stacked
// bar chart.
func (s *Server) handlePublicRankings(c *gin.Context) {
	period := c.DefaultQuery("period", "today")
	switch period {
	case "today", "week", "month", "year":
	default:
		period = "today"
	}
	res, err := s.store.ModelRankings(c.Request.Context(), period)
	if err != nil {
		httpErr(c, err)
		return
	}
	entries := make([]gin.H, 0, len(res.Models))
	for _, m := range res.Models {
		change := 0.0
		if m.PrevTokens > 0 {
			change = (float64(m.Tokens) - float64(m.PrevTokens)) / float64(m.PrevTokens) * 100
		}
		entries = append(entries, gin.H{
			"model_id":   m.ModelID,
			"provider":   m.Provider,
			"tokens":     m.Tokens,
			"requests":   m.Requests,
			"cost_micro": m.CostMicro,
			"change_pct": change,
		})
	}
	buckets := make([]gin.H, 0, len(res.Buckets))
	for _, b := range res.Buckets {
		buckets = append(buckets, gin.H{
			"bucket":   b.Bucket,
			"model_id": b.ModelID,
			"tokens":   b.Tokens,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"period":       res.Period,
		"total_tokens": res.TotalTokens,
		"models":       entries,
		"buckets":      buckets,
	}})
}