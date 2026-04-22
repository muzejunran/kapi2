#!/bin/bash

echo "=== 测试AI助手服务 ==="

BASE_URL="http://localhost:8080"

# 测试1：健康检查
echo "测试1: 健康检查"
curl -s "$BASE_URL/health" | jq .
echo ""

# 测试2：创建会话
echo "测试2: 创建会话"
SESSION_RESPONSE=$(curl -s -X POST "$BASE_URL/sessions" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test_user","page_context":"home"}')
echo $SESSION_RESPONSE | jq .
SESSION_ID=$(echo $SESSION_RESPONSE | jq -r '.session_id')
echo "会话ID: $SESSION_ID"
echo ""

# 测试3：发送消息
echo "测试3: 发送消息"
if [ -n "$SESSION_ID" ]; then
  curl -s -X POST "$BASE_URL/sessions/$SESSION_ID/messages" \
    -H "Content-Type: application/json" \
    -d "{\"message\":\"你好\",\"page_context\":\"home\"}" | jq .
else
  echo "跳过：未获取到会话ID"
fi
echo ""

# 测试4：流式消息
echo "测试4: 流式消息（按Ctrl+C停止）"
if [ -n "$SESSION_ID" ]; then
  curl -s -X POST "$BASE_URL/sessions/$SESSION_ID/stream" \
    -H "Content-Type: application/json" \
    -d "{\"message\":\"昨天星巴克38元\",\"page_context\":\"bills.add\"}" --no-buffer
else
  echo "跳过：未获取到会话ID"
fi
echo ""

echo "=== 测试完成 ==="