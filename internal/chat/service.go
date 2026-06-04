package chat

import (
	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
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

type KnowledgeRequest struct {
	KnowledgeID string `json:"knowledge_id" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Content     string `json:"content" binding:"required"`
	Category    string `json:"category" binding:"required"`
	Owner       string `json:"owner"`
	Version     string `json:"version" binding:"required"`
	Status      string `json:"status" binding:"required"`
}

type RAGEvalCaseRequest struct {
	CaseID              string `json:"case_id" binding:"required"`
	QueryText           string `json:"query_text" binding:"required"`
	ExpectedKnowledgeID string `json:"expected_knowledge_id"`
	ExpectedIntent      string `json:"expected_intent"`
	ShouldAnswer        bool   `json:"should_answer"`
	Status              string `json:"status" binding:"required"`
}

type RAGSearchRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k"`
}

type Service struct {
	aiClient      AIClient
	riskRouter    *routing.RiskRouter
	dynamicRouter *routing.DynamicRuleRouter
	store         Store
	traceLogger   *TraceLogger
}

type AIClient interface {
	Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error)
	UpsertVectors(ctx context.Context, request ai.VectorUpsertRequest) (ai.VectorUpsertResponse, error)
	SearchVectors(ctx context.Context, request ai.VectorSearchRequest) (ai.VectorSearchResponse, error)
}

type traceRecorder struct {
	startedAt time.Time
	record    TraceLogRecord
}

func (r *traceRecorder) stage(name string, status string, startedAt time.Time, detail string) {
	r.record.Stages = append(r.record.Stages, TraceStageRecord{
		Name:      name,
		Status:    status,
		LatencyMS: elapsedMilliseconds(startedAt),
		Detail:    detail,
	})
}

func (r *traceRecorder) finish(response SendMessageResponse, model string, totalLatencyMS int) {
	r.record.ResponseText = response.Content.Text
	r.record.Intent = response.Intent
	r.record.Route = response.Route
	r.record.ResponseType = response.ResponseType
	r.record.RiskLevel = response.RiskLevel
	r.record.HandoffRequired = response.Handoff.Required
	r.record.HandoffReason = response.Handoff.Reason
	r.record.ModelUsed = model
	r.record.TotalLatencyMS = totalLatencyMS
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
		traceLogger:   NewTraceLogger(store),
	}
}

func (s *Service) Send(ctx context.Context, request SendMessageRequest) (SendMessageResponse, error) {
	startedAt := time.Now()
	traceID := newID("trace")
	conversationID := request.ConversationID

	// trace 只在内存中累计阶段信息，最终交给异步 logger 写库，避免拖慢用户回复。
	trace := &traceRecorder{
		startedAt: startedAt,
		record: TraceLogRecord{
			TraceID:        traceID,
			ConversationID: conversationID,
			MessageID:      request.MessageID,
			UserID:         request.UserID,
			Channel:        request.Channel,
			MessageType:    request.MessageType,
			UserMessage:    request.Message,
		},
	}

	stageStarted := time.Now()
	if err := s.store.SaveConversation(ctx, ConversationRecord{
		ConversationID: conversationID,
		UserID:         request.UserID,
		Channel:        request.Channel,
		Status:         "active",
	}); err != nil {
		trace.stage("save_conversation", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.stage("save_conversation", "ok", stageStarted, "会话写入")

	stageStarted = time.Now()
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      request.MessageID,
		ConversationID: conversationID,
		SenderType:     "user",
		MessageType:    request.MessageType,
		Content:        request.Message,
	}); err != nil {
		trace.stage("save_user_message", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.stage("save_user_message", "ok", stageStarted, "用户消息写入")

	// 图片/语音当前不进入 RAG/LLM，先引导用户补充文字信息。
	stageStarted = time.Now()
	if result, matched := s.matchMediaGuide(request.MessageType); matched {
		trace.stage("media_guide", "hit", stageStarted, result.Intent)
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, result, trace)
	}
	trace.stage("media_guide", "miss", stageStarted, request.MessageType)

	// 强规则优先于 AI：命中后直接回复或转人工，不再继续走 RAG/LLM。
	stageStarted = time.Now()
	if result, matched := s.matchRule(request.Message); matched {
		trace.stage("rule_match", "hit", stageStarted, result.Route+"/"+result.Intent)
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, result, trace)
	}
	trace.stage("rule_match", "miss", stageStarted, "无规则命中")

	// 智能回复
	stageStarted = time.Now()
	smartReplyEnabled, err := s.store.GetFeatureFlag(ctx, "smart_reply_enabled")
	if err != nil {
		trace.stage("feature_flag", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	if !smartReplyEnabled {
		trace.stage("feature_flag", "off", stageStarted, "smart_reply_enabled=false")
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, routing.RiskResult{
			Intent:      "feature_disabled",
			Route:       "fixed_faq_fallback",
			RiskLevel:   "low",
			Reply:       "您好，当前智能回复已关闭。您可以选择常见问题或转人工继续处理。",
			Suggestions: []string{"密码错误过多", "找回账号密码", "转人工"},
		}, trace)
	}
	trace.stage("feature_flag", "on", stageStarted, "smart_reply_enabled=true")

	// 规则未命中后先走 RAG；高置信知识直接回答，低置信再交给 LLM。
	stageStarted = time.Now()
	ragResult, ragMatches, ragMatched, err := s.searchRAG(ctx, request.Message, 3)
	if err != nil {
		trace.stage("rag_search", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.record.RAGMatches = ragMatches
	if ragMatched {
		trace.stage("rag_search", "direct_answer", stageStarted, fmt.Sprintf("召回 %d 条，命中 %s %.3f", len(ragMatches), ragResult.KnowledgeID, ragResult.Score))
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, routing.RiskResult{
			Intent:      "rag_knowledge",
			Route:       "rag",
			RiskLevel:   "low",
			Reply:       ragReply(ragResult),
			Suggestions: []string{"有用", "没用", "转人工"},
		}, trace)
	}
	trace.stage("rag_search", "rewrite_to_llm", stageStarted, fmt.Sprintf("召回 %d 条，无高置信知识，进入 LLM", len(ragMatches)))

	// LLM 是最后一层智能回复兜底，Python 服务内部会做意图识别和模型调用。
	stageStarted = time.Now()
	aiResponse, err := s.aiClient.Reply(ctx, ai.ReplyRequest{
		SessionID:       conversationID,
		UserID:          request.UserID,
		Message:         request.Message,
		History:         []ai.HistoryItem{},
		BusinessContext: map[string]any{},
	})
	if err != nil {
		trace.stage("llm_reply", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.stage("llm_reply", "ok", stageStarted, aiResponse.Intent)

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
	// 主业务消息仍同步落库，保证会话记录完整；观测日志单独异步写。
	if err := s.store.SaveMessage(ctx, MessageRecord{
		MessageID:      newID("message"),
		ConversationID: conversationID,
		SenderType:     "assistant",
		MessageType:    "text",
		Content:        response.Content.Text,
	}); err != nil {
		s.saveTraceError(ctx, trace, err)
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
		ModelUsed:       "ai-service",
	}); err != nil {
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.finish(response, "ai-service", elapsedMilliseconds(startedAt))
	s.emitTrace(trace)
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

	// 规则从 MySQL 加载到内存，用户消息进来时只做内存匹配，不每次查库。
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

func (s *Service) ListTraceLogs(ctx context.Context, limit int) ([]TraceLogRecord, error) {
	return s.store.ListTraceLogs(ctx, limit)
}

func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error) {
	return s.store.ListFeatureFlags(ctx)
}

func (s *Service) SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error) {
	return s.store.SetFeatureFlag(ctx, key, enabled)
}

func (s *Service) ListKnowledge(ctx context.Context) ([]KnowledgeRecord, error) {
	return s.store.ListKnowledge(ctx)
}

func (s *Service) CreateKnowledge(ctx context.Context, request KnowledgeRequest) (KnowledgeRecord, error) {
	record, err := s.store.CreateKnowledge(ctx, knowledgeRecordFromRequest(0, request))
	if err != nil {
		return KnowledgeRecord{}, err
	}
	if err := s.rebuildKnowledgeChunks(ctx, record); err != nil {
		return KnowledgeRecord{}, err
	}
	return record, nil
}

func (s *Service) UpdateKnowledge(ctx context.Context, id int64, request KnowledgeRequest) (KnowledgeRecord, error) {
	record, err := s.store.UpdateKnowledge(ctx, knowledgeRecordFromRequest(id, request))
	if err != nil {
		return KnowledgeRecord{}, err
	}
	if err := s.rebuildKnowledgeChunks(ctx, record); err != nil {
		return KnowledgeRecord{}, err
	}
	return record, nil
}

func (s *Service) SyncKnowledgeChunks(ctx context.Context) (KnowledgeChunkSyncResult, error) {
	knowledge, err := s.store.ListKnowledge(ctx)
	if err != nil {
		return KnowledgeChunkSyncResult{}, err
	}

	// 全量同步只处理 published 知识；草稿不进入 RAG 检索。
	chunks := []KnowledgeChunkRecord{}
	publishedCount := 0
	for _, record := range knowledge {
		if record.Status != "published" {
			continue
		}
		publishedCount++
		chunks = append(chunks, buildKnowledgeChunks(record)...)
	}

	if err := s.store.ReplaceKnowledgeChunks(ctx, chunks); err != nil {
		return KnowledgeChunkSyncResult{}, err
	}

	vectorTotal, model, err := s.upsertKnowledgeChunks(ctx, chunks)
	if err != nil {
		return KnowledgeChunkSyncResult{}, err
	}
	return KnowledgeChunkSyncResult{
		KnowledgeTotal: publishedCount,
		ChunkTotal:     len(chunks),
		VectorTotal:    vectorTotal,
		Model:          model,
	}, nil
}

func (s *Service) rebuildKnowledgeChunks(ctx context.Context, record KnowledgeRecord) error {
	chunks := []KnowledgeChunkRecord{}
	if record.Status == "published" {
		chunks = buildKnowledgeChunks(record)
	}
	// 单条知识变更时，先替换 MySQL chunk，再同步向量，保证两边 chunk_id 对齐。
	if err := s.store.ReplaceKnowledgeChunksByKnowledgeID(ctx, record.KnowledgeID, chunks); err != nil {
		return err
	}
	_, _, err := s.upsertKnowledgeChunks(ctx, chunks)
	return err
}

func (s *Service) upsertKnowledgeChunks(ctx context.Context, chunks []KnowledgeChunkRecord) (int, string, error) {
	if len(chunks) == 0 {
		return 0, "", nil
	}

	// Gateway 不直接生成向量，只把 chunk 文本交给 Python AI Service。
	vectorChunks := make([]ai.VectorChunk, 0, len(chunks))
	for _, chunk := range chunks {
		vectorChunks = append(vectorChunks, ai.VectorChunk{
			ChunkID:     chunk.ChunkID,
			KnowledgeID: chunk.KnowledgeID,
			ChunkText:   chunk.ChunkText,
		})
	}

	response, err := s.aiClient.UpsertVectors(ctx, ai.VectorUpsertRequest{Chunks: vectorChunks})
	if err != nil {
		return 0, "", err
	}

	vectorIDs := map[string]string{}
	for _, item := range response.Items {
		vectorIDs[item.ChunkID] = item.VectorID
	}
	if err := s.store.UpdateKnowledgeChunkVectorIDs(ctx, vectorIDs); err != nil {
		return 0, "", err
	}
	return len(response.Items), response.Model, nil
}

func (s *Service) SearchRAG(ctx context.Context, query string, topK int) (RAGSearchResult, bool, error) {
	result, _, matched, err := s.searchRAG(ctx, query, topK)
	return result, matched, err
}

func (s *Service) searchRAG(ctx context.Context, query string, topK int) (RAGSearchResult, []RAGSearchResult, bool, error) {
	// Python 负责 embedding + Milvus 召回；Gateway 根据 chunk_id 回查 MySQL 取完整知识内容。
	response, err := s.aiClient.SearchVectors(ctx, ai.VectorSearchRequest{Query: query, TopK: topK})
	if err != nil {
		return RAGSearchResult{}, nil, false, err
	}
	if len(response.Items) == 0 {
		return RAGSearchResult{}, []RAGSearchResult{}, false, nil
	}

	chunkIDs := make([]string, 0, len(response.Items))
	scores := map[string]float64{}
	chunkText := map[string]string{}
	for _, item := range response.Items {
		chunkIDs = append(chunkIDs, item.ChunkID)
		scores[item.ChunkID] = item.Score
		chunkText[item.ChunkID] = item.ChunkText
	}
	records, err := s.store.GetKnowledgeByChunkIDs(ctx, chunkIDs)
	if err != nil {
		return RAGSearchResult{}, nil, false, err
	}

	matches := make([]RAGSearchResult, 0, len(response.Items))
	for _, item := range response.Items {
		result, ok := records[item.ChunkID]
		if !ok {
			result = RAGSearchResult{
				ChunkID:     item.ChunkID,
				KnowledgeID: item.KnowledgeID,
			}
		}
		result.Score = scores[item.ChunkID]
		result.ChunkText = firstNonEmpty(chunkText[item.ChunkID], result.ChunkText)
		matches = append(matches, result)
	}
	// 低于阈值时不直接拿知识库回答，避免相似但不准确的内容误导用户。
	if matches[0].Score < 0.78 {
		return RAGSearchResult{}, matches, false, nil
	}
	return matches[0], matches, true, nil
}

func (s *Service) ListRAGEvalCases(ctx context.Context) ([]RAGEvalCaseRecord, error) {
	return s.store.ListRAGEvalCases(ctx)
}

func (s *Service) CreateRAGEvalCase(ctx context.Context, request RAGEvalCaseRequest) (RAGEvalCaseRecord, error) {
	return s.store.CreateRAGEvalCase(ctx, ragEvalCaseRecordFromRequest(0, request))
}

func (s *Service) UpdateRAGEvalCase(ctx context.Context, id int64, request RAGEvalCaseRequest) (RAGEvalCaseRecord, error) {
	return s.store.UpdateRAGEvalCase(ctx, ragEvalCaseRecordFromRequest(id, request))
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

func (s *Service) saveRoutedResponse(ctx context.Context, startedAt time.Time, traceID string, request SendMessageRequest, result routing.RiskResult, trace *traceRecorder) (SendMessageResponse, error) {
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
		s.saveTraceError(ctx, trace, err)
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
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	if trace != nil {
		trace.finish(response, "", elapsedMilliseconds(startedAt))
		s.emitTrace(trace)
	}
	return response, nil
}

func (s *Service) saveTraceError(ctx context.Context, trace *traceRecorder, err error) {
	if trace == nil || err == nil {
		return
	}
	trace.record.ErrorMessage = err.Error()
	trace.record.TotalLatencyMS = elapsedMilliseconds(trace.startedAt)
	s.emitTrace(trace)
}

func (s *Service) emitTrace(trace *traceRecorder) {
	if trace == nil || s.traceLogger == nil {
		return
	}
	s.traceLogger.Enqueue(trace.record)
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

func ragReply(result RAGSearchResult) string {
	if result.Content != "" {
		return result.Content
	}
	return result.ChunkText
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

func knowledgeRecordFromRequest(id int64, request KnowledgeRequest) KnowledgeRecord {
	return KnowledgeRecord{
		ID:          id,
		KnowledgeID: request.KnowledgeID,
		Title:       request.Title,
		Content:     request.Content,
		Category:    request.Category,
		Owner:       request.Owner,
		Version:     request.Version,
		Status:      request.Status,
	}
}

func buildKnowledgeChunks(record KnowledgeRecord) []KnowledgeChunkRecord {
	const maxChunkRunes = 500
	const overlapRunes = 80

	text := normalizeKnowledgeText(record.Title, record.Content)
	runes := []rune(text)
	if len(runes) == 0 {
		return []KnowledgeChunkRecord{}
	}

	chunks := []KnowledgeChunkRecord{}
	for start := 0; start < len(runes); {
		end := start + maxChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		chunkText := strings.TrimSpace(string(runes[start:end]))
		if chunkText != "" {
			index := len(chunks) + 1
			chunks = append(chunks, KnowledgeChunkRecord{
				ChunkID:     fmt.Sprintf("%s_%s_%03d", record.KnowledgeID, record.Version, index),
				KnowledgeID: record.KnowledgeID,
				Version:     record.Version,
				ChunkText:   chunkText,
				TokenCount:  estimateTokenCount(chunkText),
			})
		}
		if end == len(runes) {
			break
		}
		start = end - overlapRunes
		if start < 0 {
			start = end
		}
	}
	return chunks
}

func normalizeKnowledgeText(title string, content string) string {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" {
		return "内容：" + content
	}
	if content == "" {
		return "标题：" + title
	}
	return "标题：" + title + "\n内容：" + content
}

func estimateTokenCount(text string) int {
	count := utf8.RuneCountInString(text)
	if count == 0 {
		return 0
	}
	return (count + 1) / 2
}

func ragEvalCaseRecordFromRequest(id int64, request RAGEvalCaseRequest) RAGEvalCaseRecord {
	return RAGEvalCaseRecord{
		ID:                  id,
		CaseID:              request.CaseID,
		QueryText:           request.QueryText,
		ExpectedKnowledgeID: request.ExpectedKnowledgeID,
		ExpectedIntent:      request.ExpectedIntent,
		ShouldAnswer:        request.ShouldAnswer,
		Status:              request.Status,
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
