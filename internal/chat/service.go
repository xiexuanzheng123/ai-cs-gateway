package chat

import (
	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
	"context"
	"time"
)

type SendMessageRequest struct {
	ConversationID string            `json:"conversation_id" binding:"required"`
	UserID         string            `json:"user_id" binding:"required"`
	MessageID      string            `json:"message_id" binding:"required"`
	MessageType    string            `json:"message_type" binding:"required"`
	Message        string            `json:"message" binding:"required"`
	Channel        string            `json:"channel" binding:"required"`
	Metadata       map[string]string `json:"metadata"`
}

type SendMessageResponse struct {
	TraceID        string          `json:"trace_id"`
	ConversationID string          `json:"conversation_id"`
	ResponseType   string          `json:"response_type"`
	Content        ResponseContent `json:"content"`
	Handoff        HandoffDecision `json:"handoff"`
	Intent         string          `json:"intent"`
	Route          string          `json:"route"`
	RiskLevel      string          `json:"risk_level"`
	MessageID      string          `json:"message_id"`
}

type ResponseContent struct {
	Text    string           `json:"text"`
	Buttons []ResponseButton `json:"buttons"`
}

type ResponseButton struct {
	Text   string `json:"text"`
	Action string `json:"action"`
}

type HandoffDecision struct {
	Required bool   `json:"required"`
	Reason   string `json:"reason"`
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

type HandoffRequest struct {
	ConversationID string `json:"conversation_id" binding:"required"`
	MessageID      string `json:"message_id"`
	UserID         string `json:"user_id" binding:"required"`
	Reason         string `json:"reason"`
	Channel        string `json:"channel" binding:"required"`
}

type HandoffResponse struct {
	Success   bool   `json:"success"`
	HandoffID string `json:"handoff_id"`
	Status    string `json:"status"`
}

type RuleConfigRequest struct {
	RuleType    string `json:"rule_type" binding:"required"`
	Pattern     string `json:"pattern" binding:"required"`
	Action      string `json:"action" binding:"required"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type Service struct {
	aiClient      AIClient
	riskRouter    *routing.RiskRouter
	dynamicRouter *routing.DynamicRuleRouter
	store         Store
}

type AIClient interface {
	Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error)
}

func NewService(aiClient AIClient, riskRouter *routing.RiskRouter, store Store) *Service {
	if store == nil {
		store = NoopStore{}
	}
	return &Service{
		aiClient:      aiClient,
		riskRouter:    riskRouter,
		dynamicRouter: routing.NewDynamicRuleRouter(),
		store:         store,
	}
}

func (s *Service) Send(ctx context.Context, request SendMessageRequest) (SendMessageResponse, error) {
	startedAt := time.Now()
	traceID := newID("trace")
	conversationID := request.ConversationID

	if err := s.store.SaveConversation(ctx, ConversationRecord{
		ConversationID: conversationID,
		UserID:         request.UserID,
		Channel:        request.Channel,
		Status:         "active",
	}); err != nil {
		return SendMessageResponse{}, err
	}
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      request.MessageID,
		ConversationID: conversationID,
		SenderType:     "user",
		MessageType:    request.MessageType,
		Content:        request.Message,
	}); err != nil {
		return SendMessageResponse{}, err
	}

	if result, matched := s.matchMediaGuide(request.MessageType); matched {
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, result)
	}

	if result, matched := s.matchRule(request.Message); matched {
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, result)
	}

	smartReplyEnabled, err := s.store.GetFeatureFlag(ctx, "smart_reply_enabled")
	if err != nil {
		return SendMessageResponse{}, err
	}
	if !smartReplyEnabled {
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, routing.RiskResult{
			Intent:      "feature_disabled",
			Route:       "fixed_faq_fallback",
			RiskLevel:   "low",
			Reply:       "您好，当前智能回复已关闭。您可以选择常见问题或转人工继续处理。",
			Suggestions: []string{"密码错误过多", "找回账号密码", "转人工"},
		})
	}

	aiResponse, err := s.aiClient.Reply(ctx, ai.ReplyRequest{
		SessionID:       conversationID,
		UserID:          request.UserID,
		Message:         request.Message,
		History:         []ai.HistoryItem{},
		BusinessContext: map[string]any{},
	})
	if err != nil {
		return SendMessageResponse{}, err
	}

	response := SendMessageResponse{
		TraceID:        traceID,
		ConversationID: conversationID,
		ResponseType:   responseType(aiResponse.TransferToHuman, "answer"),
		Content:        responseContent(aiResponse.Reply, aiResponse.Suggestions),
		Handoff:        handoffDecision(aiResponse.TransferToHuman, aiResponse.Intent),
		Intent:         aiResponse.Intent,
		Route:          "ai_reply",
		RiskLevel:      aiResponse.RiskLevel,
		MessageID:      request.MessageID,
	}
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      newID("message"),
		ConversationID: conversationID,
		SenderType:     "assistant",
		MessageType:    "text",
		Content:        response.Content.Text,
	}); err != nil {
		return SendMessageResponse{}, err
	}
	if err := s.store.SaveAIEvent(ctx, AIEventRecord{
		TraceID:         traceID,
		ConversationID:  conversationID,
		MessageID:       request.MessageID,
		Intent:          response.Intent,
		Route:           "ai_reply",
		ResponseType:    response.ResponseType,
		HandoffRequired: response.Handoff.Required,
		HandoffReason:   response.Handoff.Reason,
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

func (s *Service) CreateHandoff(ctx context.Context, request HandoffRequest) (HandoffResponse, error) {
	handoffID := newID("handoff")
	status := "transferred"
	if err := s.store.SaveHandoff(ctx, HandoffRecord{
		HandoffID:      handoffID,
		ConversationID: request.ConversationID,
		MessageID:      request.MessageID,
		UserID:         request.UserID,
		Reason:         firstNonEmpty(request.Reason, "user_requested"),
		Source:         request.Channel,
		Status:         status,
	}); err != nil {
		return HandoffResponse{}, err
	}
	return HandoffResponse{
		Success:   true,
		HandoffID: handoffID,
		Status:    status,
	}, nil
}

func (s *Service) ListRules(ctx context.Context) ([]RuleConfigRecord, error) {
	return s.store.ListRules(ctx)
}

func (s *Service) CreateRule(ctx context.Context, request RuleConfigRequest) (RuleConfigRecord, error) {
	return s.store.CreateRule(ctx, ruleRecordFromRequest(0, request))
}

func (s *Service) UpdateRule(ctx context.Context, id int64, request RuleConfigRequest) (RuleConfigRecord, error) {
	return s.store.UpdateRule(ctx, ruleRecordFromRequest(id, request))
}

func (s *Service) ReloadRules(ctx context.Context) error {
	rules, err := s.store.ListEnabledRules(ctx)
	if err != nil {
		return err
	}

	dynamicRules := make([]routing.DynamicRule, 0, len(rules))
	for _, rule := range rules {
		dynamicRules = append(dynamicRules, routing.DynamicRule{
			ID:          rule.ID,
			RuleType:    rule.RuleType,
			Pattern:     rule.Pattern,
			Action:      rule.Action,
			Priority:    rule.Priority,
			Enabled:     rule.Enabled,
			Description: rule.Description,
		})
	}
	s.dynamicRouter.ReplaceRules(dynamicRules)
	return nil
}

func (s *Service) GetDashboard(ctx context.Context) (DashboardStats, error) {
	return s.store.GetDashboardStats(ctx)
}

func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error) {
	return s.store.ListFeatureFlags(ctx)
}

func (s *Service) SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error) {
	return s.store.SetFeatureFlag(ctx, key, enabled)
}

func (s *Service) matchRule(message string) (routing.RiskResult, bool) {
	if result, matched := s.dynamicRouter.Match(message); matched {
		return result, true
	}
	return s.riskRouter.Match(message)
}

func (s *Service) matchMediaGuide(messageType string) (routing.RiskResult, bool) {
	switch messageType {
	case "image":
		return routing.RiskResult{
			Intent:      "media_image_guide",
			Route:       "media_guide",
			RiskLevel:   "low",
			Reply:       "我已经收到图片。请再补充一下问题描述，例如订单号、账号信息或遇到的具体现象，方便继续处理。",
			Suggestions: []string{"补充问题描述", "转人工"},
		}, true
	case "audio":
		return routing.RiskResult{
			Intent:      "media_audio_guide",
			Route:       "media_guide",
			RiskLevel:   "low",
			Reply:       "我已经收到语音。为了避免识别偏差，请用文字补充一下核心问题，或直接选择转人工。",
			Suggestions: []string{"文字描述问题", "转人工"},
		}, true
	default:
		return routing.RiskResult{}, false
	}
}

func (s *Service) saveRoutedResponse(ctx context.Context, startedAt time.Time, traceID string, request SendMessageRequest, result routing.RiskResult) (SendMessageResponse, error) {
	response := SendMessageResponse{
		TraceID:        traceID,
		ConversationID: request.ConversationID,
		ResponseType:   responseType(result.TransferToHuman, routeResponseType(result.Route)),
		Content:        responseContent(result.Reply, result.Suggestions),
		Handoff:        handoffDecision(result.TransferToHuman, result.Intent),
		Intent:         result.Intent,
		Route:          result.Route,
		RiskLevel:      result.RiskLevel,
		MessageID:      request.MessageID,
	}
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      newID("message"),
		ConversationID: request.ConversationID,
		SenderType:     "assistant",
		MessageType:    "text",
		Content:        response.Content.Text,
	}); err != nil {
		return SendMessageResponse{}, err
	}
	if err := s.store.SaveAIEvent(ctx, AIEventRecord{
		TraceID:         traceID,
		ConversationID:  request.ConversationID,
		MessageID:       request.MessageID,
		Intent:          response.Intent,
		Route:           result.Route,
		ResponseType:    response.ResponseType,
		HandoffRequired: response.Handoff.Required,
		HandoffReason:   response.Handoff.Reason,
		LatencyMS:       elapsedMilliseconds(startedAt),
	}); err != nil {
		return SendMessageResponse{}, err
	}
	return response, nil
}

func responseContent(text string, suggestions []string) ResponseContent {
	buttons := make([]ResponseButton, 0, len(suggestions))
	for _, suggestion := range suggestions {
		action := "send_message"
		if suggestion == "转人工" || suggestion == "等待人工客服" {
			action = "handoff"
		}
		buttons = append(buttons, ResponseButton{Text: suggestion, Action: action})
	}
	return ResponseContent{Text: text, Buttons: buttons}
}

func responseType(handoff bool, fallback string) string {
	if handoff {
		return "handoff"
	}
	return fallback
}

func routeResponseType(route string) string {
	if route == "media_guide" {
		return "guide"
	}
	return "answer"
}

func handoffDecision(required bool, reason string) HandoffDecision {
	if !required {
		return HandoffDecision{Required: false}
	}
	return HandoffDecision{Required: true, Reason: reason}
}

func firstNonEmpty(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func ruleRecordFromRequest(id int64, request RuleConfigRequest) RuleConfigRecord {
	return RuleConfigRecord{
		ID:          id,
		RuleType:    request.RuleType,
		Pattern:     request.Pattern,
		Action:      request.Action,
		Priority:    request.Priority,
		Enabled:     request.Enabled,
		Description: request.Description,
	}
}

func elapsedMilliseconds(startedAt time.Time) int {
	elapsed := time.Since(startedAt).Milliseconds()
	if elapsed < 0 {
		return 0
	}
	return int(elapsed)
}

func handoffReason(response SendMessageResponse) string {
	if !response.Handoff.Required {
		return ""
	}
	return response.Handoff.Reason
}
