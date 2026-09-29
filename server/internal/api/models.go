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
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": data})
}