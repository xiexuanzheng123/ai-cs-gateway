package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type HistoryItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ReplyRequest struct {
	SessionID       string         `json:"session_id"`
	UserID          string         `json:"user_id"`
	Message         string         `json:"message"`
	History         []HistoryItem  `json:"history"`
	BusinessContext map[string]any `json:"business_context"`
}

type ReplyResponse struct {
	Reply           string   `json:"reply"`
	Intent          string   `json:"intent"`
	Route           string   `json:"route"`
	RiskLevel       string   `json:"risk_level"`
	TransferToHuman bool     `json:"transfer_to_human"`
	Suggestions     []string `json:"suggestions"`
}

type VectorChunk struct {
	ChunkID     string `json:"chunk_id"`
	KnowledgeID string `json:"knowledge_id"`
	ChunkText   string `json:"chunk_text"`
}

type VectorUpsertRequest struct {
	Chunks []VectorChunk `json:"chunks"`
}

type VectorUpsertItem struct {
	ChunkID  string `json:"chunk_id"`
	VectorID string `json:"vector_id"`
}

type VectorUpsertResponse struct {
	Items     []VectorUpsertItem `json:"items"`
	Dimension int                `json:"dimension"`
	Model     string             `json:"model"`
}

type VectorSearchRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

type VectorSearchItem struct {
	ChunkID     string  `json:"chunk_id"`
	KnowledgeID string  `json:"knowledge_id"`
	Score       float64 `json:"score"`
	ChunkText   string  `json:"chunk_text"`
}

type VectorSearchResponse struct {
	Items     []VectorSearchItem `json:"items"`
	Dimension int                `json:"dimension"`
	Model     string             `json:"model"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Reply(ctx context.Context, request ReplyRequest) (ReplyResponse, error) {
	var response ReplyResponse
	// LLM 回复统一通过 Python AI Service，Go 网关不直接持有模型 SDK。
	if err := c.post(ctx, "/v1/ai/reply", request, &response); err != nil {
		return ReplyResponse{}, err
	}
	return response, nil
}

func (c *Client) UpsertVectors(ctx context.Context, request VectorUpsertRequest) (VectorUpsertResponse, error) {
	var response VectorUpsertResponse
	// 知识库 chunk 写向量库：Python 负责 embedding 和 Milvus upsert。
	if err := c.post(ctx, "/vector/chunks/upsert", request, &response); err != nil {
		return VectorUpsertResponse{}, err
	}
	return response, nil
}

func (c *Client) SearchVectors(ctx context.Context, request VectorSearchRequest) (VectorSearchResponse, error) {
	var response VectorSearchResponse
	// RAG 检索第一段：Python 返回 chunk_id 和分数，Go 再回查 MySQL 取完整知识内容。
	if err := c.post(ctx, "/vector/search", request, &response); err != nil {
		return VectorSearchResponse{}, err
	}
	return response, nil
}

func (c *Client) post(ctx context.Context, path string, request any, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal ai request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+path,
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("build ai request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	// 这里沿用上游请求 ctx；用户请求取消时，未完成的 AI 调用也会被取消。
	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("call ai service: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return fmt.Errorf("ai service returned status %d", httpResponse.StatusCode)
	}

	if err := json.NewDecoder(httpResponse.Body).Decode(&response); err != nil {
		return fmt.Errorf("decode ai response: %w", err)
	}
	return nil
}
