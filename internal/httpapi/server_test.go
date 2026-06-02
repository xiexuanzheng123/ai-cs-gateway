package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-cs-gateway/internal/config"

	"github.com/gin-gonic/gin"
)

func TestHealthzReportsDisabledDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := NewServer(config.Config{
		AIServiceBaseURL: "http://localhost:8000",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	assertHealthField(t, body, "status", "ok")
	assertHealthField(t, body, "mysql", "disabled")
	assertHealthField(t, body, "redis", "disabled")
}

func assertHealthField(t *testing.T, body map[string]string, key string, expected string) {
	t.Helper()
	if body[key] != expected {
		t.Fatalf("expected %s=%q, got %q", key, expected, body[key])
	}
}
