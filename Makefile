.PHONY: build build-server build-skill run clean test help

# ── 默认 ──────────────────────────────────────────────────────────────────────
help:
	@echo "AI助手服务系统 - 可用命令："
	@echo "  make build        - 编译两个服务"
	@echo "  make run          - 本地启动（需要 Redis）"
	@echo "  make test         - 运行测试"
	@echo "  make clean        - 清理构建文件"

# ── 编译 ──────────────────────────────────────────────────────────────────────
build: build-server build-skill
	@if [ -d "web-client" ]; then rm -rf bin/web-client && cp -r web-client bin/; fi
	@echo "✓ 编译完成"

build-server:
	@echo "编译 kapi-server..."
	@mkdir -p bin
	@CGO_ENABLED=0 go build -o bin/kapi-server ./cmd/server
	@echo "✓ kapi-server"

build-skill:
	@echo "编译 skill-server..."
	@mkdir -p bin
	@CGO_ENABLED=0 go build -o bin/skill-server ./cmd/skill-server
	@echo "✓ skill-server"

# ── 本地运行 ──────────────────────────────────────────────────────────────────
run: build
	@./run.sh

# ── 其他 ──────────────────────────────────────────────────────────────────────
test:
	@echo "运行测试..."
	@go test ./...

clean:
	@rm -rf bin/kapi-server bin/skill-server bin/web-client
	@echo "✓ 清理完成"
