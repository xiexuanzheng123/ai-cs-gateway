package main

import (
	"log"

	"ai-cs-gateway/internal/config"
	"ai-cs-gateway/internal/httpapi"
)

func main() {
	cfg := config.Load()
	server := httpapi.NewServer(cfg)

	if err := server.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
