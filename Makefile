.PHONY: build run test clean docker-up docker-down help

# 默认目标
help:
	@echo "AI助手服务系统 - 可用命令："
	@echo "  make build    - 构建服务"
	@echo "  make run      - 启动服务（需要Redis）"
	@echo "  make test     - 运行测试"
	@echo "  make clean    - 清理构建文件"
	@echo "  make docker-up   - 启动Redis容器"
	@echo "  make docker-down - 停止Redis容器"
	@echo "  make dev      - 开发模式（热重载）"

# 构建服务
build:
	@echo "构建AI助手服务..."
	@go build -o ai-assistant ./cmd/server/main.go
	@echo "构建完成: ./ai-assistant"

# 启动服务
run: build
	@echo "启动服务..."
	@./ai-assistant

# 运行测试
test:
	@echo "运行测试..."
	@./test.sh

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -f ai-assistant cmd/server/ai-assistant
	@echo "清理完成"

# 启动Redis容器
docker-up:
	@echo "启动Redis容器..."
	@docker run -d --name redis-assistant -p 6379:6379 redis:7-alpine
	@echo "Redis已启动"

# 停止Redis容器
docker-down:
	@echo "停止Redis容器..."
	@docker stop redis-assistant 2>/dev/null || true
	@docker rm redis-assistant 2>/dev/null || true
	@echo "Redis已停止"

# 开发模式
dev:
	@echo "开发模式 - 使用air进行热重载"
	@which air > /dev/null || (echo "请先安装air: go install github.com/cosmtrek/air@latest" && exit 1)
	@air