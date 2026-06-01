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
	RiskLevel       string   `json:"risk_level"`
	TransferToHuman bool     `json:"transfer_to_human"`
	Suggestions     []string `json:"suggestions"`
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
	body, err := json.Marshal(request)
	if err != nil {
		return ReplyResponse{}, fmt.Errorf("marshal ai request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/ai/reply",
		bytes.NewReader(body),
	)
	if err != nil {
		return ReplyResponse{}, fmt.Errorf("build ai request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return ReplyResponse{}, fmt.Errorf("call ai service: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return ReplyResponse{}, fmt.Errorf("ai service returned status %d", httpResponse.StatusCode)
	}

	var response ReplyResponse
	if err := json.NewDecoder(httpResponse.Body).Decode(&response); err != nil {
		return ReplyResponse{}, fmt.Errorf("decode ai response: %w", err)
	}

	return response, nil
}
