package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ai-cs-gateway/internal/ai"

	"github.com/redis/go-redis/v9"
)

const sessionMemoryTTL = 24 * time.Hour
const sessionMemoryMaxMessages = 6

type SessionMemory interface {
	Get(ctx context.Context, conversationID string) (SessionMemoryRecord, bool, error)
	Set(ctx context.Context, record SessionMemoryRecord) error
}

type SessionMemoryRecord struct {
	ConversationID string                 `json:"conversation_id"`
	Messages       []SessionMemoryMessage `json:"messages"`
	LastIntent     string                 `json:"last_intent"`
	LastRoute      string                 `json:"last_route"`
	LastCitations  []CitationRecord       `json:"last_citations"`
	UpdatedAt      int64                  `json:"updated_at"`
}

type SessionMemoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RedisSessionMemory struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisSessionMemory(client *redis.Client, ttl time.Duration) *RedisSessionMemory {
	if ttl <= 0 {
		ttl = sessionMemoryTTL
	}
	return &RedisSessionMemory{
		client: client,
		ttl:    ttl,
	}
}

func (m *RedisSessionMemory) Get(ctx context.Context, conversationID string) (SessionMemoryRecord, bool, error) {
	if m == nil || m.client == nil {
		return SessionMemoryRecord{}, false, nil
	}
	value, err := m.client.Get(ctx, sessionMemoryKey(conversationID)).Result()
	if err == redis.Nil {
		return SessionMemoryRecord{}, false, nil
	}
	if err != nil {
		return SessionMemoryRecord{}, false, err
	}
	var record SessionMemoryRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return SessionMemoryRecord{}, false, err
	}
	return record, true, nil
}

func (m *RedisSessionMemory) Set(ctx context.Context, record SessionMemoryRecord) error {
	if m == nil || m.client == nil || record.ConversationID == "" {
		return nil
	}
	record.Messages = trimSessionMessages(record.Messages)
	record.UpdatedAt = time.Now().Unix()
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return m.client.Set(ctx, sessionMemoryKey(record.ConversationID), body, m.ttl).Err()
}

func sessionMemoryKey(conversationID string) string {
	return fmt.Sprintf("ai-cs:session:%s", conversationID)
}

func memoryHistory(record SessionMemoryRecord) []ai.HistoryItem {
	messages := trimSessionMessages(record.Messages)
	history := make([]ai.HistoryItem, 0, len(messages))
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		if message.Content == "" {
			continue
		}
		history = append(history, ai.HistoryItem{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return history
}

func nextSessionMemory(record SessionMemoryRecord, request SendMessageRequest, response SendMessageResponse) SessionMemoryRecord {
	record.ConversationID = request.ConversationID
	record.Messages = append(record.Messages,
		SessionMemoryMessage{Role: "user", Content: request.Message},
		SessionMemoryMessage{Role: "assistant", Content: response.Content.Text},
	)
	record.Messages = trimSessionMessages(record.Messages)
	record.LastIntent = response.Intent
	record.LastRoute = response.Route
	record.LastCitations = response.Citations
	return record
}

func trimSessionMessages(messages []SessionMemoryMessage) []SessionMemoryMessage {
	if len(messages) <= sessionMemoryMaxMessages {
		return messages
	}
	return append([]SessionMemoryMessage(nil), messages[len(messages)-sessionMemoryMaxMessages:]...)
}
