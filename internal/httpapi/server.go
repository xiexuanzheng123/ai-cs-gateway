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
	// AI 服务客户端，通过 HTTP 调用 Python AI Service。
	aiClient := ai.NewClient(cfg.AIServiceBaseURL, time.Duration(cfg.AIServiceTimeoutSeconds)*time.Second)
	// 客服服务，负责编排 AI 回复逻辑。核心业务：Send、反馈、RAG、规则…
	chatService := chat.NewService(aiClient, routing.NewRiskRouter(), chatStore)
	if err := chatService.ReloadRules(context.Background()); err != nil {
		log.Printf("load rules: %v", err)
	}
	// 客服 handler，负责 HTTP 接口转换。绑 JSON、调 Service、写 JSON 响应
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
	router.POST("/api/customer-service/rag/search", chatHandler.SearchRAG)

	// 后台配置接口：当前先放在 H5 管理页里，后续可独立成真正的管理后台。
	router.GET("/api/customer-service/admin/rules", chatHandler.ListRules)
	router.POST("/api/customer-service/admin/rules", chatHandler.CreateRule)
	router.PUT("/api/customer-service/admin/rules/:id", chatHandler.UpdateRule)
	router.POST("/api/customer-service/admin/rules/reload", chatHandler.ReloadRules)
	router.GET("/api/customer-service/admin/dashboard", chatHandler.Dashboard)
	router.GET("/api/customer-service/admin/trace-logs", chatHandler.ListTraceLogs)
	router.GET("/api/customer-service/admin/flags", chatHandler.ListFeatureFlags)
	router.PUT("/api/customer-service/admin/flags/:key", chatHandler.SetFeatureFlag)
	router.GET("/api/customer-service/admin/categories", chatHandler.ListCategories)
	router.POST("/api/customer-service/admin/categories", chatHandler.CreateCategory)
	router.PUT("/api/customer-service/admin/categories/:id", chatHandler.UpdateCategory)
	router.DELETE("/api/customer-service/admin/categories/:id", chatHandler.DeleteCategory)
	router.GET("/api/customer-service/admin/knowledge", chatHandler.ListKnowledge)
	router.POST("/api/customer-service/admin/knowledge", chatHandler.CreateKnowledge)
	router.PUT("/api/customer-service/admin/knowledge/:id", chatHandler.UpdateKnowledge)
	router.POST("/api/customer-service/admin/knowledge/chunks/sync", chatHandler.SyncKnowledgeChunks)
	router.GET("/api/customer-service/admin/rag-eval-cases", chatHandler.ListRAGEvalCases)
	router.POST("/api/customer-service/admin/rag-eval-cases", chatHandler.CreateRAGEvalCase)
	router.PUT("/api/customer-service/admin/rag-eval-cases/:id", chatHandler.UpdateRAGEvalCase)

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
