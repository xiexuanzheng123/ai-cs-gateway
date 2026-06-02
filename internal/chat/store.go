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

type Store interface {
	SaveConversation(ctx context.Context, record ConversationRecord) error
	SaveMessage(ctx context.Context, record MessageRecord) error
	SaveAIEvent(ctx context.Context, record AIEventRecord) error
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
