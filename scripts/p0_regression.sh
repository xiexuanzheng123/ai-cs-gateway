#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

post_chat() {
  local message_type="$1"
  local message="$2"
  local message_id="m_$(date +%s%N)"
  curl -sS -X POST "$BASE_URL/api/customer-service/chat" \
    -H 'Content-Type: application/json' \
    -d "{
      \"conversation_id\": \"c_p0\",
      \"user_id\": \"u_p0\",
      \"message_id\": \"$message_id\",
      \"message_type\": \"$message_type\",
      \"message\": \"$message\",
      \"channel\": \"h5\",
      \"metadata\": {
        \"platform\": \"p0\"
      }
    }"
}

expect_contains() {
  local name="$1"
  local payload="$2"
  local expected="$3"
  if [[ "$payload" != *"$expected"* ]]; then
    echo "FAIL $name: expected response to contain $expected"
    echo "$payload"
    exit 1
  fi
  echo "PASS $name"
}

greeting="$(post_chat text '你好')"
expect_contains "greeting" "$greeting" '"response_type":"answer"'

refund="$(post_chat text '我要退款')"
expect_contains "refund_handoff" "$refund" '"required":true'

faq="$(post_chat text '密码错误次数过多')"
expect_contains "faq" "$faq" '"intent":"faq"'

image="$(post_chat image '[图片]')"
expect_contains "image_guide" "$image" '"response_type":"guide"'

audio="$(post_chat audio '[语音]')"
expect_contains "audio_guide" "$audio" '"response_type":"guide"'

invalid="$(curl -sS -X POST "$BASE_URL/api/customer-service/chat" -H 'Content-Type: application/json' -d '{}')"
expect_contains "invalid_request" "$invalid" '"code":"invalid_chat_request"'
