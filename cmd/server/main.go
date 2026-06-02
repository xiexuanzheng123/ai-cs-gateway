package main

import (
	"log"

	"ai-cs-gateway/internal/config"
	"ai-cs-gateway/internal/httpapi"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	server := httpapi.NewServer(cfg)

	if err := server.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
