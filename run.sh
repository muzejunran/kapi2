#!/bin/bash

echo "=== 启动在线AI助手服务系统 ==="

# 检查Docker是否运行
if ! docker info > /dev/null 2>&1; then
    echo "错误: Docker未运行，请先启动Docker"
    exit 1
fi

# 检查Redis容器是否运行
if ! docker ps | grep -q redis; then
    echo "启动Redis..."
    docker run -d --name redis-assistant -p 6379:6379 redis:7-alpine
    sleep 2
else
    echo "Redis已在运行"
fi

# 创建必要目录
mkdir -p skills/builtin skills/org skills/user
mkdir -p logs

# 等待Redis就绪
echo "等待Redis启动..."
for i in {1..10}; do
    if docker exec redis-assistant redis-cli ping > /dev/null 2>&1; then
        echo "Redis已就绪"
        break
    fi
    sleep 1
done

# 构建服务
echo "构建服务..."
go build -o cmd/server/ai-assistant ./cmd/server/main.go

# 启动服务
echo ""
echo "=========================================="
echo "服务启动成功！"
echo "=========================================="
echo "API Gateway: http://localhost:8080"
echo "Web Client:  http://localhost:8080/web-client/index.html"
echo "健康检查:   http://localhost:8080/health"
echo ""
echo "按 Ctrl+C 停止服务"
echo "=========================================="

# 运行服务
./cmd/server/ai-assistant