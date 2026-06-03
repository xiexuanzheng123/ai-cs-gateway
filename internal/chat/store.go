package chat

import "context"

type ConversationRecord struct {
	ConversationID string
	UserID         string
	Channel        string
	Status         string
}

type MessageRecord struct {
	MessageID      string
	ConversationID string
	SenderType     string
	MessageType    string
	Content        string
}

type AIEventRecord struct {
	TraceID         string
	ConversationID  string
	MessageID       string
	Intent          string
	Route           string
	ResponseType    string
	HandoffRequired bool
	HandoffReason   string
	LatencyMS       int
	ModelUsed       string
}

type FeedbackRecord struct {
	ConversationID string
	MessageID      string
	UserID         string
	Rating         string
	Comment        string
	ActionTaken    string
}

type HandoffRecord struct {
	HandoffID      string
	ConversationID string
	MessageID      string
	UserID         string
	Reason         string
	Source         string
	Status         string
}

type RuleConfigRecord struct {
	ID          int64  `json:"id"`
	RuleType    string `json:"rule_type"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
	Priority    int    `json:"priority"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type FeatureFlagRecord struct {
	Key         string `json:"key"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

type DashboardStats struct {
	TotalConversations int64   `json:"total_conversations"`
	TotalMessages      int64   `json:"total_messages"`
	TotalAIEvents      int64   `json:"total_ai_events"`
	RuleHitCount       int64   `json:"rule_hit_count"`
	HandoffCount       int64   `json:"handoff_count"`
	FeedbackCount      int64   `json:"feedback_count"`
	PositiveFeedback   int64   `json:"positive_feedback"`
	NegativeFeedback   int64   `json:"negative_feedback"`
	AverageLatencyMS   float64 `json:"average_latency_ms"`
}

type KnowledgeRecord struct {
	ID          int64  `json:"id"`
	KnowledgeID string `json:"knowledge_id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Category    string `json:"category"`
	Owner       string `json:"owner"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

type RAGEvalCaseRecord struct {
	ID                  int64  `json:"id"`
	CaseID              string `json:"case_id"`
	QueryText           string `json:"query_text"`
	ExpectedKnowledgeID string `json:"expected_knowledge_id"`
	ExpectedIntent      string `json:"expected_intent"`
	ShouldAnswer        bool   `json:"should_answer"`
	Status              string `json:"status"`
}

type Store interface {
	SaveConversation(ctx context.Context, record ConversationRecord) error
	SaveMessage(ctx context.Context, record MessageRecord) error
	SaveAIEvent(ctx context.Context, record AIEventRecord) error
	SaveFeedback(ctx context.Context, record FeedbackRecord) error
	SaveHandoff(ctx context.Context, record HandoffRecord) error
	ListRules(ctx context.Context) ([]RuleConfigRecord, error)
	ListEnabledRules(ctx context.Context) ([]RuleConfigRecord, error)
	CreateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error)
	UpdateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error)
	GetFeatureFlag(ctx context.Context, key string) (bool, error)
	ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error)
	SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error)
	GetDashboardStats(ctx context.Context) (DashboardStats, error)
	ListKnowledge(ctx context.Context) ([]KnowledgeRecord, error)
	CreateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error)
	UpdateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error)
	ListRAGEvalCases(ctx context.Context) ([]RAGEvalCaseRecord, error)
	CreateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error)
	UpdateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error)
}

type NoopStore struct{}

func (NoopStore) SaveConversation(ctx context.Context, record ConversationRecord) error {
	return nil
}

func (NoopStore) SaveMessage(ctx context.Context, record MessageRecord) error {
	return nil
}

func (NoopStore) SaveAIEvent(ctx context.Context, record AIEventRecord) error {
	return nil
}

func (NoopStore) SaveFeedback(ctx context.Context, record FeedbackRecord) error {
	return nil
}

func (NoopStore) SaveHandoff(ctx context.Context, record HandoffRecord) error {
	return nil
}

func (NoopStore) ListRules(ctx context.Context) ([]RuleConfigRecord, error) {
	return []RuleConfigRecord{}, nil
}

func (NoopStore) ListEnabledRules(ctx context.Context) ([]RuleConfigRecord, error) {
	return []RuleConfigRecord{}, nil
}

func (NoopStore) CreateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	record.ID = 0
	return record, nil
}

func (NoopStore) UpdateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	return record, nil
}

func (NoopStore) GetFeatureFlag(ctx context.Context, key string) (bool, error) {
	return true, nil
}

func (NoopStore) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error) {
	return []FeatureFlagRecord{
		{Key: "smart_reply_enabled", Enabled: true, Description: "智能回复总开关"},
	}, nil
}

func (NoopStore) SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error) {
	return FeatureFlagRecord{Key: key, Enabled: enabled}, nil
}

func (NoopStore) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	return DashboardStats{}, nil
}

func (NoopStore) ListKnowledge(ctx context.Context) ([]KnowledgeRecord, error) {
	return []KnowledgeRecord{}, nil
}

func (NoopStore) CreateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error) {
	return record, nil
}

func (NoopStore) UpdateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error) {
	return record, nil
}

func (NoopStore) ListRAGEvalCases(ctx context.Context) ([]RAGEvalCaseRecord, error) {
	return []RAGEvalCaseRecord{}, nil
}

func (NoopStore) CreateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error) {
	return record, nil
}

func (NoopStore) UpdateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error) {
	return record, nil
}
