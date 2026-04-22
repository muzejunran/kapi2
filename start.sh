#!/bin/bash

# 启动在线AI助手服务系统

echo "=== 启动在线AI助手服务系统 ==="

# 检查Go环境
if ! command -v go &> /dev/null; then
    echo "错误: 请先安装Go 1.21+"
    exit 1
fi

# 检查Redis
if ! docker ps | grep -q redis; then
    echo "启动Redis..."
    docker run -d --name redis -p 6379:6379 redis:7-alpine
    sleep 3
fi

# 创建必要目录
mkdir -p skills/builtin skills/org skills/user
mkdir -p logs

# 安装依赖
echo "安装依赖..."
go mod download

# 启动服务
echo "启动服务..."
echo "API Gateway: http://localhost:8080"
echo "Web Client: http://localhost:8080/web-client/index.html"
echo "健康检查: http://localhost:8080/health"
echo ""
echo "按 Ctrl+C 停止服务"

# 运行主服务
go run cmd/server/main.go