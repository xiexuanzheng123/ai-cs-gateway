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
