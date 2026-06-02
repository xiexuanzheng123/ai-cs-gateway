# AI Customer Service Gateway

Go HTTP gateway for H5 and future native client robot entry.

## Responsibilities

- Expose the stable chat API.
- Apply hard handoff rules before AI calls.
- Call the Python AI service for normal questions.
- Later: persist sessions and messages to MySQL, cache short context in Redis, and call business tool APIs.

## Run

先启动 devops 里的 MySQL（与下方 DSN 账号一致）：

```bash
cd ../ai-cs-devops
docker compose up -d mysql
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
MYSQL_DSN=ai_cs:ai_cs_pass@tcp(127.0.0.1:3306)/ai_customer_service?parseTime=true&charset=utf8mb4&loc=Local
```

未设置 `MYSQL_DSN` 时仍可启动，但不会持久化会话（使用内存 NoopStore）。

## API

```bash
curl http://localhost:8080/healthz
```

```bash
curl -X POST http://localhost:8080/api/chat/send \
  -H 'Content-Type: application/json' \
  -d '{
    "user_id": "demo-user-001",
    "message": "密码错误太多怎么办",
    "source": "h5",
    "metadata": {
      "platform": "h5"
    }
  }'
```
