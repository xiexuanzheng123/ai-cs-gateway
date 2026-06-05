package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type MySQLStore struct {
	db *sql.DB
}

func nullParentID(parentID int64) sql.NullInt64 {
	if parentID <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parentID, Valid: true}
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

func (s *MySQLStore) SaveTraceLog(ctx context.Context, record TraceLogRecord) error {
	// stages/rag_matches 保留为 JSON，方便后台一次性展示完整链路详情。
	stagesJSON, err := json.Marshal(record.Stages)
	if err != nil {
		return fmt.Errorf("marshal trace stages: %w", err)
	}
	ragMatchesJSON, err := json.Marshal(record.RAGMatches)
	if err != nil {
		return fmt.Errorf("marshal trace rag matches: %w", err)
	}
	citationsJSON, err := json.Marshal(record.Citations)
	if err != nil {
		return fmt.Errorf("marshal trace citations: %w", err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO cs_trace_log
		 (trace_id, conversation_id, message_id, user_id, channel, message_type,
		  user_message, response_text, intent, route, response_type, risk_level,
		  handoff_required, handoff_reason, model_used, total_latency_ms,
		  stages, rag_matches, citations, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		  response_text = VALUES(response_text),
		  intent = VALUES(intent),
		  route = VALUES(route),
		  response_type = VALUES(response_type),
		  risk_level = VALUES(risk_level),
		  handoff_required = VALUES(handoff_required),
		  handoff_reason = VALUES(handoff_reason),
		  model_used = VALUES(model_used),
		  total_latency_ms = VALUES(total_latency_ms),
		  stages = VALUES(stages),
		  rag_matches = VALUES(rag_matches),
		  citations = VALUES(citations),
		  error_message = VALUES(error_message)`,
		record.TraceID,
		record.ConversationID,
		record.MessageID,
		record.UserID,
		record.Channel,
		record.MessageType,
		record.UserMessage,
		record.ResponseText,
		record.Intent,
		record.Route,
		record.ResponseType,
		record.RiskLevel,
		record.HandoffRequired,
		record.HandoffReason,
		record.ModelUsed,
		record.TotalLatencyMS,
		string(stagesJSON),
		string(ragMatchesJSON),
		string(citationsJSON),
		record.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("save trace log: %w", err)
	}
	return nil
}

func (s *MySQLStore) ListTraceLogs(ctx context.Context, limit int) ([]TraceLogRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, trace_id, conversation_id, message_id, user_id, channel, message_type,
		        COALESCE(user_message, ''), COALESCE(response_text, ''), COALESCE(intent, ''),
		        COALESCE(route, ''), COALESCE(response_type, ''), COALESCE(risk_level, ''),
		        handoff_required, COALESCE(handoff_reason, ''), COALESCE(model_used, ''),
		        COALESCE(total_latency_ms, 0), COALESCE(stages, JSON_ARRAY()),
		        COALESCE(rag_matches, JSON_ARRAY()), COALESCE(citations, JSON_ARRAY()),
		        COALESCE(error_message, ''),
		        DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s')
		 FROM cs_trace_log
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list trace logs: %w", err)
	}
	defer rows.Close()

	records := []TraceLogRecord{}
	for rows.Next() {
		var record TraceLogRecord
		var stagesRaw string
		var ragMatchesRaw string
		var citationsRaw string
		if err := rows.Scan(
			&record.ID,
			&record.TraceID,
			&record.ConversationID,
			&record.MessageID,
			&record.UserID,
			&record.Channel,
			&record.MessageType,
			&record.UserMessage,
			&record.ResponseText,
			&record.Intent,
			&record.Route,
			&record.ResponseType,
			&record.RiskLevel,
			&record.HandoffRequired,
			&record.HandoffReason,
			&record.ModelUsed,
			&record.TotalLatencyMS,
			&stagesRaw,
			&ragMatchesRaw,
			&citationsRaw,
			&record.ErrorMessage,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan trace log: %w", err)
		}
		_ = json.Unmarshal([]byte(stagesRaw), &record.Stages)
		_ = json.Unmarshal([]byte(ragMatchesRaw), &record.RAGMatches)
		_ = json.Unmarshal([]byte(citationsRaw), &record.Citations)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace logs: %w", err)
	}
	return records, nil
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
		// 没配置时默认开启，避免本地初始化漏配导致智能回复整条链路关闭。
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

func (s *MySQLStore) ListCategories(ctx context.Context) ([]CategoryRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT c.id, COALESCE(c.parent_id, 0), c.name, c.level, c.path, c.sort_order,
		        COUNT(child.id) AS child_count
		 FROM cs_category c
		 LEFT JOIN cs_category child ON child.parent_id = c.id
		 GROUP BY c.id, c.parent_id, c.name, c.level, c.path, c.sort_order
		 ORDER BY c.path, c.sort_order, c.id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	records := []CategoryRecord{}
	for rows.Next() {
		var record CategoryRecord
		if err := rows.Scan(
			&record.ID,
			&record.ParentID,
			&record.Name,
			&record.Level,
			&record.Path,
			&record.SortOrder,
			&record.ChildCount,
		); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	return records, nil
}

func (s *MySQLStore) CreateCategory(ctx context.Context, record CategoryRecord) (CategoryRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CategoryRecord{}, fmt.Errorf("begin create category: %w", err)
	}
	defer tx.Rollback()

	level := 1
	parentPath := ""
	if record.ParentID > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT level, path FROM cs_category WHERE id = ?`, record.ParentID).Scan(&level, &parentPath); err != nil {
			return CategoryRecord{}, fmt.Errorf("get parent category: %w", err)
		}
		level++
		if level > 3 {
			return CategoryRecord{}, fmt.Errorf("category level cannot exceed 3")
		}
	}

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO cs_category (parent_id, name, level, path, sort_order)
		 VALUES (?, ?, ?, '', ?)`,
		nullParentID(record.ParentID),
		record.Name,
		level,
		record.SortOrder,
	)
	if err != nil {
		return CategoryRecord{}, fmt.Errorf("create category: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return CategoryRecord{}, fmt.Errorf("get category id: %w", err)
	}
	path := fmt.Sprintf("/%d", id)
	if parentPath != "" {
		path = fmt.Sprintf("%s/%d", parentPath, id)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE cs_category SET path = ? WHERE id = ?`, path, id); err != nil {
		return CategoryRecord{}, fmt.Errorf("update category path: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CategoryRecord{}, fmt.Errorf("commit category: %w", err)
	}

	record.ID = id
	record.Level = level
	record.Path = path
	return record, nil
}

func (s *MySQLStore) UpdateCategory(ctx context.Context, record CategoryRecord) (CategoryRecord, error) {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE cs_category SET name = ?, sort_order = ? WHERE id = ?`,
		record.Name,
		record.SortOrder,
		record.ID,
	)
	if err != nil {
		return CategoryRecord{}, fmt.Errorf("update category: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CategoryRecord{}, fmt.Errorf("get category affected rows: %w", err)
	}
	if affected == 0 {
		return CategoryRecord{}, fmt.Errorf("category not found")
	}
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT id, COALESCE(parent_id, 0), name, level, path, sort_order
		 FROM cs_category WHERE id = ?`,
		record.ID,
	).Scan(&record.ID, &record.ParentID, &record.Name, &record.Level, &record.Path, &record.SortOrder); err != nil {
		return CategoryRecord{}, fmt.Errorf("get updated category: %w", err)
	}
	return record, nil
}

func (s *MySQLStore) DeleteCategory(ctx context.Context, id int64) error {
	var childCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cs_category WHERE parent_id = ?`, id).Scan(&childCount); err != nil {
		return fmt.Errorf("count child categories: %w", err)
	}
	if childCount > 0 {
		return fmt.Errorf("category has child categories")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM cs_category WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get category delete affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("category not found")
	}
	return nil
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
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cs_knowledge WHERE id = ?`, record.ID).Scan(&exists); err != nil {
			return KnowledgeRecord{}, fmt.Errorf("check knowledge existence: %w", err)
		}
		if exists == 0 {
			return KnowledgeRecord{}, fmt.Errorf("knowledge not found")
		}
	}
	return record, nil
}

func (s *MySQLStore) ReplaceKnowledgeChunks(ctx context.Context, records []KnowledgeChunkRecord) error {
	return s.replaceKnowledgeChunks(ctx, "", records)
}

func (s *MySQLStore) ReplaceKnowledgeChunksByKnowledgeID(ctx context.Context, knowledgeID string, records []KnowledgeChunkRecord) error {
	return s.replaceKnowledgeChunks(ctx, knowledgeID, records)
}

func (s *MySQLStore) replaceKnowledgeChunks(ctx context.Context, knowledgeID string, records []KnowledgeChunkRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace knowledge chunks: %w", err)
	}
	defer tx.Rollback()

	// chunk 采用先删后插，避免知识内容变短时残留旧 chunk 被继续召回。
	if knowledgeID == "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cs_knowledge_chunk`); err != nil {
			return fmt.Errorf("clear knowledge chunks: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cs_knowledge_chunk WHERE knowledge_id = ?`, knowledgeID); err != nil {
			return fmt.Errorf("clear knowledge chunks by knowledge id: %w", err)
		}
	}

	stmt, err := tx.PrepareContext(
		ctx,
		`INSERT INTO cs_knowledge_chunk
		 (chunk_id, knowledge_id, version, chunk_text, token_count, vector_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare knowledge chunk insert: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		if _, err := stmt.ExecContext(
			ctx,
			record.ChunkID,
			record.KnowledgeID,
			record.Version,
			record.ChunkText,
			record.TokenCount,
			record.VectorID,
		); err != nil {
			return fmt.Errorf("insert knowledge chunk: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit knowledge chunks: %w", err)
	}
	return nil
}

func (s *MySQLStore) ListKnowledgeChunksWithoutVector(ctx context.Context) ([]KnowledgeChunkRecord, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT chunk_id, knowledge_id, version, chunk_text, token_count, COALESCE(vector_id, '')
		 FROM cs_knowledge_chunk
		 WHERE vector_id IS NULL OR vector_id = ''
		 ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list knowledge chunks without vector: %w", err)
	}
	defer rows.Close()

	records := []KnowledgeChunkRecord{}
	for rows.Next() {
		var record KnowledgeChunkRecord
		if err := rows.Scan(
			&record.ChunkID,
			&record.KnowledgeID,
			&record.Version,
			&record.ChunkText,
			&record.TokenCount,
			&record.VectorID,
		); err != nil {
			return nil, fmt.Errorf("scan knowledge chunk: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate knowledge chunks: %w", err)
	}
	return records, nil
}

func (s *MySQLStore) UpdateKnowledgeChunkVectorIDs(ctx context.Context, vectorIDs map[string]string) error {
	if len(vectorIDs) == 0 {
		return nil
	}
	// Milvus 写入成功后回填 vector_id，后台可据此判断 chunk 是否已经进入向量库。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update vector ids: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `UPDATE cs_knowledge_chunk SET vector_id = ? WHERE chunk_id = ?`)
	if err != nil {
		return fmt.Errorf("prepare update vector id: %w", err)
	}
	defer stmt.Close()

	for chunkID, vectorID := range vectorIDs {
		if _, err := stmt.ExecContext(ctx, vectorID, chunkID); err != nil {
			return fmt.Errorf("update vector id: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit vector ids: %w", err)
	}
	return nil
}

func (s *MySQLStore) GetKnowledgeByChunkIDs(ctx context.Context, chunkIDs []string) (map[string]RAGSearchResult, error) {
	if len(chunkIDs) == 0 {
		return map[string]RAGSearchResult{}, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(chunkIDs)), ",")
	args := make([]any, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		args = append(args, chunkID)
	}

	// Milvus 只返回 chunk_id；最终回答内容必须回查 MySQL，保证展示的是业务知识原文。
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT c.chunk_id, c.knowledge_id, k.title, k.content, c.chunk_text
		 FROM cs_knowledge_chunk c
		 JOIN cs_knowledge k ON k.knowledge_id = c.knowledge_id
		 WHERE c.chunk_id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get knowledge by chunk ids: %w", err)
	}
	defer rows.Close()

	records := map[string]RAGSearchResult{}
	for rows.Next() {
		var record RAGSearchResult
		if err := rows.Scan(
			&record.ChunkID,
			&record.KnowledgeID,
			&record.Title,
			&record.Content,
			&record.ChunkText,
		); err != nil {
			return nil, fmt.Errorf("scan rag knowledge: %w", err)
		}
		records[record.ChunkID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rag knowledge: %w", err)
	}
	return records, nil
}

func (s *MySQLStore) SearchKnowledgeByKeyword(ctx context.Context, query string, limit int) ([]RAGSearchResult, error) {
	keywords := keywordTerms(query)
	if len(keywords) == 0 {
		return []RAGSearchResult{}, nil
	}
	if limit <= 0 {
		limit = 3
	}

	whereParts := make([]string, 0, len(keywords))
	args := make([]any, 0, len(keywords)*3+3)
	for _, keyword := range keywords {
		like := "%" + keyword + "%"
		whereParts = append(whereParts, "(k.title LIKE ? OR k.content LIKE ? OR c.chunk_text LIKE ?)")
		args = append(args, like, like, like)
	}
	args = append(args, keywordOrderArgs(keywords[0])...)
	args = append(args, limit)

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT c.chunk_id, c.knowledge_id, k.title, k.content, c.chunk_text
		 FROM cs_knowledge_chunk c
		 JOIN cs_knowledge k ON k.knowledge_id = c.knowledge_id
		 WHERE k.status = 'published' AND (`+strings.Join(whereParts, " OR ")+`)
		 ORDER BY
		   CASE
		     WHEN k.title LIKE ? THEN 0
		     WHEN c.chunk_text LIKE ? THEN 1
		     ELSE 2
		   END,
		   k.updated_at DESC,
		   c.id ASC
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("search knowledge by keyword: %w", err)
	}
	defer rows.Close()

	results := []RAGSearchResult{}
	for rows.Next() {
		var record RAGSearchResult
		if err := rows.Scan(
			&record.ChunkID,
			&record.KnowledgeID,
			&record.Title,
			&record.Content,
			&record.ChunkText,
		); err != nil {
			return nil, fmt.Errorf("scan keyword knowledge: %w", err)
		}
		record.Score = keywordScore(record, keywords)
		record.Source = "keyword"
		results = append(results, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keyword knowledge: %w", err)
	}
	return results, nil
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

func (s *MySQLStore) SaveRAGEvalRun(ctx context.Context, record RAGEvalRunRecord) (RAGEvalRunRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RAGEvalRunRecord{}, fmt.Errorf("begin rag eval run tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO cs_rag_eval_run
		 (run_id, total, passed, failed, pass_rate, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.RunID,
		record.Total,
		record.Passed,
		record.Failed,
		record.PassRate,
		record.DurationMS,
	)
	if err != nil {
		return RAGEvalRunRecord{}, fmt.Errorf("save rag eval run: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return RAGEvalRunRecord{}, fmt.Errorf("get rag eval run id: %w", err)
	}
	record.ID = id

	for index := range record.Items {
		item := &record.Items[index]
		item.RunID = record.RunID
		matchesJSON, err := json.Marshal(item.Matches)
		if err != nil {
			return RAGEvalRunRecord{}, fmt.Errorf("marshal rag eval matches: %w", err)
		}
		itemResult, err := tx.ExecContext(
			ctx,
			`INSERT INTO cs_rag_eval_run_item
			 (run_id, case_id, query_text, expected_knowledge_id, should_answer,
			  matched, passed, reason, top1_knowledge_id, top1_score, matches, duration_ms)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.RunID,
			item.CaseID,
			item.QueryText,
			item.ExpectedKnowledgeID,
			item.ShouldAnswer,
			item.Matched,
			item.Passed,
			item.Reason,
			item.Top1KnowledgeID,
			item.Top1Score,
			string(matchesJSON),
			item.DurationMS,
		)
		if err != nil {
			return RAGEvalRunRecord{}, fmt.Errorf("save rag eval run item: %w", err)
		}
		item.ID, _ = itemResult.LastInsertId()
	}

	if err := tx.Commit(); err != nil {
		return RAGEvalRunRecord{}, fmt.Errorf("commit rag eval run: %w", err)
	}
	return record, nil
}

func (s *MySQLStore) ListRAGEvalRuns(ctx context.Context, limit int) ([]RAGEvalRunRecord, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, run_id, total, passed, failed, pass_rate, duration_ms,
		        DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s')
		 FROM cs_rag_eval_run
		 ORDER BY id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list rag eval runs: %w", err)
	}
	defer rows.Close()

	records := []RAGEvalRunRecord{}
	runIDs := []string{}
	for rows.Next() {
		var record RAGEvalRunRecord
		if err := rows.Scan(
			&record.ID,
			&record.RunID,
			&record.Total,
			&record.Passed,
			&record.Failed,
			&record.PassRate,
			&record.DurationMS,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan rag eval run: %w", err)
		}
		records = append(records, record)
		runIDs = append(runIDs, record.RunID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rag eval runs: %w", err)
	}
	if len(runIDs) == 0 {
		return records, nil
	}

	itemsByRunID, err := s.listRAGEvalRunItems(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	for index := range records {
		records[index].Items = itemsByRunID[records[index].RunID]
	}
	return records, nil
}

func (s *MySQLStore) listRAGEvalRunItems(ctx context.Context, runIDs []string) (map[string][]RAGEvalRunItemRecord, error) {
	placeholders := make([]string, 0, len(runIDs))
	args := make([]any, 0, len(runIDs))
	for _, runID := range runIDs {
		placeholders = append(placeholders, "?")
		args = append(args, runID)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, run_id, case_id, query_text, COALESCE(expected_knowledge_id, ''),
		        should_answer, matched, passed, COALESCE(reason, ''),
		        COALESCE(top1_knowledge_id, ''), top1_score,
		        COALESCE(matches, JSON_ARRAY()), duration_ms
		 FROM cs_rag_eval_run_item
		 WHERE run_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY id ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list rag eval run items: %w", err)
	}
	defer rows.Close()

	records := map[string][]RAGEvalRunItemRecord{}
	for rows.Next() {
		var record RAGEvalRunItemRecord
		var matchesRaw string
		if err := rows.Scan(
			&record.ID,
			&record.RunID,
			&record.CaseID,
			&record.QueryText,
			&record.ExpectedKnowledgeID,
			&record.ShouldAnswer,
			&record.Matched,
			&record.Passed,
			&record.Reason,
			&record.Top1KnowledgeID,
			&record.Top1Score,
			&matchesRaw,
			&record.DurationMS,
		); err != nil {
			return nil, fmt.Errorf("scan rag eval run item: %w", err)
		}
		_ = json.Unmarshal([]byte(matchesRaw), &record.Matches)
		records[record.RunID] = append(records[record.RunID], record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rag eval run items: %w", err)
	}
	return records, nil
}
