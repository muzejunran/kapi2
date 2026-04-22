#!/bin/bash

echo "=== 启动在线AI助手服务系统（简化版） ==="

# 检查Redis连接
echo "检查Redis连接..."
if ! redis-cli ping > /dev/null 2>&1; then
    echo "错误: Redis未运行，请先启动Redis服务"
    echo ""
    echo "启动Redis的方法："
    echo "1. 使用Docker: docker run -d -p 6379:6379 redis:7-alpine"
    echo "2. 使用本地: redis-server"
    exit 1
fi

echo "Redis已就绪"

# 创建必要目录
mkdir -p skills/builtin skills/org skills/user

# 构建服务
echo "构建服务..."
go build -o ai-assistant ./cmd/server/main.go

# 启动服务
echo ""
echo "=========================================="
echo "服务启动成功！"
echo "=========================================="
echo "Web客户端: http://localhost:8080/web-client/index.html"
echo "健康检查:  http://localhost:8080/health"
echo ""
echo "按 Ctrl+C 停止服务"
echo "=========================================="
echo ""

# 运行服务
./ai-assistant