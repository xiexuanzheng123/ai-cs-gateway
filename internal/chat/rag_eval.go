package chat

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type RAGEvalRunResult struct {
	Total          int                    `json:"total"`
	Passed         int                    `json:"passed"`
	Failed         int                    `json:"failed"`
	PassRate       float64                `json:"pass_rate"`
	DurationMS     int                    `json:"duration_ms"`
	QualitySummary RAGEvalQualitySummary  `json:"quality_summary"`
	Items          []RAGEvalRunItemResult `json:"items"`
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

type RAGEvalQualitySummary struct {
	Top1HitRate             float64 `json:"top1_hit_rate"`
	Top3HitRate             float64 `json:"top3_hit_rate"`
	ShouldAnswerMissCount   int     `json:"should_answer_miss_count"`
	ShouldNotAnswerHitCount int     `json:"should_not_answer_hit_count"`
	MissingCitationCount    int     `json:"missing_citation_count"`
	SuspectedHallucination  int     `json:"suspected_hallucination"`
	MajorUnsafeCount        int     `json:"major_unsafe_count"`
	MajorUnsafeRate         float64 `json:"major_unsafe_rate"`
	AverageCaseLatencyMS    int     `json:"average_case_latency_ms"`
}

type RAGEvalSeedResult struct {
	TargetTotal     int `json:"target_total"`
	BeforeTotal     int `json:"before_total"`
	Created         int `json:"created"`
	NegativeCreated int `json:"negative_created"`
	AfterTotal      int `json:"after_total"`
}

var defaultRAGEvalNegativeCases = []RAGEvalCaseRecord{
	{
		CaseID:       "neg_mars_concert_ticket",
		QueryText:    "火星演唱会门票怎么领取",
		ShouldAnswer: false,
		Status:       "active",
	},
	{
		CaseID:       "neg_weather_query",
		QueryText:    "今天北京天气怎么样",
		ShouldAnswer: false,
		Status:       "active",
	},
	{
		CaseID:       "neg_flight_booking",
		QueryText:    "帮我订一张明天飞纽约的机票",
		ShouldAnswer: false,
		Status:       "active",
	},
	{
		CaseID:       "neg_stock_price",
		QueryText:    "特斯拉股价现在是多少",
		ShouldAnswer: false,
		Status:       "active",
	},
	{
		CaseID:       "neg_game_rank",
		QueryText:    "王者荣耀怎么快速上王者",
		ShouldAnswer: false,
		Status:       "active",
	},
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
	result.QualitySummary = summarizeRAGEvalQuality(result.Items)
	result.DurationMS = elapsedMilliseconds(startedAt)
	saved, err := s.store.SaveRAGEvalRun(ctx, ragEvalRunRecordFromResult(newID("rag-eval"), result))
	if err != nil {
		return RAGEvalRunResult{}, err
	}
	return ragEvalRunResultFromRecord(saved), nil
}

func (s *Service) SeedRAGEvalCases(ctx context.Context, targetTotal int) (RAGEvalSeedResult, error) {
	if targetTotal <= 0 {
		targetTotal = 200
	}
	if targetTotal > 500 {
		targetTotal = 500
	}

	existingCases, err := s.store.ListRAGEvalCases(ctx)
	if err != nil {
		return RAGEvalSeedResult{}, err
	}
	result := RAGEvalSeedResult{
		TargetTotal: targetTotal,
		BeforeTotal: len(existingCases),
		AfterTotal:  len(existingCases),
	}

	existingCaseIDs := map[string]struct{}{}
	autoPositiveCount := 0
	for _, item := range existingCases {
		existingCaseIDs[item.CaseID] = struct{}{}
		if strings.HasPrefix(item.CaseID, "auto_") {
			autoPositiveCount++
		}
	}

	if autoPositiveCount < targetTotal {
		knowledge, err := s.store.ListKnowledge(ctx)
		if err != nil {
			return RAGEvalSeedResult{}, err
		}
		for _, record := range knowledge {
			if autoPositiveCount >= targetTotal {
				break
			}
			if record.Status != "published" || strings.TrimSpace(record.Question) == "" {
				continue
			}
			caseID := "auto_" + record.KnowledgeID
			if _, exists := existingCaseIDs[caseID]; exists {
				continue
			}
			_, err := s.store.CreateRAGEvalCase(ctx, RAGEvalCaseRecord{
				CaseID:              caseID,
				QueryText:           record.Question,
				ExpectedKnowledgeID: record.KnowledgeID,
				ExpectedIntent:      record.Category,
				ShouldAnswer:        true,
				Status:              "active",
			})
			if err != nil {
				return RAGEvalSeedResult{}, err
			}
			existingCaseIDs[caseID] = struct{}{}
			autoPositiveCount++
			result.Created++
			result.AfterTotal++
		}
	}

	for _, negativeCase := range defaultRAGEvalNegativeCases {
		if _, exists := existingCaseIDs[negativeCase.CaseID]; exists {
			continue
		}
		_, err := s.store.CreateRAGEvalCase(ctx, negativeCase)
		if err != nil {
			return RAGEvalSeedResult{}, err
		}
		existingCaseIDs[negativeCase.CaseID] = struct{}{}
		result.NegativeCreated++
		result.AfterTotal++
	}
	return result, nil
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

func summarizeRAGEvalQuality(items []RAGEvalRunItemResult) RAGEvalQualitySummary {
	summary := RAGEvalQualitySummary{}
	if len(items) == 0 {
		return summary
	}
	expectedCases := 0
	top1Hits := 0
	top3Hits := 0
	totalDuration := 0
	for _, item := range items {
		totalDuration += item.DurationMS
		if item.ShouldAnswer && !item.Matched {
			summary.ShouldAnswerMissCount++
			summary.MajorUnsafeCount++
		}
		if !item.ShouldAnswer && item.Matched {
			summary.ShouldNotAnswerHitCount++
			summary.SuspectedHallucination++
			summary.MajorUnsafeCount++
		}
		if item.ShouldAnswer && item.Matched && len(item.Matches) == 0 {
			summary.MissingCitationCount++
			summary.SuspectedHallucination++
			summary.MajorUnsafeCount++
		}

		expectedKnowledgeID := strings.TrimSpace(item.ExpectedKnowledgeID)
		if expectedKnowledgeID == "" {
			continue
		}
		expectedCases++
		if item.Top1KnowledgeID == expectedKnowledgeID {
			top1Hits++
			top3Hits++
			continue
		}
		for _, match := range item.Matches {
			if match.KnowledgeID == expectedKnowledgeID {
				top3Hits++
				break
			}
		}
	}
	if expectedCases > 0 {
		summary.Top1HitRate = float64(top1Hits) / float64(expectedCases)
		summary.Top3HitRate = float64(top3Hits) / float64(expectedCases)
	}
	summary.MajorUnsafeRate = float64(summary.MajorUnsafeCount) / float64(len(items))
	summary.AverageCaseLatencyMS = totalDuration / len(items)
	return summary
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
		Total:          record.Total,
		Passed:         record.Passed,
		Failed:         record.Failed,
		PassRate:       record.PassRate,
		DurationMS:     record.DurationMS,
		QualitySummary: summarizeRAGEvalQuality(items),
		Items:          items,
	}
}
