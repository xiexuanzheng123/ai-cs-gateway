package config

import (
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr         string
	AIServiceBaseURL string
	MySQLDSN         string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
}

func Load() Config {
	return Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		AIServiceBaseURL: env("AI_SERVICE_BASE_URL", "http://localhost:8000"),
		MySQLDSN:         os.Getenv("MYSQL_DSN"),
		RedisAddr:        os.Getenv("REDIS_ADDR"),
		RedisPassword:    os.Getenv("REDIS_PASSWORD"),
		RedisDB:          envInt("REDIS_DB", 0),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
