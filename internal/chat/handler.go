package chat

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Send(c *gin.Context) {
	var request SendMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_chat_request", err)
		return
	}

	// Handler 只做 HTTP 入参/出参转换，真正的客服编排逻辑都在 Service.Send。
	response, err := h.service.Send(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "chat_failed", err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) Feedback(c *gin.Context) {
	var request FeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_feedback_request", err)
		return
	}

	response, err := h.service.SaveFeedback(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "feedback_failed", err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) Handoff(c *gin.Context) {
	var request HandoffRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_handoff_request", err)
		return
	}

	response, err := h.service.CreateHandoff(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "handoff_failed", err)
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *Handler) ListRules(c *gin.Context) {
	rules, err := h.service.ListRules(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "list_rules_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *Handler) CreateRule(c *gin.Context) {
	var request RuleConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_rule_request", err)
		return
	}

	rule, err := h.service.CreateRule(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "create_rule_failed", err)
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (h *Handler) UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(c, "invalid_rule_id", nil)
		return
	}

	var request RuleConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_rule_request", err)
		return
	}

	rule, err := h.service.UpdateRule(c.Request.Context(), id, request)
	if err != nil {
		writeUpstreamError(c, "update_rule_failed", err)
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (h *Handler) ReloadRules(c *gin.Context) {
	// 规则变更后显式 reload，把 MySQL 配置刷新到内存路由器。
	if err := h.service.ReloadRules(c.Request.Context()); err != nil {
		writeUpstreamError(c, "reload_rules_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) Dashboard(c *gin.Context) {
	stats, err := h.service.GetDashboard(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "dashboard_failed", err)
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *Handler) ListTraceLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	records, err := h.service.ListTraceLogs(c.Request.Context(), limit)
	if err != nil {
		writeUpstreamError(c, "list_trace_logs_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": records})
}

func (h *Handler) ListFeatureFlags(c *gin.Context) {
	flags, err := h.service.ListFeatureFlags(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "list_flags_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"flags": flags})
}

func (h *Handler) SetFeatureFlag(c *gin.Context) {
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_flag_request", err)
		return
	}

	flag, err := h.service.SetFeatureFlag(c.Request.Context(), c.Param("key"), request.Enabled)
	if err != nil {
		writeUpstreamError(c, "set_flag_failed", err)
		return
	}

	c.JSON(http.StatusOK, flag)
}

func (h *Handler) ListKnowledge(c *gin.Context) {
	records, err := h.service.ListKnowledge(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "list_knowledge_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"knowledge": records})
}

func (h *Handler) CreateKnowledge(c *gin.Context) {
	var request KnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_knowledge_request", err)
		return
	}

	record, err := h.service.CreateKnowledge(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "create_knowledge_failed", err)
		return
	}

	c.JSON(http.StatusOK, record)
}

func (h *Handler) UpdateKnowledge(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(c, "invalid_knowledge_id", nil)
		return
	}

	var request KnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_knowledge_request", err)
		return
	}

	record, err := h.service.UpdateKnowledge(c.Request.Context(), id, request)
	if err != nil {
		writeUpstreamError(c, "update_knowledge_failed", err)
		return
	}

	c.JSON(http.StatusOK, record)
}

func (h *Handler) SyncKnowledgeChunks(c *gin.Context) {
	// 手动全量同步：重建 MySQL chunk，并触发 Python 服务写入 Milvus。
	result, err := h.service.SyncKnowledgeChunks(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "sync_knowledge_chunks_failed", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) SearchRAG(c *gin.Context) {
	var request RAGSearchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_rag_search_request", err)
		return
	}
	topK := request.TopK
	if topK <= 0 {
		topK = 3
	}

	// 内部调试接口：只验证 RAG 召回，不会写会话消息或触发 LLM。
	result, matched, err := h.service.SearchRAG(c.Request.Context(), request.Query, topK)
	if err != nil {
		writeUpstreamError(c, "rag_search_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"matched": matched,
		"result":  result,
	})
}

func (h *Handler) ListRAGEvalCases(c *gin.Context) {
	records, err := h.service.ListRAGEvalCases(c.Request.Context())
	if err != nil {
		writeUpstreamError(c, "list_rag_eval_cases_failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"cases": records})
}

func (h *Handler) CreateRAGEvalCase(c *gin.Context) {
	var request RAGEvalCaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_rag_eval_case_request", err)
		return
	}

	record, err := h.service.CreateRAGEvalCase(c.Request.Context(), request)
	if err != nil {
		writeUpstreamError(c, "create_rag_eval_case_failed", err)
		return
	}

	c.JSON(http.StatusOK, record)
}

func (h *Handler) UpdateRAGEvalCase(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(c, "invalid_rag_eval_case_id", nil)
		return
	}

	var request RAGEvalCaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeBadRequest(c, "invalid_rag_eval_case_request", err)
		return
	}

	record, err := h.service.UpdateRAGEvalCase(c.Request.Context(), id, request)
	if err != nil {
		writeUpstreamError(c, "update_rag_eval_case_failed", err)
		return
	}

	c.JSON(http.StatusOK, record)
}
