#!/bin/bash

# 生成性能测试报告

RESULTS_DIR="$1"
TIMESTAMP="$2"

if [ -z "$RESULTS_DIR" ] || [ -z "$TIMESTAMP" ]; then
    echo "Usage: $0 <results_dir> <timestamp>"
    exit 1
fi

REPORT_FILE="$RESULTS_DIR/performance_report_$TIMESTAMP.md"

echo "# AI助手服务性能测试报告" > "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "## 测试信息" >> "$REPORT_FILE"
echo "- 测试时间: $(date)" >> "$REPORT_FILE"
echo "- 测试环境: 本地开发环境" >> "$REPORT_FILE"
echo "- 服务版本: 1.0.0" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "## 测试结果摘要" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

# 分析健康检查测试
if [ -f "$RESULTS_DIR/health_test_$TIMESTAMP.txt" ]; then
    echo "### 1. 健康检查 (100并发)" >> "$REPORT_FILE"
    echo "\`\`\`" >> "$REPORT_FILE"
    grep -A 5 "Requests/sec" "$RESULTS_DIR/health_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    grep -A 5 "Latency" "$RESULTS_DIR/health_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    echo "\`\`\`" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# 分析创建会话测试
if [ -f "$RESULTS_DIR/create_session_test_$TIMESTAMP.txt" ]; then
    echo "### 2. 创建会话 (50并发)" >> "$REPORT_FILE"
    echo "\`\`\`" >> "$REPORT_FILE"
    grep -A 5 "Requests/sec" "$RESULTS_DIR/create_session_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    grep -A 5 "Latency" "$RESULTS_DIR/create_session_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    echo "\`\`\`" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# 分析发送消息测试
if [ -f "$RESULTS_DIR/send_message_test_$TIMESTAMP.txt" ]; then
    echo "### 3. 发送消息 (30并发)" >> "$REPORT_FILE"
    echo "\`\`\`" >> "$REPORT_FILE"
    grep -A 5 "Requests/sec" "$RESULTS_DIR/send_message_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    grep -A 5 "Latency" "$RESULTS_DIR/send_message_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
    echo "\`\`\`" >> "$REPORT_FILE"
    echo "" >> "$REPORT_FILE"
fi

# 分析并发会话测试
for concurrent in 30 60 100; do
    if [ -f "$RESULTS_DIR/concurrent_${concurrent}_test_$TIMESTAMP.txt" ]; then
        echo "### 4. 并发会话 (${concurrent}路)" >> "$REPORT_FILE"
        echo "\`\`\`" >> "$REPORT_FILE"
        grep -A 5 "Requests/sec" "$RESULTS_DIR/concurrent_${concurrent}_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
        grep -A 5 "Latency" "$RESULTS_DIR/concurrent_${concurrent}_test_$TIMESTAMP.txt" >> "$REPORT_FILE" || echo "数据提取失败"
        echo "\`\`\`" >> "$REPORT_FILE"
        echo "" >> "$REPORT_FILE"
    fi
done

echo "## SLA合规性分析" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "### 目标指标" >> "$REPORT_FILE"
echo "- **首字延迟 P95**: ≤ 2s" >> "$REPORT_FILE"
echo "- **整轮延迟 P95**: ≤ 5s (无工具调用), ≤ 12s (含工具调用)" >> "$REPORT_FILE"
echo "- **并发会话**: 30-100路" >> "$REPORT_FILE"
echo "- **可用性**: ≥ 99.9%" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "### 合规性评估" >> "$REPORT_FILE"
echo "| 指标 | 目标 | 实际 | 状态 |" >> "$REPORT_FILE"
echo "|------|------|------|------|" >> "$REPORT_FILE"
echo "| 并发会话 | 30-100路 | 待评估 | ⏳ |" >> "$REPORT_FILE"
echo "| 延迟 P95 | ≤5s | 待评估 | ⏳ |" >> "$REPORT_FILE"
echo "| 首字延迟 P95 | ≤2s | 待评估 | ⏳ |" >> "$REPORT_FILE"
echo "| 可用性 | ≥99.9% | 待评估 | ⏳ |" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "## 性能分析" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "### 优势" >> "$REPORT_FILE"
echo "- 基础架构设计合理" >> "$REPORT_FILE"
echo "- 支持流式响应" >> "$REPORT_FILE"
echo "- 具备基本监控能力" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "### 需要改进" >> "$REPORT_FILE"
echo "- 需要实现真正的并发会话管理" >> "$REPORT_FILE"
echo "- 需要添加性能SLA监控" >> "$REPORT_FILE"
echo "- 需要实现Prompt Caching" >> "$REPORT_FILE"
echo "- 需要优化上下文管理" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "## 优化建议" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"
echo "1. **实现会话池**: 使用真正的并发会话管理" >> "$REPORT_FILE"
echo "2. **添加缓存**: 实现Prompt Caching减少重复计算" >> "$REPORT_FILE"
echo "3. **优化模型调用**: 实现模型分级路由和连接池" >> "$REPORT_FILE"
echo "4. **添加监控**: 实现详细的性能指标和告警" >> "$REPORT_FILE"
echo "5. **限流优化**: 实现更精细的限流策略" >> "$REPORT_FILE"
echo "" >> "$REPORT_FILE"

echo "---" >> "$REPORT_FILE"
echo "*报告生成时间: $(date)*" >> "$REPORT_FILE"

echo "测试报告已生成: $REPORT_FILE"