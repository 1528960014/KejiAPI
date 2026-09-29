package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleModels is the OpenAI-compatible model list.
func (s *Server) handleModels(c *gin.Context) {
	models, err := s.store.ListModels(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := []gin.H{}
	for _, m := range models {
		if !m.Enabled {
			continue
		}
		data = append(data, gin.H{
			"id":       m.ModelID,
			"object":   "model",
			"created":  0,
			"owned_by": m.Provider,
			// M3: non-OpenAI extras (ignored by OpenAI SDKs) so the pricing
			// page can show live prices without an admin call.
			"input_price_per_1k":  m.InputPricePer1k,
			"output_price_per_1k": m.OutputPricePer1k,
			// M4: per-item pricing for media models (image/video/tts/music).
			"price_unit": m.PriceUnit,
			"unit_price": m.UnitPrice,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}

// handlePublicModels lists enabled models with prices for the pricing page.
func (s *Server) handlePublicModels(c *gin.Context) {
	models, err := s.store.ListModels(c.Request.Context())
	if err != nil {
		httpErr(c, err)
		return
	}
	data := make([]gin.H, 0, len(models))
	for i := range models {
		m := &models[i]
		if !m.Enabled {
			continue
		}
		data = append(data, gin.H{
			"id":                  m.ModelID,
			"provider":            m.Provider,
			"capabilities":        m.Capabilities,
			"input_price_per_1k":  m.InputPricePer1k,
			"output_price_per_1k": m.OutputPricePer1k,
			"price_unit":          m.PriceUnit,
			"unit_price":          m.UnitPrice,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}
