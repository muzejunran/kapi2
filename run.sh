#!/bin/bash
set -e

# ── 配置（按需修改） ────────────────────────────────────────────────────────────
SKILL_SERVER_PORT=${SKILL_SERVER_PORT:-8090}
SERVER_PORT=${SERVER_PORT:-8080}
REDIS_ADDR=${REDIS_ADDR:-127.0.0.1:6379}
MYSQL_DSN=${MYSQL_DSN:-"root:11111111@tcp(127.0.0.1:3306)/kapi?charset=utf8mb4&parseTime=true"}
LLM_ENDPOINT=${LLM_ENDPOINT:-"https://open.bigmodel.cn/api/paas/v4/chat/completions"}
LLM_API_KEY=${LLM_API_KEY:-""}
MODEL_NAME=${MODEL_NAME:-"glm-4-flash"}
MAX_TOKENS=${MAX_TOKENS:-4000}
TEMPERATURE=${TEMPERATURE:-0.7}

# ── 编译（如果二进制不存在或源码更新） ──────────────────────────────────────────
if [ ! -f bin/kapi-server ] || [ ! -f bin/skill-server ]; then
    echo "二进制不存在，先编译..."
    ./build.sh
fi

# ── 检查 Redis（本地，非 Docker） ─────────────────────────────────────────────
if ! redis-cli -h 127.0.0.1 -p 6379 ping > /dev/null 2>&1; then
    echo "⚠  Redis 未运行，请先启动:"
    echo "   brew services start redis   # macOS"
    echo "   sudo systemctl start redis  # Linux"
    exit 1
fi
echo "✓ Redis 已就绪"

# ── 清理旧进程 ────────────────────────────────────────────────────────────────
cleanup() {
    echo ""
    echo "停止服务..."
    [ -n "$SKILL_PID" ] && kill "$SKILL_PID" 2>/dev/null || true
    [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null || true
    echo "已停止"
    exit 0
}
trap cleanup SIGINT SIGTERM

# ── 启动 skill-server（后台） ────────────────────────────────────────────────
echo "启动 skill-server :${SKILL_SERVER_PORT} ..."
SKILL_SERVER_PORT="$SKILL_SERVER_PORT" \
MYSQL_DSN="$MYSQL_DSN" \
    bin/skill-server &
SKILL_PID=$!

# 等待 skill-server 就绪
for i in {1..10}; do
    if curl -sf "http://127.0.0.1:${SKILL_SERVER_PORT}/health" > /dev/null 2>&1; then
        echo "✓ skill-server 已就绪"
        break
    fi
    sleep 0.5
    if [ "$i" -eq 10 ]; then
        echo "✗ skill-server 启动超时"
        kill "$SKILL_PID" 2>/dev/null || true
        exit 1
    fi
done

# ── 启动 kapi-server（前台） ─────────────────────────────────────────────────
echo "启动 kapi-server :${SERVER_PORT} ..."
echo ""
echo "=========================================="
echo "  Web Client:  http://localhost:${SERVER_PORT}/web-client/"
echo "  健康检查:    http://localhost:${SERVER_PORT}/health"
echo "  skill-server: http://localhost:${SKILL_SERVER_PORT}/health"
echo "=========================================="
echo "按 Ctrl+C 停止所有服务"
echo ""

SERVER_PORT="$SERVER_PORT" \
REDIS_ADDR="$REDIS_ADDR" \
MYSQL_DSN="$MYSQL_DSN" \
LLM_ENDPOINT="$LLM_ENDPOINT" \
LLM_API_KEY="$LLM_API_KEY" \
MODEL_NAME="$MODEL_NAME" \
MAX_TOKENS="$MAX_TOKENS" \
TEMPERATURE="$TEMPERATURE" \
SKILL_SERVER_URL="http://127.0.0.1:${SKILL_SERVER_PORT}" \
    bin/kapi-server &
SERVER_PID=$!

wait "$SERVER_PID"
