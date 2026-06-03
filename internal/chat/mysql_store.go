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

func (s *MySQLStore) ListRules(ctx context.Context) ([]RuleConfigRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, rule_type, pattern, action, priority, enabled, COALESCE(description, '')
		 FROM cs_rule_config
		 ORDER BY priority DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return scanRules(rows)
}

func (s *MySQLStore) ListEnabledRules(ctx context.Context) ([]RuleConfigRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, rule_type, pattern, action, priority, enabled, COALESCE(description, '')
		 FROM cs_rule_config
		 WHERE enabled = 1
		 ORDER BY priority DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return scanRules(rows)
}

func scanRules(rows *sql.Rows) ([]RuleConfigRecord, error) {
	defer rows.Close()

	rules := []RuleConfigRecord{}
	for rows.Next() {
		var rule RuleConfigRecord
		if err := rows.Scan(
			&rule.ID,
			&rule.RuleType,
			&rule.Pattern,
			&rule.Action,
			&rule.Priority,
			&rule.Enabled,
			&rule.Description,
		); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return rules, nil
}

func (s *MySQLStore) CreateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_rule_config
		 (rule_type, pattern, action, priority, enabled, description)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.RuleType,
		record.Pattern,
		record.Action,
		record.Priority,
		record.Enabled,
		record.Description,
	)
	if err != nil {
		return RuleConfigRecord{}, fmt.Errorf("create rule: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RuleConfigRecord{}, fmt.Errorf("get rule id: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *MySQLStore) UpdateRule(ctx context.Context, record RuleConfigRecord) (RuleConfigRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE cs_rule_config
		 SET rule_type = ?, pattern = ?, action = ?, priority = ?, enabled = ?, description = ?
		 WHERE id = ?`,
		record.RuleType,
		record.Pattern,
		record.Action,
		record.Priority,
		record.Enabled,
		record.Description,
		record.ID,
	)
	if err != nil {
		return RuleConfigRecord{}, fmt.Errorf("update rule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RuleConfigRecord{}, fmt.Errorf("get affected rows: %w", err)
	}
	if affected == 0 {
		return RuleConfigRecord{}, fmt.Errorf("rule not found")
	}
	return record, nil
}

func (s *MySQLStore) GetFeatureFlag(ctx context.Context, key string) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(
		ctx,
		`SELECT enabled FROM cs_feature_flag WHERE flag_key = ?`,
		key,
	).Scan(&enabled)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get feature flag: %w", err)
	}
	return enabled, nil
}

func (s *MySQLStore) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT flag_key, enabled, COALESCE(description, '')
		 FROM cs_feature_flag
		 ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	defer rows.Close()

	flags := []FeatureFlagRecord{}
	for rows.Next() {
		var flag FeatureFlagRecord
		if err := rows.Scan(&flag.Key, &flag.Enabled, &flag.Description); err != nil {
			return nil, fmt.Errorf("scan feature flag: %w", err)
		}
		flags = append(flags, flag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature flags: %w", err)
	}
	return flags, nil
}

func (s *MySQLStore) SetFeatureFlag(ctx context.Context, key string, enabled bool) (FeatureFlagRecord, error) {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_feature_flag (flag_key, enabled, description)
		 VALUES (?, ?, '')
		 ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), updated_at = CURRENT_TIMESTAMP`,
		key,
		enabled,
	)
	if err != nil {
		return FeatureFlagRecord{}, fmt.Errorf("set feature flag: %w", err)
	}
	var flag FeatureFlagRecord
	err = s.db.QueryRowContext(
		ctx,
		`SELECT flag_key, enabled, COALESCE(description, '')
		 FROM cs_feature_flag
		 WHERE flag_key = ?`,
		key,
	).Scan(&flag.Key, &flag.Enabled, &flag.Description)
	if err != nil {
		return FeatureFlagRecord{}, fmt.Errorf("get updated feature flag: %w", err)
	}
	return flag, nil
}

func (s *MySQLStore) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats
	queries := []struct {
		name string
		dest *int64
		sql  string
	}{
		{name: "conversation count", dest: &stats.TotalConversations, sql: `SELECT COUNT(*) FROM cs_conversation`},
		{name: "message count", dest: &stats.TotalMessages, sql: `SELECT COUNT(*) FROM cs_message`},
		{name: "event count", dest: &stats.TotalAIEvents, sql: `SELECT COUNT(*) FROM cs_ai_event`},
		{name: "rule hit count", dest: &stats.RuleHitCount, sql: `SELECT COUNT(*) FROM cs_ai_event WHERE route IN ('db_rule', 'fixed_faq', 'rule_handoff', 'media_guide', 'fixed_faq_fallback')`},
		{name: "handoff count", dest: &stats.HandoffCount, sql: `SELECT COUNT(*) FROM cs_ai_event WHERE handoff_required = 1`},
		{name: "feedback count", dest: &stats.FeedbackCount, sql: `SELECT COUNT(*) FROM cs_feedback`},
		{name: "positive feedback", dest: &stats.PositiveFeedback, sql: `SELECT COUNT(*) FROM cs_feedback WHERE rating = 'thumbs_up'`},
		{name: "negative feedback", dest: &stats.NegativeFeedback, sql: `SELECT COUNT(*) FROM cs_feedback WHERE rating = 'thumbs_down'`},
	}
	for _, query := range queries {
		if err := s.db.QueryRowContext(ctx, query.sql).Scan(query.dest); err != nil {
			return DashboardStats{}, fmt.Errorf("%s: %w", query.name, err)
		}
	}

	var average sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, `SELECT AVG(latency_ms) FROM cs_ai_event WHERE latency_ms IS NOT NULL`).Scan(&average); err != nil {
		return DashboardStats{}, fmt.Errorf("average latency: %w", err)
	}
	if average.Valid {
		stats.AverageLatencyMS = average.Float64
	}
	return stats, nil
}

func (s *MySQLStore) ListKnowledge(ctx context.Context) ([]KnowledgeRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, knowledge_id, title, content, category, COALESCE(owner, ''), version, status
		 FROM cs_knowledge
		 ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list knowledge: %w", err)
	}
	defer rows.Close()

	records := []KnowledgeRecord{}
	for rows.Next() {
		var record KnowledgeRecord
		if err := rows.Scan(
			&record.ID,
			&record.KnowledgeID,
			&record.Title,
			&record.Content,
			&record.Category,
			&record.Owner,
			&record.Version,
			&record.Status,
		); err != nil {
			return nil, fmt.Errorf("scan knowledge: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge: %w", err)
	}
	return records, nil
}

func (s *MySQLStore) CreateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_knowledge
		 (knowledge_id, title, content, category, owner, version, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		record.KnowledgeID,
		record.Title,
		record.Content,
		record.Category,
		record.Owner,
		record.Version,
		record.Status,
	)
	if err != nil {
		return KnowledgeRecord{}, fmt.Errorf("create knowledge: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return KnowledgeRecord{}, fmt.Errorf("get knowledge id: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *MySQLStore) UpdateKnowledge(ctx context.Context, record KnowledgeRecord) (KnowledgeRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE cs_knowledge
		 SET knowledge_id = ?, title = ?, content = ?, category = ?, owner = ?, version = ?, status = ?
		 WHERE id = ?`,
		record.KnowledgeID,
		record.Title,
		record.Content,
		record.Category,
		record.Owner,
		record.Version,
		record.Status,
		record.ID,
	)
	if err != nil {
		return KnowledgeRecord{}, fmt.Errorf("update knowledge: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return KnowledgeRecord{}, fmt.Errorf("get knowledge affected rows: %w", err)
	}
	if affected == 0 {
		return KnowledgeRecord{}, fmt.Errorf("knowledge not found")
	}
	return record, nil
}

func (s *MySQLStore) ListRAGEvalCases(ctx context.Context) ([]RAGEvalCaseRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, case_id, query_text, COALESCE(expected_knowledge_id, ''),
		        COALESCE(expected_intent, ''), should_answer, status
		 FROM cs_rag_eval_case
		 ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list rag eval cases: %w", err)
	}
	defer rows.Close()

	records := []RAGEvalCaseRecord{}
	for rows.Next() {
		var record RAGEvalCaseRecord
		if err := rows.Scan(
			&record.ID,
			&record.CaseID,
			&record.QueryText,
			&record.ExpectedKnowledgeID,
			&record.ExpectedIntent,
			&record.ShouldAnswer,
			&record.Status,
		); err != nil {
			return nil, fmt.Errorf("scan rag eval case: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rag eval cases: %w", err)
	}
	return records, nil
}

func (s *MySQLStore) CreateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO cs_rag_eval_case
		 (case_id, query_text, expected_knowledge_id, expected_intent, should_answer, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.CaseID,
		record.QueryText,
		record.ExpectedKnowledgeID,
		record.ExpectedIntent,
		record.ShouldAnswer,
		record.Status,
	)
	if err != nil {
		return RAGEvalCaseRecord{}, fmt.Errorf("create rag eval case: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RAGEvalCaseRecord{}, fmt.Errorf("get rag eval case id: %w", err)
	}
	record.ID = id
	return record, nil
}

func (s *MySQLStore) UpdateRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalCaseRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE cs_rag_eval_case
		 SET case_id = ?, query_text = ?, expected_knowledge_id = ?, expected_intent = ?, should_answer = ?, status = ?
		 WHERE id = ?`,
		record.CaseID,
		record.QueryText,
		record.ExpectedKnowledgeID,
		record.ExpectedIntent,
		record.ShouldAnswer,
		record.Status,
		record.ID,
	)
	if err != nil {
		return RAGEvalCaseRecord{}, fmt.Errorf("update rag eval case: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RAGEvalCaseRecord{}, fmt.Errorf("get rag eval case affected rows: %w", err)
	}
	if affected == 0 {
		return RAGEvalCaseRecord{}, fmt.Errorf("rag eval case not found")
	}
	return record, nil
}
