package httpapi

import (
	"net/http"
	"time"

	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/chat"
	"ai-cs-gateway/internal/config"
	"ai-cs-gateway/internal/routing"

	"github.com/gin-gonic/gin"
)

func NewServer(cfg config.Config) *gin.Engine {
	router := gin.Default()

	aiClient := ai.NewClient(cfg.AIServiceBaseURL, 3*time.Second)
	chatService := chat.NewService(aiClient, routing.NewRiskRouter())
	chatHandler := chat.NewHandler(chatService)

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/api/chat/send", chatHandler.Send)

	return router
}
