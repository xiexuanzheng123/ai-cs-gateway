package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRAGCacheTTL = 5 * time.Minute
const ragSearchCachePrefix = "ai-cs:rag:search:"

type RAGCache interface {
	Get(ctx context.Context, query string, topK int) ([]RAGSearchResult, bool, error)
	Set(ctx context.Context, query string, topK int, matches []RAGSearchResult) error
	Clear(ctx context.Context) error
}

type RedisRAGCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisRAGCache(client *redis.Client, ttl time.Duration) *RedisRAGCache {
	if ttl <= 0 {
		ttl = defaultRAGCacheTTL
	}
	return &RedisRAGCache{
		client: client,
		ttl:    ttl,
	}
}

func (c *RedisRAGCache) Get(ctx context.Context, query string, topK int) ([]RAGSearchResult, bool, error) {
	if c == nil || c.client == nil {
		return nil, false, nil
	}
	value, err := c.client.Get(ctx, ragCacheKey(query, topK)).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var matches []RAGSearchResult
	if err := json.Unmarshal([]byte(value), &matches); err != nil {
		return nil, false, err
	}
	return matches, true, nil
}

func (c *RedisRAGCache) Set(ctx context.Context, query string, topK int, matches []RAGSearchResult) error {
	if c == nil || c.client == nil {
		return nil
	}
	body, err := json.Marshal(matches)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, ragCacheKey(query, topK), body, c.ttl).Err()
}

func (c *RedisRAGCache) Clear(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}

	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, ragSearchCachePrefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if nextCursor == 0 {
			return nil
		}
		cursor = nextCursor
	}
}

func ragCacheKey(query string, topK int) string {
	return fmt.Sprintf("%s%d:%s", ragSearchCachePrefix, topK, normalizeRAGCacheQuery(query))
}

func normalizeRAGCacheQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
}
