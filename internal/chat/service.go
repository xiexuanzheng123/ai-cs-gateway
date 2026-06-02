package chat

import (
	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
	"context"
	"time"
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

type FeedbackRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
	MessageID      string `json:"message_id" binding:"required"`
	UserID         string `json:"user_id" binding:"required"`
	Rating         string `json:"rating" binding:"required"`
	Comment        string `json:"comment"`
	ActionTaken    string `json:"action_taken"`
}

type FeedbackResponse struct {
	Success bool `json:"success"`
}

type Service struct {
	aiClient   AIClient
	riskRouter *routing.RiskRouter
	store      Store
}

type AIClient interface {
	Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error)
}

func NewService(aiClient AIClient, riskRouter *routing.RiskRouter, store Store) *Service {
	if store == nil {
		store = NoopStore{}
	}
	return &Service{aiClient: aiClient, riskRouter: riskRouter, store: store}
}

func (s *Service) Send(ctx context.Context, request SendMessageRequest) (SendMessageResponse, error) {
	startedAt := time.Now()
	traceID := newID("trace")
	sessionID := firstNonEmpty(request.SessionID, newID("session"))
	messageID := newID("message")
	channel := firstNonEmpty(request.Source, "h5")

	if err := s.store.SaveConversation(ctx, ConversationRecord{
		ConversationID: sessionID,
		UserID:         request.UserID,
		Channel:        channel,
		Status:         "active",
	}); err != nil {
		return SendMessageResponse{}, err
	}
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      messageID,
		ConversationID: sessionID,
		SenderType:     "user",
		MessageType:    "text",
		Content:        request.Message,
	}); err != nil {
		return SendMessageResponse{}, err
	}

	if result, matched := s.riskRouter.Match(request.Message); matched {
		replyType := "answer"
		if result.TransferToHuman {
			replyType = "handoff"
		}
		response := SendMessageResponse{
			SessionID:       sessionID,
			MessageID:       messageID,
			Reply:           result.Reply,
			ReplyType:       replyType,
			TransferToHuman: result.TransferToHuman,
			RiskLevel:       result.RiskLevel,
			Intent:          result.Intent,
			Suggestions:     result.Suggestions,
		}
		if err := s.store.SaveMessage(ctx, MessageRecord{
			MessageID:      newID("message"),
			ConversationID: sessionID,
			SenderType:     "assistant",
			MessageType:    "text",
			Content:        response.Reply,
		}); err != nil {
			return SendMessageResponse{}, err
		}
		if err := s.store.SaveAIEvent(ctx, AIEventRecord{
			TraceID:         traceID,
			ConversationID:  sessionID,
			MessageID:       messageID,
			Intent:          response.Intent,
			Route:           result.Route,
			ResponseType:    response.ReplyType,
			HandoffRequired: response.TransferToHuman,
			HandoffReason:   handoffReason(response),
			LatencyMS:       elapsedMilliseconds(startedAt),
		}); err != nil {
			return SendMessageResponse{}, err
		}
		return response, nil
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

	response := SendMessageResponse{
		SessionID:       sessionID,
		MessageID:       messageID,
		Reply:           aiResponse.Reply,
		ReplyType:       "answer",
		TransferToHuman: aiResponse.TransferToHuman,
		RiskLevel:       aiResponse.RiskLevel,
		Intent:          aiResponse.Intent,
		Suggestions:     aiResponse.Suggestions,
	}
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      newID("message"),
		ConversationID: sessionID,
		SenderType:     "assistant",
		MessageType:    "text",
		Content:        response.Reply,
	}); err != nil {
		return SendMessageResponse{}, err
	}
	if err := s.store.SaveAIEvent(ctx, AIEventRecord{
		TraceID:         traceID,
		ConversationID:  sessionID,
		MessageID:       messageID,
		Intent:          response.Intent,
		Route:           "ai_reply",
		ResponseType:    response.ReplyType,
		HandoffRequired: response.TransferToHuman,
		HandoffReason:   "",
		LatencyMS:       elapsedMilliseconds(startedAt),
		ModelUsed:       "mock",
	}); err != nil {
		return SendMessageResponse{}, err
	}
	return response, nil
}

func (s *Service) SaveFeedback(ctx context.Context, request FeedbackRequest) (FeedbackResponse, error) {
	if err := s.store.SaveFeedback(ctx, FeedbackRecord{
		ConversationID: request.ConversationID,
		MessageID:      request.MessageID,
		UserID:         request.UserID,
		Rating:         request.Rating,
		Comment:        request.Comment,
		ActionTaken:    request.ActionTaken,
	}); err != nil {
		return FeedbackResponse{}, err
	}
	return FeedbackResponse{Success: true}, nil
}

func firstNonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func elapsedMilliseconds(startedAt time.Time) int {
	elapsed := time.Since(startedAt).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return int(elapsed)
}

func handoffReason(response SendMessageResponse) string {
	if !response.TransferToHuman {
		return ""
	}
	return response.Intent
}
