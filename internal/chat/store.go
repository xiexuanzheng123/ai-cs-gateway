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

type Store interface {
	SaveConversation(ctx context.Context, record ConversationRecord) error
	SaveMessage(ctx context.Context, record MessageRecord) error
}

type NoopStore struct{}

func (NoopStore) SaveConversation(ctx context.Context, record ConversationRecord) error {
	return nil
}

func (NoopStore) SaveMessage(ctx context.Context, record MessageRecord) error {
	return nil
}
