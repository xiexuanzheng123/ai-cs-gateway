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

func (s *MySQLStore) SaveAIEvent(ctx context.Context, record AIEventRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_ai_event
		 (trace_id, conversation_id, message_id, intent, route, response_type,
		  handoff_required, handoff_reason, latency_ms, model_used)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.TraceID,
		record.ConversationID,
		record.MessageID,
		record.Intent,
		record.Route,
		record.ResponseType,
		record.HandoffRequired,
		record.HandoffReason,
		record.LatencyMS,
		record.ModelUsed,
	)
	if err != nil {
		return fmt.Errorf("save ai event: %w", err)
	}
	return nil
}

func (s *MySQLStore) SaveFeedback(ctx context.Context, record FeedbackRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_feedback
		 (conversation_id, message_id, user_id, rating, comment, action_taken)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.ConversationID,
		record.MessageID,
		record.UserID,
		record.Rating,
		record.Comment,
		record.ActionTaken,
	)
	if err != nil {
		return fmt.Errorf("save feedback: %w", err)
	}
	return nil
}

func (s *MySQLStore) SaveHandoff(ctx context.Context, record HandoffRecord) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_handoff_event
		 (handoff_id, conversation_id, message_id, user_id, reason, source, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.HandoffID,
		record.ConversationID,
		record.MessageID,
		record.UserID,
		record.Reason,
		record.Source,
		record.Status,
	)
	if err != nil {
		return fmt.Errorf("save handoff: %w", err)
	}
	return nil
}
