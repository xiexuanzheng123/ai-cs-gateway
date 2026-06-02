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
	chatHandler := chat.NewHandler(chatService)

	router.GET("/healthz", func(c *gin.Context) {
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

	router.POST("/api/chat/send", chatHandler.Send)

	router.POST("/api/customer-service/feedback", chatHandler.Feedback)

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
