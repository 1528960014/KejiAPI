package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
}