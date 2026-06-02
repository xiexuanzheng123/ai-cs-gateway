package chat

import (
	"context"
	"database/sql"
	"fmt"
)

type MySQLStore struct {
	db *sql.DB
}

func NewMySQLStore(db *sql.DB) *MySQLStore {
	return &MySQLStore{db: db}
}

func (s *MySQLStore) SaveConversation(ctx context.Context, record ConversationRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_conversation (conversation_id, user_id, channel, status)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		   user_id = VALUES(user_id),
		   channel = VALUES(channel),
		   status = VALUES(status),
		   updated_at = CURRENT_TIMESTAMP`,
		record.ConversationID,
		record.UserID,
		record.Channel,
		record.Status,
	)
	if err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}

func (s *MySQLStore) SaveMessage(ctx context.Context, record MessageRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_message (message_id, conversation_id, sender_type, message_type, content)
		 VALUES (?, ?, ?, ?, ?)`,
		record.MessageID,
		record.ConversationID,
		record.SenderType,
		record.MessageType,
		record.Content,
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}
