#!/bin/bash

# AI助手服务性能压测脚本
# 使用 wrk 进行HTTP性能测试

BASE_URL="http://localhost:8080"
RESULTS_DIR="test_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 创建结果目录
mkdir -p "$RESULTS_DIR"

echo "=== AI助手服务性能压测 ==="
echo "开始时间: $(date)"
echo "目标地址: $BASE_URL"
echo ""

# 检查服务是否可用
echo "检查服务状态..."
if ! curl -s "$BASE_URL/health" > /dev/null; then
    echo "错误: 服务未运行，请先启动服务"
    exit 1
fi
echo "服务状态: 正常"
echo ""

# 压测场景配置
SCENARIOS=(
    "健康检查:health:GET:30:10s"
    "创建会话:sessions:POST:30:10s"
    "获取会话:sessions/test_123:GET:30:10s"
    "发送消息:sessions/test_123/messages:POST:30:10s"
    "流式消息:sessions/test_123/stream:POST:30:10s"
)

# 1. 健康检查压测
echo "=== 场景1: 健康检查 ==="
wrk -t4 -c100 -d30s -s scripts/health.lua "$BASE_URL/health" \
    > "$RESULTS_DIR/health_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/health_test_$TIMESTAMP.txt"

# 2. 创建会话压测
echo "=== 场景2: 创建会话 ==="
wrk -t4 -c50 -d30s -s scripts/create_session.lua "$BASE_URL/sessions" \
    > "$RESULTS_DIR/create_session_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/create_session_test_$TIMESTAMP.txt"

# 3. 发送消息压测
echo "=== 场景3: 发送消息 ==="
wrk -t4 -c30 -d30s -s scripts/send_message.lua "$BASE_URL/sessions/test_123/messages" \
    > "$RESULTS_DIR/send_message_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/send_message_test_$TIMESTAMP.txt"

# 4. 并发会话压测
echo "=== 场景4: 并发会话 (30路) ==="
wrk -t4 -c30 -d30s -s scripts/concurrent_sessions.lua "$BASE_URL/sessions" \
    > "$RESULTS_DIR/concurrent_30_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/concurrent_30_test_$TIMESTAMP.txt"

echo "=== 场景5: 并发会话 (60路) ==="
wrk -t4 -c60 -d30s -s scripts/concurrent_sessions.lua "$BASE_URL/sessions" \
    > "$RESULTS_DIR/concurrent_60_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/concurrent_60_test_$TIMESTAMP.txt"

echo "=== 场景6: 并发会话 (100路) ==="
wrk -t4 -c100 -d30s -s scripts/concurrent_sessions.lua "$BASE_URL/sessions" \
    > "$RESULTS_DIR/concurrent_100_test_$TIMESTAMP.txt" 2>&1
echo "结果保存至: $RESULTS_DIR/concurrent_100_test_$TIMESTAMP.txt"

# 生成测试报告
echo ""
echo "=== 生成测试报告 ==="
./scripts/generate_report.sh "$RESULTS_DIR" "$TIMESTAMP"

echo ""
echo "=== 压测完成 ==="
echo "结束时间: $(date)"
echo "所有结果保存在: $RESULTS_DIR/"