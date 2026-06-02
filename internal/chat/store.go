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

type Store interface {
	SaveConversation(ctx context.Context, record ConversationRecord) error
	SaveMessage(ctx context.Context, record MessageRecord) error
	SaveAIEvent(ctx context.Context, record AIEventRecord) error
	SaveFeedback(ctx context.Context, record FeedbackRecord) error
	SaveHandoff(ctx context.Context, record HandoffRecord) error
	ListRules(ctx context.Context) ([]RuleConfigRecord, error)
	CreateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error)
	UpdateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error)
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

func (NoopStore) CreateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	record.ID = 0
	return record, nil
}

func (NoopStore) UpdateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	return record, nil
}
