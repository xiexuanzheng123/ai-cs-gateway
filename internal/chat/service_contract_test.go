package chat

import (
	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
	"context"
	"testing"
)

type contractAIClient struct{}

func (c contractAIClient) Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error) {
	return ai.ReplyResponse{
		Reply:           "AI reply",
		Intent:          "general",
		RiskLevel:       "low",
		TransferToHuman: false,
		Suggestions:     []string{"转人工"},
	}, nil
}

func TestSendUsesDocumentChatContract(t *testing.T) {
	store := &contractStore{}
	service := NewService(contractAIClient{}, routing.NewRiskRouter(), store)

	response, err := service.Send(context.Background(), SendMessageRequest{
		ConversationID: "c_001",
		UserID:         "u_001",
		MessageID:      "m_001",
		MessageType:    "text",
		Message:        "你好",
		Channel:        "h5",
	})
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if response.TraceID == "" {
		t.Fatal("expected trace id")
	}
	if response.ConversationID != "c_001" {
		t.Fatalf("expected conversation_id c_001, got %q", response.ConversationID)
	}
	if response.ResponseType != "answer" {
		t.Fatalf("expected response_type answer, got %q", response.ResponseType)
	}
	if response.Content.Text == "" {
		t.Fatal("expected content text")
	}
	if response.Handoff.Required {
		t.Fatal("expected greeting not to require handoff")
	}
	if len(response.Content.Buttons) == 0 {
		t.Fatal("expected response buttons")
	}
	if store.messages[0].MessageType != "text" {
		t.Fatalf("expected user message type text, got %q", store.messages[0].MessageType)
	}
}

func TestSendGuidesImageAndAudioMessages(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
		wantIntent  string
	}{
		{name: "image", messageType: "image", wantIntent: "media_image_guide"},
		{name: "audio", messageType: "audio", wantIntent: "media_audio_guide"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(contractAIClient{}, routing.NewRiskRouter(), &contractStore{})
			response, err := service.Send(context.Background(), SendMessageRequest{
				ConversationID: "c_" + tt.name,
				UserID:         "u_001",
				MessageID:      "m_" + tt.name,
				MessageType:    tt.messageType,
				Message:        "media payload",
				Channel:        "h5",
			})
			if err != nil {
				t.Fatalf("send failed: %v", err)
			}
			if response.ResponseType != "guide" {
				t.Fatalf("expected guide response, got %q", response.ResponseType)
			}
			if response.Handoff.Required {
				t.Fatal("expected media guide not to require handoff")
			}
			if len(response.Content.Buttons) == 0 {
				t.Fatal("expected guide buttons")
			}
		})
	}
}

type contractStore struct {
	NoopStore
	messages []MessageRecord
}

func (s *contractStore) SaveMessage(ctx context.Context, record MessageRecord) error {
	s.messages = append(s.messages, record)
	return nil
}
