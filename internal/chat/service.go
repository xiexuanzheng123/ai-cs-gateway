package chat

import (
	"context"

	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
)

type SendMessageRequest struct {
	SessionID string            `json:"session_id"`
	UserID    string            `json:"user_id" binding:"required"`
	Message   string            `json:"message" binding:"required"`
	Source    string            `json:"source"`
	Metadata  map[string]string `json:"metadata"`
}

type SendMessageResponse struct {
	SessionID       string   `json:"session_id"`
	MessageID       string   `json:"message_id"`
	Reply           string   `json:"reply"`
	ReplyType       string   `json:"reply_type"`
	TransferToHuman bool     `json:"transfer_to_human"`
	RiskLevel       string   `json:"risk_level"`
	Intent          string   `json:"intent"`
	Suggestions     []string `json:"suggestions"`
}

type Service struct {
	aiClient   *ai.Client
	riskRouter *routing.RiskRouter
}

func NewService(aiClient *ai.Client, riskRouter *routing.RiskRouter) *Service {
	return &Service{aiClient: aiClient, riskRouter: riskRouter}
}

func (s *Service) Send(ctx context.Context, request SendMessageRequest) (SendMessageResponse, error) {
	sessionID := firstNonEmpty(request.SessionID, newID("session"))
	messageID := newID("message")

	if result, matched := s.riskRouter.Match(request.Message); matched {
		return SendMessageResponse{
			SessionID:       sessionID,
			MessageID:       messageID,
			Reply:           result.Reply,
			ReplyType:       "handoff",
			TransferToHuman: true,
			RiskLevel:       result.RiskLevel,
			Intent:          result.Intent,
			Suggestions:     result.Suggestions,
		}, nil
	}

	aiResponse, err := s.aiClient.Reply(ctx, ai.ReplyRequest{
		SessionID:       sessionID,
		UserID:          request.UserID,
		Message:         request.Message,
		History:         []ai.HistoryItem{},
		BusinessContext: map[string]any{},
	})
	if err != nil {
		return SendMessageResponse{}, err
	}

	return SendMessageResponse{
		SessionID:       sessionID,
		MessageID:       messageID,
		Reply:           aiResponse.Reply,
		ReplyType:       "answer",
		TransferToHuman: aiResponse.TransferToHuman,
		RiskLevel:       aiResponse.RiskLevel,
		Intent:          aiResponse.Intent,
		Suggestions:     aiResponse.Suggestions,
	}, nil
}

func firstNonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
