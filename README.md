# AI Customer Service Gateway

Go HTTP gateway for H5 and future native client robot entry.

## Responsibilities

- Expose the stable chat API.
- Apply hard handoff rules before AI calls.
- Call the Python AI service for normal questions.
- Later: persist sessions and messages to MySQL, cache short context in Redis, and call business tool APIs.

## Run

```bash
go mod tidy
go test ./...
go run ./cmd/server
```

## Environment

```env
HTTP_ADDR=:8080
AI_SERVICE_BASE_URL=http://localhost:8000
```

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
