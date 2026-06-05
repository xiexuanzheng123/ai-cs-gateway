package chat

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type RAGEvalRunResult struct {
	Total      int                    `json:"total"`
	Passed     int                    `json:"passed"`
	Failed     int                    `json:"failed"`
	PassRate   float64                `json:"pass_rate"`
	DurationMS int                    `json:"duration_ms"`
	Items      []RAGEvalRunItemResult `json:"items"`
}

type RAGEvalRunItemResult struct {
	CaseID              string            `json:"case_id"`
	QueryText           string            `json:"query_text"`
	ExpectedKnowledgeID string            `json:"expected_knowledge_id"`
	ShouldAnswer        bool              `json:"should_answer"`
	Matched             bool              `json:"matched"`
	Passed              bool              `json:"passed"`
	Reason              string            `json:"reason"`
	Top1KnowledgeID     string            `json:"top1_knowledge_id"`
	Top1Score           float64           `json:"top1_score"`
	Matches             []RAGSearchResult `json:"matches"`
	DurationMS          int               `json:"duration_ms"`
}

func (s *Service) RunRAGEvalCases(ctx context.Context) (RAGEvalRunResult, error) {
	startedAt := time.Now()
	cases, err := s.store.ListRAGEvalCases(ctx)
	if err != nil {
		return RAGEvalRunResult{}, err
	}

	result := RAGEvalRunResult{
		Items: []RAGEvalRunItemResult{},
	}
	for _, item := range cases {
		if item.Status != "active" {
			continue
		}
		result.Total++
		itemResult, err := s.runRAGEvalCase(ctx, item)
		if err != nil {
			return RAGEvalRunResult{}, err
		}
		if itemResult.Passed {
			result.Passed++
		} else {
			result.Failed++
		}
		result.Items = append(result.Items, itemResult)
	}
	if result.Total > 0 {
		result.PassRate = float64(result.Passed) / float64(result.Total)
	}
	result.DurationMS = elapsedMilliseconds(startedAt)
	saved, err := s.store.SaveRAGEvalRun(ctx, ragEvalRunRecordFromResult(newID("rag-eval"), result))
	if err != nil {
		return RAGEvalRunResult{}, err
	}
	return ragEvalRunResultFromRecord(saved), nil
}

func (s *Service) runRAGEvalCase(ctx context.Context, record RAGEvalCaseRecord) (RAGEvalRunItemResult, error) {
	startedAt := time.Now()
	_, matches, matched, _, err := s.searchRAG(ctx, record.QueryText, 3)
	if err != nil {
		return RAGEvalRunItemResult{}, err
	}

	result := RAGEvalRunItemResult{
		CaseID:              record.CaseID,
		QueryText:           record.QueryText,
		ExpectedKnowledgeID: record.ExpectedKnowledgeID,
		ShouldAnswer:        record.ShouldAnswer,
		Matched:             matched,
		Matches:             matches,
		DurationMS:          elapsedMilliseconds(startedAt),
	}
	if len(matches) > 0 {
		result.Top1KnowledgeID = matches[0].KnowledgeID
		result.Top1Score = matches[0].Score
	}

	result.Passed, result.Reason = evaluateRAGCase(record, matched, matches)
	return result, nil
}

func evaluateRAGCase(record RAGEvalCaseRecord, matched bool, matches []RAGSearchResult) (bool, string) {
	if record.ShouldAnswer != matched {
		return false, fmt.Sprintf("should_answer=%t matched=%t", record.ShouldAnswer, matched)
	}
	if !record.ShouldAnswer {
		return true, "预期不回答，实际未命中"
	}

	expectedKnowledgeID := strings.TrimSpace(record.ExpectedKnowledgeID)
	if expectedKnowledgeID == "" {
		return true, "未配置期望知识，仅校验应答"
	}
	if len(matches) == 0 {
		return false, "没有召回候选"
	}
	if matches[0].KnowledgeID == expectedKnowledgeID {
		return true, "Top1 命中期望知识"
	}
	for _, match := range matches {
		if match.KnowledgeID == expectedKnowledgeID {
			return true, "Top3 命中期望知识"
		}
	}
	return false, fmt.Sprintf("未命中期望知识，Top1=%s", matches[0].KnowledgeID)
}

func ragEvalRunRecordFromResult(runID string, result RAGEvalRunResult) RAGEvalRunRecord {
	items := make([]RAGEvalRunItemRecord, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, RAGEvalRunItemRecord{
			RunID:               runID,
			CaseID:              item.CaseID,
			QueryText:           item.QueryText,
			ExpectedKnowledgeID: item.ExpectedKnowledgeID,
			ShouldAnswer:        item.ShouldAnswer,
			Matched:             item.Matched,
			Passed:              item.Passed,
			Reason:              item.Reason,
			Top1KnowledgeID:     item.Top1KnowledgeID,
			Top1Score:           item.Top1Score,
			Matches:             item.Matches,
			DurationMS:          item.DurationMS,
		})
	}
	return RAGEvalRunRecord{
		RunID:      runID,
		Total:      result.Total,
		Passed:     result.Passed,
		Failed:     result.Failed,
		PassRate:   result.PassRate,
		DurationMS: result.DurationMS,
		Items:      items,
	}
}

func ragEvalRunResultFromRecord(record RAGEvalRunRecord) RAGEvalRunResult {
	items := make([]RAGEvalRunItemResult, 0, len(record.Items))
	for _, item := range record.Items {
		items = append(items, RAGEvalRunItemResult{
			CaseID:              item.CaseID,
			QueryText:           item.QueryText,
			ExpectedKnowledgeID: item.ExpectedKnowledgeID,
			ShouldAnswer:        item.ShouldAnswer,
			Matched:             item.Matched,
			Passed:              item.Passed,
			Reason:              item.Reason,
			Top1KnowledgeID:     item.Top1KnowledgeID,
			Top1Score:           item.Top1Score,
			Matches:             item.Matches,
			DurationMS:          item.DurationMS,
		})
	}
	return RAGEvalRunResult{
		Total:      record.Total,
		Passed:     record.Passed,
		Failed:     record.Failed,
		PassRate:   record.PassRate,
		DurationMS: record.DurationMS,
		Items:      items,
	}
}
