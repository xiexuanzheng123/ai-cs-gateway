# AI Customer Service Gateway

Go HTTP gateway for H5 and future native client robot entry.

## Responsibilities

- Expose the stable chat API.
- Apply hard handoff rules before AI calls.
- Orchestrate rules, short-term memory, RAG, LLM, response validation, and handoff.
- Persist sessions/messages/trace logs to MySQL and cache memory/RAG results in Redis.
- Call Python AI service for embedding, vector search, BM25 search, rerank, and LLM reply.

## Run

推荐从 workspace 根目录按统一顺序启动，见 `../README.md`。

单独启动 gateway 前，需要 devops 里的 MySQL、Redis、Milvus、OpenSearch，以及 Python AI Service 已运行：

```bash
cd ../ai-cs-devops
docker compose up -d mysql redis milvus opensearch
```

再启动 gateway（会读取项目根目录 `.env`）：

```bash
cp .env.example .env   # 首次
go mod tidy
go test ./...
go run ./cmd/server
```

## Environment

复制 `.env.example` 为 `.env` 即可；`go run` 会通过 `godotenv` 自动加载。

```env
HTTP_ADDR=:8080
AI_SERVICE_BASE_URL=http://localhost:8000
MYSQL_DSN=ai_cs_github:<mysql-password>@tcp(127.0.0.1:3306)/ai_customer_service_github?parseTime=true&charset=utf8mb4&loc=Local
REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=1
```

未设置 `MYSQL_DSN` 时仍可启动，但不会持久化会话（使用内存 NoopStore）。

## API

### Stable chat contract

```bash
curl http://localhost:8080/api/customer-service/health
```

```bash
curl -X POST http://localhost:8080/api/customer-service/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "conversation_id": "c_local",
    "user_id": "demo-user-001",
    "message_id": "m_local",
    "message_type": "text",
    "message": "密码错误太多怎么办",
    "channel": "h5",
    "metadata": {
      "platform": "h5"
    }
  }'
```

```bash
curl -X POST http://localhost:8080/api/customer-service/handoff \
  -H 'Content-Type: application/json' \
  -d '{
    "conversation_id": "session-local",
    "message_id": "message-local",
    "user_id": "demo-user-001",
    "reason": "user_requested",
    "channel": "h5"
  }'
```

```bash
curl http://localhost:8080/api/customer-service/admin/rules
```

```bash
curl http://localhost:8080/api/customer-service/admin/dashboard
curl http://localhost:8080/api/customer-service/admin/flags
curl http://localhost:8080/api/customer-service/admin/trace-logs
curl http://localhost:8080/api/customer-service/admin/quality-stats
```

## RAG Flow

1. Strong rules and handoff rules run first.
2. Redis short-term memory is read.
3. Gateway calls AI Service for OpenSearch BM25 recall and Milvus vector recall.
4. Gateway merges candidates and calls AI Service `/rerank`.
5. Gateway calls AI Service `/v1/ai/reply` with retrieved passages.
6. Response validator checks citation, low-quality reply, and unsafe promise.
7. Trace log records branch, retrieved knowledge, validator result, model, token usage, cost estimate, and latency.

## Knowledge Operations

Admin APIs support direct edit plus version operations:

```bash
curl http://localhost:8080/api/customer-service/admin/knowledge
curl http://localhost:8080/api/customer-service/admin/knowledge/{id}/versions
curl -X POST http://localhost:8080/api/customer-service/admin/knowledge/{id}/publish
curl -X POST http://localhost:8080/api/customer-service/admin/knowledge/{id}/versions/{version_id}/rollback
curl -X POST http://localhost:8080/api/customer-service/admin/knowledge/chunks/sync
```

## P0 Regression

Gateway 启动后执行：

```bash
bash scripts/p0_regression.sh
```

## RAG Regression

不依赖测试同学，可从本地 `cs_knowledge` 自动抽取 200 条 `published` 知识生成回归 case：

- `query_text` = 知识 `question`
- `expected_knowledge_id` = `knowledge_id`
- `should_answer` = `true`

同时会补齐少量负例（如「火星演唱会门票怎么领取」），验证无知识问题不强答。
H5 管理页「RAG 回归集」提供「一键回归」按钮；命令行可用：

```bash
bash scripts/rag_regression.sh
```

分步调用：

```bash
curl -X POST "http://localhost:8080/api/customer-service/admin/rag-eval-cases/seed?target_total=200"
curl -X POST http://localhost:8080/api/customer-service/admin/rag-eval-cases/run
curl http://localhost:8080/api/customer-service/admin/rag-eval-runs
```

运行结果会返回 Top1 / Top3 命中率、疑似幻觉数和 major / unsafe 近似指标。
