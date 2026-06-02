package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"

	"github.com/gin-gonic/gin"
)

type feedbackAIClient struct{}

func (f feedbackAIClient) Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error) {
	return ai.ReplyResponse{}, nil
}

type feedbackStore struct {
	NoopStore
	records []FeedbackRecord
}

func (s *feedbackStore) SaveFeedback(ctx context.Context, record FeedbackRecord) error {
	s.records = append(s.records, record)
	return nil
}

func TestFeedbackHandlerStoresFeedback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &feedbackStore{}
	service := NewService(feedbackAIClient{}, routing.NewRiskRouter(), store)
	handler := NewHandler(service)

	router := gin.New()
	router.POST("/api/customer-service/feedback", handler.Feedback)

	body, err := json.Marshal(FeedbackRequest{
		ConversationID: "session-test",
		MessageID:      "message-test",
		UserID:         "user-001",
		Rating:         "thumbs_down",
		Comment:        "没有解决",
		ActionTaken:    "handoff",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/customer-service/feedback", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", response.Code, response.Body.String())
	}
	if len(store.records) != 1 {
		t.Fatalf("expected 1 feedback record, got %d", len(store.records))
	}
	if store.records[0].Rating != "thumbs_down" {
		t.Fatalf("unexpected rating %s", store.records[0].Rating)
	}
	if response.Body.String() != `{"success":true}` {
		t.Fatalf("unexpected response body %s", response.Body.String())
	}
}
