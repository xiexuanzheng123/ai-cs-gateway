package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/chat"
	"ai-cs-gateway/internal/config"
	"ai-cs-gateway/internal/routing"
	"ai-cs-gateway/internal/storage"

	"github.com/gin-gonic/gin"
)

func NewServer(cfg config.Config) *gin.Engine {
	router := gin.Default()

	chatStore := chat.Store(chat.NoopStore{})
	var mysqlDB *sql.DB
	if cfg.MySQLDSN != "" {
		db, err := storage.OpenMySQL(cfg.MySQLDSN)
		if err != nil {
			log.Fatalf("connect mysql: %v", err)
		}
		mysqlDB = db
		chatStore = chat.NewMySQLStore(db)
	}

	var redisCheck func(context.Context) error
	if cfg.RedisAddr != "" {
		redisClient, err := storage.OpenRedis(context.Background(), cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err != nil {
			log.Fatalf("connect redis: %v", err)
		}
		redisCheck = func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		}
	}

	aiClient := ai.NewClient(cfg.AIServiceBaseURL, 3*time.Second)
	chatService := chat.NewService(aiClient, routing.NewRiskRouter(), chatStore)
	if err := chatService.ReloadRules(context.Background()); err != nil {
		log.Printf("load rules: %v", err)
	}
	chatHandler := chat.NewHandler(chatService)

	router.GET("/api/customer-service/health", func(c *gin.Context) {
		mysqlStatus := checkStatus(c.Request.Context(), mysqlDB == nil, func(ctx context.Context) error {
			return mysqlDB.PingContext(ctx)
		})
		redisStatus := checkStatus(c.Request.Context(), redisCheck == nil, redisCheck)

		status := "ok"
		httpStatus := http.StatusOK
		if mysqlStatus == "error" || redisStatus == "error" {
			status = "error"
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status": status,
			"mysql":  mysqlStatus,
			"redis":  redisStatus,
		})
	})

	router.POST("/api/customer-service/chat", chatHandler.Send)
	router.POST("/api/customer-service/feedback", chatHandler.Feedback)
	router.POST("/api/customer-service/handoff", chatHandler.Handoff)
	router.GET("/api/customer-service/admin/rules", chatHandler.ListRules)
	router.POST("/api/customer-service/admin/rules", chatHandler.CreateRule)
	router.PUT("/api/customer-service/admin/rules/:id", chatHandler.UpdateRule)
	router.POST("/api/customer-service/admin/rules/reload", chatHandler.ReloadRules)
	router.GET("/api/customer-service/admin/dashboard", chatHandler.Dashboard)
	router.GET("/api/customer-service/admin/flags", chatHandler.ListFeatureFlags)
	router.PUT("/api/customer-service/admin/flags/:key", chatHandler.SetFeatureFlag)

	return router
}

func checkStatus(ctx context.Context, disabled bool, check func(context.Context) error) string {
	if disabled {
		return "disabled"
	}
	if err := check(ctx); err != nil {
		return "error"
	}
	return "ok"
}
