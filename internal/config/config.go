package config

import "os"

type Config struct {
	HTTPAddr         string
	AIServiceBaseURL string
	MySQLDSN         string
}

func Load() Config {
	return Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		AIServiceBaseURL: env("AI_SERVICE_BASE_URL", "http://localhost:8000"),
		MySQLDSN:         os.Getenv("MYSQL_DSN"),
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
