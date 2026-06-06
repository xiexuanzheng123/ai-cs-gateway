package chat

import (
	"ai-cs-gateway/internal/ai"
	"ai-cs-gateway/internal/routing"
	"context"
	"fmt"
	"sort"
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
	TraceID        string           `json:"trace_id"`
	ConversationID string           `json:"conversation_id"`
	ResponseType   string           `json:"response_type"`
	Content        ResponseContent  `json:"content"`
	Handoff        HandoffDecision  `json:"handoff"`
	Intent         string           `json:"intent"`
	Route          string           `json:"route"`
	RiskLevel      string           `json:"risk_level"`
	Citations      []CitationRecord `json:"citations"`
	MessageID      string           `json:"message_id"`
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

type CategoryRequest struct {
	ParentID  int64  `json:"parent_id"`
	Name      string `json:"name" binding:"required"`
	SortOrder int    `json:"sort_order"`
}

type KnowledgeRequest struct {
	KnowledgeID string `json:"knowledge_id" binding:"required"`
	Question    string `json:"question" binding:"required"`
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
	ragCache      RAGCache
	sessionMemory SessionMemory
	traceLogger   *TraceLogger
}

type AIClient interface {
	Reply(ctx context.Context, request ai.ReplyRequest) (ai.ReplyResponse, error)
	UpsertVectors(ctx context.Context, request ai.VectorUpsertRequest) (ai.VectorUpsertResponse, error)
	SearchVectors(ctx context.Context, request ai.VectorSearchRequest) (ai.VectorSearchResponse, error)
	UpsertKeywords(ctx context.Context, request ai.KeywordUpsertRequest) (ai.KeywordUpsertResponse, error)
	SearchKeywords(ctx context.Context, request ai.KeywordSearchRequest) (ai.KeywordSearchResponse, error)
	Rerank(ctx context.Context, request ai.RerankRequest) (ai.RerankResponse, error)
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
	r.record.Citations = response.Citations
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

func (s *Service) SetRAGCache(cache RAGCache) {
	s.ragCache = cache
}

func (s *Service) SetSessionMemory(memory SessionMemory) {
	s.sessionMemory = memory
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

	stageStarted = time.Now()
	memory, memoryStatus := s.loadSessionMemory(ctx, conversationID)
	trace.stage("session_memory", memoryStatus, stageStarted, fmt.Sprintf("history=%d", len(memory.Messages)))

	// 规则未命中后先走 RAG：无可用资料则固定兜底；有资料则把召回片段交给 LLM 组织回答。
	stageStarted = time.Now()
	_, ragMatches, _, cacheStatus, err := s.searchRAG(ctx, request.Message, 3)
	if err != nil {
		trace.stage("rag_search", "error", stageStarted, err.Error())
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.record.RAGMatches = ragMatches
	if !ragHasUsableKnowledge(ragMatches) {
		detail := fmt.Sprintf("召回 %d 条，无可用知识（零召回或分数低于 %.2f）", len(ragMatches), ragMinScore)
		trace.stage("rag_search", "no_knowledge", stageStarted, withCacheStatus(detail, cacheStatus))
		return s.saveRoutedResponse(ctx, startedAt, traceID, request, routing.RiskResult{
			Intent:          "rag_no_knowledge",
			Route:           "rag_fallback",
			RiskLevel:       "low",
			Reply:           ragNoKnowledgeFallbackReply,
			Suggestions:     []string{"继续描述问题", "转人工"},
			TransferToHuman: false,
		}, trace)
	}

	usableMatches := usableRAGMatches(ragMatches)
	trace.stage(
		"rag_search",
		"rag_llm",
		stageStarted,
		withCacheStatus(fmt.Sprintf("召回 %d 条，可用 %d 条，Top %.3f", len(ragMatches), len(usableMatches), usableMatches[0].Score), cacheStatus),
	)
	if shouldDirectReplyRAG(request.Message, usableMatches) {
		trace.record.Citations = citationsFromRAGMatches(usableMatches[:1])
		trace.stage("rag_direct", "hit", time.Now(), fmt.Sprintf("Top %.3f，跳过 LLM", usableMatches[0].Score))
		return s.saveRAGDirectResponse(ctx, startedAt, traceID, request, usableMatches[0], memory, trace)
	}

	stageStarted = time.Now()
	aiResponse, err := s.aiClient.Reply(ctx, ai.ReplyRequest{
		SessionID: conversationID,
		UserID:    request.UserID,
		Message:   request.Message,
		History:   memoryHistory(memory),
		BusinessContext: map[string]any{
			"retrieved_passages": buildRetrievedPassages(usableMatches),
		},
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
		Route:          firstNonEmpty(aiResponse.Route, "rag_llm"),
		RiskLevel:      aiResponse.RiskLevel,
		Citations:      citationsFromRetrievedDocs(aiResponse.RetrievedDocs),
		MessageID:      request.MessageID,
	}

	stageStarted = time.Now()
	validation := validateResponse(responseValidationInput{
		UserMessage: request.Message,
		Response:    response,
		RAGMatches:  usableMatches,
	})
	if validation.Passed {
		trace.stage("response_validator", "pass", stageStarted, validation.Reason)
	} else {
		trace.stage("response_validator", validation.Action, stageStarted, validation.Reason)
		response = applyValidationAction(response, validation)
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
		Route:           response.Route,
		ResponseType:    response.ResponseType,
		HandoffRequired: response.Handoff.Required,
		HandoffReason:   response.Handoff.Reason,
		LatencyMS:       elapsedMilliseconds(startedAt),
		ModelUsed:       firstNonEmpty(aiResponse.Model, "ai-service"),
		InputTokens:     aiResponse.InputTokens,
		OutputTokens:    aiResponse.OutputTokens,
		EstimatedCost:   aiResponse.EstimatedCost,
	}); err != nil {
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.record.InputTokens = aiResponse.InputTokens
	trace.record.OutputTokens = aiResponse.OutputTokens
	trace.record.EstimatedCost = aiResponse.EstimatedCost
	trace.finish(response, firstNonEmpty(aiResponse.Model, "ai-service"), elapsedMilliseconds(startedAt))
	s.saveSessionMemory(ctx, memory, request, response, trace)
	s.emitTrace(trace)
	return response, nil
}

func (s *Service) saveRAGDirectResponse(ctx context.Context, startedAt time.Time, traceID string, request SendMessageRequest, match RAGSearchResult, memory SessionMemoryRecord, trace *traceRecorder) (SendMessageResponse, error) {
	response := SendMessageResponse{
		TraceID:        traceID,
		ConversationID: request.ConversationID,
		ResponseType:   "answer",
		Content:        responseContent(ragDirectReplyText(match), nil),
		Handoff:        HandoffDecision{Required: false},
		Intent:         "rag_direct",
		Route:          "rag_direct",
		RiskLevel:      "low",
		Citations:      citationsFromRAGMatches([]RAGSearchResult{match}),
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
		Route:           response.Route,
		ResponseType:    response.ResponseType,
		HandoffRequired: response.Handoff.Required,
		HandoffReason:   response.Handoff.Reason,
		LatencyMS:       elapsedMilliseconds(startedAt),
		ModelUsed:       "rag_direct",
	}); err != nil {
		s.saveTraceError(ctx, trace, err)
		return SendMessageResponse{}, err
	}
	trace.finish(response, "rag_direct", elapsedMilliseconds(startedAt))
	s.saveSessionMemory(ctx, memory, request, response, trace)
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

func (s *Service) GetQualityStats(ctx context.Context, limit int) (QualityStats, error) {
	return s.store.GetQualityStats(ctx, limit)
}

func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error) {
	return s.store.ListFeatureFlags(ctx)
}

func (s *Service) SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error) {
	return s.store.SetFeatureFlag(ctx, key, enabled)
}

func (s *Service) ListCategories(ctx context.Context) ([]CategoryRecord, error) {
	return s.store.ListCategories(ctx)
}

func (s *Service) CreateCategory(ctx context.Context, request CategoryRequest) (CategoryRecord, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return CategoryRecord{}, fmt.Errorf("category name is required")
	}
	return s.store.CreateCategory(ctx, CategoryRecord{
		ParentID:  request.ParentID,
		Name:      name,
		SortOrder: request.SortOrder,
	})
}

func (s *Service) UpdateCategory(ctx context.Context, id int64, request CategoryRequest) (CategoryRecord, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return CategoryRecord{}, fmt.Errorf("category name is required")
	}
	return s.store.UpdateCategory(ctx, CategoryRecord{
		ID:        id,
		Name:      name,
		SortOrder: request.SortOrder,
	})
}

func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	return s.store.DeleteCategory(ctx, id)
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
	s.clearRAGCache(ctx)
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
	s.clearRAGCache(ctx)
	return record, nil
}

func (s *Service) ListKnowledgeVersions(ctx context.Context, id int64) ([]KnowledgeVersionRecord, error) {
	record, err := s.store.GetKnowledgeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.store.ListKnowledgeVersions(ctx, record.KnowledgeID)
}

func (s *Service) PublishKnowledge(ctx context.Context, id int64) (KnowledgeRecord, error) {
	record, err := s.store.UpdateKnowledgeStatus(ctx, id, "published")
	if err != nil {
		return KnowledgeRecord{}, err
	}
	if err := s.rebuildKnowledgeChunks(ctx, record); err != nil {
		return KnowledgeRecord{}, err
	}
	s.clearRAGCache(ctx)
	return record, nil
}

func (s *Service) RollbackKnowledge(ctx context.Context, id int64, versionID int64) (KnowledgeRecord, error) {
	current, err := s.store.GetKnowledgeByID(ctx, id)
	if err != nil {
		return KnowledgeRecord{}, err
	}
	versions, err := s.store.ListKnowledgeVersions(ctx, current.KnowledgeID)
	if err != nil {
		return KnowledgeRecord{}, err
	}
	for _, version := range versions {
		if version.ID != versionID {
			continue
		}
		record, err := s.store.UpdateKnowledge(ctx, KnowledgeRecord{
			ID:          id,
			KnowledgeID: version.KnowledgeID,
			Question:    version.Question,
			Content:     version.Content,
			Category:    version.Category,
			Owner:       version.Owner,
			Version:     version.Version,
			Status:      version.Status,
		})
		if err != nil {
			return KnowledgeRecord{}, err
		}
		if err := s.rebuildKnowledgeChunks(ctx, record); err != nil {
			return KnowledgeRecord{}, err
		}
		s.clearRAGCache(ctx)
		return record, nil
	}
	return KnowledgeRecord{}, fmt.Errorf("knowledge version not found")
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
	s.clearRAGCache(ctx)
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

	vectorIDs := map[string]string{}
	vectorTotal := 0
	model := ""
	for start := 0; start < len(vectorChunks); start += 10 {
		end := start + 10
		if end > len(vectorChunks) {
			end = len(vectorChunks)
		}
		response, err := s.aiClient.UpsertVectors(ctx, ai.VectorUpsertRequest{Chunks: vectorChunks[start:end]})
		if err != nil {
			return 0, "", err
		}
		if model == "" {
			model = response.Model
		}
		vectorTotal += len(response.Items)
		for _, item := range response.Items {
			vectorIDs[item.ChunkID] = item.VectorID
		}
	}
	if err := s.store.UpdateKnowledgeChunkVectorIDs(ctx, vectorIDs); err != nil {
		return 0, "", err
	}

	// 同一批 chunk 同步写入 OpenSearch，后续 RAG 可走 BM25 + 向量混合召回。
	for start := 0; start < len(vectorChunks); start += 50 {
		end := start + 50
		if end > len(vectorChunks) {
			end = len(vectorChunks)
		}
		if _, err := s.aiClient.UpsertKeywords(ctx, ai.KeywordUpsertRequest{Chunks: vectorChunks[start:end]}); err != nil {
			return 0, "", err
		}
	}
	return vectorTotal, model, nil
}

func (s *Service) SearchRAG(ctx context.Context, query string, topK int) (RAGSearchResult, bool, error) {
	result, _, matched, _, err := s.searchRAG(ctx, query, topK)
	return result, matched, err
}

func (s *Service) searchRAG(ctx context.Context, query string, topK int) (RAGSearchResult, []RAGSearchResult, bool, string, error) {
	cacheStatus := "disabled"
	if s.ragCache != nil {
		cacheStatus = "miss"
		matches, hit, err := s.ragCache.Get(ctx, query, topK)
		if err == nil && hit {
			if !ragHasUsableKnowledge(matches) {
				return RAGSearchResult{}, matches, false, "hit", nil
			}
			return usableRAGMatches(matches)[0], matches, true, "hit", nil
		}
		if err != nil {
			cacheStatus = "error"
		}
	}

	keywordCh := make(chan ragSearchOutcome, 1)
	vectorCh := make(chan ragSearchOutcome, 1)
	go func() {
		matches, err := s.searchKeywordKnowledge(ctx, query, topK)
		keywordCh <- ragSearchOutcome{matches: matches, err: err}
	}()
	go func() {
		matches, err := s.searchVectorKnowledge(ctx, query, topK)
		vectorCh <- ragSearchOutcome{matches: matches, err: err}
	}()

	keywordOutcome := <-keywordCh
	vectorOutcome := <-vectorCh
	if keywordOutcome.err != nil && vectorOutcome.err != nil {
		return RAGSearchResult{}, nil, false, cacheStatus, vectorOutcome.err
	}
	keywordMatches := keywordOutcome.matches
	if keywordOutcome.err != nil {
		keywordMatches = nil
	}
	vectorMatches := vectorOutcome.matches
	if vectorOutcome.err != nil {
		vectorMatches = nil
	}

	matches := mergeRAGMatches(query, vectorMatches, keywordMatches, topK)
	matches = s.rerankRAGMatches(ctx, query, matches, topK)
	matched := ragHasUsableKnowledge(matches)
	s.setRAGCache(ctx, query, topK, matches)
	if !matched {
		return RAGSearchResult{}, matches, false, cacheStatus, nil
	}
	return matches[0], matches, true, cacheStatus, nil
}

func (s *Service) rerankRAGMatches(ctx context.Context, query string, matches []RAGSearchResult, topK int) []RAGSearchResult {
	if len(matches) <= 1 {
		return matches
	}
	if topK <= 0 {
		topK = 3
	}

	limit := len(matches)
	if limit > 5 {
		limit = 5
	}
	documents := make([]ai.RerankDocument, 0, limit)
	byID := make(map[string]RAGSearchResult, limit)
	for _, match := range matches[:limit] {
		id := firstNonEmpty(match.ChunkID, match.KnowledgeID)
		text := strings.TrimSpace(firstNonEmpty(firstNonEmpty(match.ChunkText, match.Content), match.Question))
		if id == "" || text == "" {
			continue
		}
		documents = append(documents, ai.RerankDocument{ID: id, Text: text})
		byID[id] = match
	}
	if len(documents) <= 1 {
		return matches
	}

	response, err := s.aiClient.Rerank(ctx, ai.RerankRequest{
		Query:     query,
		Documents: documents,
		TopN:      topK,
	})
	if err != nil || len(response.Items) == 0 {
		return matches
	}

	reranked := make([]RAGSearchResult, 0, len(matches))
	used := make(map[string]bool, len(response.Items))
	for _, item := range response.Items {
		match, ok := byID[item.ID]
		if !ok {
			continue
		}
		match.Score = normalizeRerankScore(item.Score, match.Score)
		if match.Source == "" {
			match.Source = "rerank"
		} else if !strings.Contains(match.Source, "rerank") {
			match.Source += "_rerank"
		}
		reranked = append(reranked, match)
		used[item.ID] = true
	}
	for _, match := range matches {
		id := firstNonEmpty(match.ChunkID, match.KnowledgeID)
		if used[id] {
			continue
		}
		reranked = append(reranked, match)
	}
	if len(reranked) > topK {
		return reranked[:topK]
	}
	return reranked
}

type ragSearchOutcome struct {
	matches []RAGSearchResult
	err     error
}

func (s *Service) searchVectorKnowledge(ctx context.Context, query string, topK int) ([]RAGSearchResult, error) {
	// Python 负责 embedding + Milvus 召回；Gateway 根据 chunk_id 回查 MySQL 取完整知识内容。
	response, err := s.aiClient.SearchVectors(ctx, ai.VectorSearchRequest{Query: query, TopK: topK})
	if err != nil {
		return nil, err
	}
	if len(response.Items) == 0 {
		return []RAGSearchResult{}, nil
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
		return nil, err
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
		result.Source = "vector"
		matches = append(matches, result)
	}
	return matches, nil
}

func normalizeRerankScore(rerankScore float64, fallback float64) float64 {
	if rerankScore <= 0 {
		return fallback
	}
	if rerankScore <= 1 {
		return 0.74 + rerankScore*0.23
	}
	normalized := 0.74 + rerankScore/(rerankScore+8)*0.23
	if normalized > 0.97 {
		return 0.97
	}
	return normalized
}

func (s *Service) searchKeywordKnowledge(ctx context.Context, query string, topK int) ([]RAGSearchResult, error) {
	response, err := s.aiClient.SearchKeywords(ctx, ai.KeywordSearchRequest{Query: query, TopK: topK})
	if err != nil {
		// OpenSearch 是主关键词召回，但不可用时不能阻断客服主流程。
		return s.store.SearchKnowledgeByKeyword(ctx, query, topK)
	}
	if len(response.Items) == 0 {
		return []RAGSearchResult{}, nil
	}

	chunkIDs := make([]string, 0, len(response.Items))
	scores := map[string]float64{}
	chunkText := map[string]string{}
	for _, item := range response.Items {
		chunkIDs = append(chunkIDs, item.ChunkID)
		scores[item.ChunkID] = normalizeOpenSearchScore(item.Score)
		chunkText[item.ChunkID] = item.ChunkText
	}
	records, err := s.store.GetKnowledgeByChunkIDs(ctx, chunkIDs)
	if err != nil {
		return nil, err
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
		result.Source = "keyword"
		matches = append(matches, result)
	}
	return matches, nil
}

func normalizeOpenSearchScore(score float64) float64 {
	if score <= 0 {
		return 0
	}
	normalized := 0.70 + score/(score+8)*0.25
	if normalized > 0.95 {
		return 0.95
	}
	return normalized
}

func (s *Service) setRAGCache(ctx context.Context, query string, topK int, matches []RAGSearchResult) {
	if s.ragCache == nil {
		return
	}
	_ = s.ragCache.Set(ctx, query, topK, matches)
}

func (s *Service) clearRAGCache(ctx context.Context) {
	if s.ragCache == nil {
		return
	}
	_ = s.ragCache.Clear(ctx)
}

func withCacheStatus(detail string, status string) string {
	if strings.TrimSpace(status) == "" {
		return detail
	}
	return fmt.Sprintf("%s，cache=%s", detail, status)
}

const (
	ragMinScore                 = 0.78
	ragStrongTextMinScore       = 0.74
	ragDirectMinScore           = 0.90
	ragDirectStrongScore        = 0.93
	ragDirectMinGap             = 0.08
	ragDirectMinContentRunes    = 20
	ragNoKnowledgeFallbackReply = "抱歉，暂未在知识库中查到与您问题直接相关的内容。您可以换个方式描述问题，或选择转人工客服为您处理。"
)

func ragPassageText(result RAGSearchResult) string {
	return firstNonEmpty(result.Content, result.ChunkText)
}

func ragHasUsableKnowledge(matches []RAGSearchResult) bool {
	if len(matches) == 0 {
		return false
	}
	if strings.TrimSpace(ragPassageText(matches[0])) == "" {
		return false
	}
	if matches[0].Score >= ragMinScore {
		return true
	}
	return matches[0].Score >= ragStrongTextMinScore && hasStrongTextEvidence(matches[0])
}

func usableRAGMatches(matches []RAGSearchResult) []RAGSearchResult {
	usable := make([]RAGSearchResult, 0, len(matches))
	for _, match := range matches {
		if strings.TrimSpace(ragPassageText(match)) == "" {
			continue
		}
		if match.Score < ragMinScore && !(match.Score >= ragStrongTextMinScore && hasStrongTextEvidence(match)) {
			continue
		}
		usable = append(usable, match)
	}
	return usable
}

func shouldDirectReplyRAG(query string, matches []RAGSearchResult) bool {
	if len(matches) == 0 {
		return false
	}
	top := matches[0]
	content := strings.TrimSpace(ragDirectReplyText(top))
	if top.Score < ragDirectMinScore {
		return false
	}
	if utf8.RuneCountInString(content) < ragDirectMinContentRunes {
		return false
	}
	if top.Score < ragDirectStrongScore && len(matches) > 1 && top.Score-matches[1].Score < ragDirectMinGap {
		return false
	}
	return !isRiskyDirectReplyQuery(query)
}

func isRiskyDirectReplyQuery(query string) bool {
	normalized := normalizeRAGText(query)
	riskyTerms := []string{
		"退款",
		"退钱",
		"未成年",
		"封号",
		"解封",
		"账号安全",
		"帐号安全",
		"换绑",
		"验证码",
		"注销",
		"人工",
		"投诉",
	}
	for _, term := range riskyTerms {
		if strings.Contains(normalized, normalizeRAGText(term)) {
			return true
		}
	}
	return false
}

func mergeRAGMatches(query string, vectorMatches []RAGSearchResult, keywordMatches []RAGSearchResult, limit int) []RAGSearchResult {
	if limit <= 0 {
		limit = 3
	}
	merged := map[string]RAGSearchResult{}
	order := []string{}
	add := func(match RAGSearchResult) {
		key := firstNonEmpty(match.ChunkID, match.KnowledgeID)
		if key == "" {
			return
		}
		existing, exists := merged[key]
		if !exists {
			merged[key] = match
			order = append(order, key)
			return
		}
		if match.Score > existing.Score {
			if existing.Source != "" && match.Source != existing.Source {
				match.Source = "hybrid"
			}
			merged[key] = match
			return
		}
		if existing.Source != "" && match.Source != "" && existing.Source != match.Source {
			existing.Source = "hybrid"
			merged[key] = existing
		}
	}
	for _, match := range vectorMatches {
		add(match)
	}
	for _, match := range keywordMatches {
		add(match)
	}

	results := make([]RAGSearchResult, 0, len(merged))
	for _, key := range order {
		results = append(results, rerankRAGMatch(query, merged[key]))
	}
	sort.SliceStable(results, func(i int, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		return results[:limit]
	}
	return results
}

func rerankRAGMatch(query string, match RAGSearchResult) RAGSearchResult {
	keywords := keywordTerms(query)
	if len(keywords) == 0 {
		return match
	}
	score := match.Score
	question := normalizeRAGText(match.Question)
	content := normalizeRAGText(firstNonEmpty(match.ChunkText, match.Content))
	queryText := normalizeRAGText(query)
	questionHits := 0
	contentHits := 0
	for _, keyword := range keywords {
		keyword = normalizeRAGText(keyword)
		if keyword == "" {
			continue
		}
		if strings.Contains(question, keyword) {
			questionHits++
			score += 0.045
			continue
		}
		if strings.Contains(content, keyword) {
			contentHits++
			score += 0.025
		}
	}
	if question != "" && strings.Contains(queryText, question) {
		score += 0.06
	}
	if questionHits >= 2 {
		score += 0.04
	}
	if questionHits > 0 && contentHits > 0 {
		score += 0.025
	}
	if match.Source == "hybrid" {
		score += 0.035
	}
	if score > 0.99 {
		score = 0.99
	}
	match.Score = score
	return match
}

func hasStrongTextEvidence(match RAGSearchResult) bool {
	return match.Source == "keyword" || match.Source == "hybrid" || strings.TrimSpace(match.Question) != ""
}

func normalizeRAGText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "？", "", "?", "", "，", "", ",", "", "。", "", ".", "", "、", "")
	return replacer.Replace(value)
}

func keywordTerms(query string) []string {
	fields := strings.FieldsFunc(strings.TrimSpace(query), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ',' || r == '，' || r == '、' || r == '?' || r == '？'
	})
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		term := strings.TrimSpace(field)
		if utf8.RuneCountInString(term) < 2 {
			continue
		}
		terms = append(terms, term)
		if len(terms) >= 5 {
			break
		}
	}
	return terms
}

func keywordOrderArgs(keyword string) []any {
	like := "%" + keyword + "%"
	return []any{like, like}
}

func keywordScore(record RAGSearchResult, keywords []string) float64 {
	question := strings.ToLower(record.Question)
	content := strings.ToLower(record.Content)
	chunkText := strings.ToLower(record.ChunkText)
	score := 0.80
	for _, keyword := range keywords {
		keyword = strings.ToLower(keyword)
		switch {
		case strings.Contains(question, keyword):
			score += 0.08
		case strings.Contains(chunkText, keyword):
			score += 0.05
		case strings.Contains(content, keyword):
			score += 0.03
		}
	}
	if score > 0.96 {
		return 0.96
	}
	return score
}

func buildRetrievedPassages(matches []RAGSearchResult) []map[string]any {
	passages := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		passages = append(passages, map[string]any{
			"knowledge_id": match.KnowledgeID,
			"chunk_id":     match.ChunkID,
			"question":     match.Question,
			"score":        match.Score,
			"source":       match.Source,
			"text":         ragPassageText(match),
		})
	}
	return passages
}

func citationsFromRetrievedDocs(docs []ai.RetrievedDocument) []CitationRecord {
	citations := make([]CitationRecord, 0, len(docs))
	for _, doc := range docs {
		if strings.TrimSpace(doc.DocID) == "" {
			continue
		}
		citations = append(citations, CitationRecord{
			DocID:    doc.DocID,
			Question: doc.Question,
			Score:    doc.Score,
		})
	}
	return citations
}

func citationsFromRAGMatches(matches []RAGSearchResult) []CitationRecord {
	citations := make([]CitationRecord, 0, len(matches))
	for _, match := range matches {
		docID := firstNonEmpty(match.KnowledgeID, match.ChunkID)
		if strings.TrimSpace(docID) == "" {
			continue
		}
		citations = append(citations, CitationRecord{
			DocID:    docID,
			Question: match.Question,
			Score:    match.Score,
		})
	}
	return citations
}

func applyValidationAction(response SendMessageResponse, validation responseValidationResult) SendMessageResponse {
	switch validation.Action {
	case "handoff":
		response.ResponseType = "handoff"
		response.Content = responseContent("这个问题需要人工客服核实处理，我已经为您转人工。", []string{"补充问题描述", "上传截图", "等待人工客服"})
		response.Handoff = HandoffDecision{Required: true, Reason: validation.Reason}
		response.Intent = firstNonEmpty(response.Intent, "response_validation_handoff")
		response.Route = "validator_handoff"
		response.RiskLevel = "high"
		response.Citations = nil
	case "fallback":
		response.ResponseType = "answer"
		response.Content = responseContent(validatorFallbackReply, []string{"继续描述问题", "转人工"})
		response.Handoff = HandoffDecision{Required: false}
		response.Intent = firstNonEmpty(response.Intent, "response_validation_fallback")
		response.Route = "validator_fallback"
		response.RiskLevel = "low"
		response.Citations = nil
	}
	return response
}

func (s *Service) ListRAGEvalCases(ctx context.Context) ([]RAGEvalCaseRecord, error) {
	return s.store.ListRAGEvalCases(ctx)
}

func (s *Service) ListRAGEvalRuns(ctx context.Context, limit int) ([]RAGEvalRunRecord, error) {
	return s.store.ListRAGEvalRuns(ctx, limit)
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

func (s *Service) loadSessionMemory(ctx context.Context, conversationID string) (SessionMemoryRecord, string) {
	if s.sessionMemory == nil {
		return SessionMemoryRecord{ConversationID: conversationID}, "disabled"
	}
	record, hit, err := s.sessionMemory.Get(ctx, conversationID)
	if err != nil {
		return SessionMemoryRecord{ConversationID: conversationID}, "error"
	}
	if !hit {
		return SessionMemoryRecord{ConversationID: conversationID}, "miss"
	}
	if record.ConversationID == "" {
		record.ConversationID = conversationID
	}
	return record, "hit"
}

func (s *Service) saveSessionMemory(ctx context.Context, memory SessionMemoryRecord, request SendMessageRequest, response SendMessageResponse, trace *traceRecorder) {
	if s.sessionMemory == nil {
		return
	}
	stageStarted := time.Now()
	record := nextSessionMemory(memory, request, response)
	if err := s.sessionMemory.Set(ctx, record); err != nil {
		if trace != nil {
			trace.stage("session_memory_write", "error", stageStarted, err.Error())
		}
		return
	}
	if trace != nil {
		trace.stage("session_memory_write", "ok", stageStarted, fmt.Sprintf("history=%d", len(record.Messages)))
	}
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

func ragDirectReplyText(result RAGSearchResult) string {
	content := strings.TrimSpace(result.Content)
	if content != "" {
		return content
	}
	text := strings.TrimSpace(result.ChunkText)
	markers := []string{"内容：", "内容:"}
	for _, marker := range markers {
		if index := strings.Index(text, marker); index >= 0 {
			return strings.TrimSpace(text[index+len(marker):])
		}
	}
	return text
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
		Question:    request.Question,
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

	text := normalizeKnowledgeText(record.Question, record.Content)
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

func normalizeKnowledgeText(question string, content string) string {
	question = strings.TrimSpace(question)
	content = strings.TrimSpace(content)
	if question == "" {
		return "内容：" + content
	}
	if content == "" {
		return "问题：" + question
	}
	return "问题：" + question + "\n内容：" + content
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
