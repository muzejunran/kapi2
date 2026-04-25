#!/bin/bash
set -e

echo "=========================================="
echo "  AI Assistant Service - 本地编译"
echo "=========================================="

mkdir -p bin

# 1. 编译主服务
echo ""
echo "[1/2] 编译 kapi-server..."
if CGO_ENABLED=0 go build -o bin/kapi-server ./cmd/server; then
    echo "✓ kapi-server 编译完成 ($(du -sh bin/kapi-server | cut -f1))"
else
    echo "✗ kapi-server 编译失败"
    exit 1
fi

# 2. 编译 skill-server
echo ""
echo "[2/2] 编译 skill-server..."
if CGO_ENABLED=0 go build -o bin/skill-server ./cmd/skill-server; then
    echo "✓ skill-server 编译完成 ($(du -sh bin/skill-server | cut -f1))"
else
    echo "✗ skill-server 编译失败"
    exit 1
fi

# 3. 复制 web-client
if [ -d "web-client" ]; then
    rm -rf bin/web-client
    cp -r web-client bin/
    echo "✓ web-client 已复制"
fi

echo ""
echo "=========================================="
echo "  编译完成"
echo "=========================================="
echo ""
echo "目录结构:"
echo "  bin/"
echo "  ├── kapi-server    (主服务)"
echo "  ├── skill-server   (工具服务)"
echo "  └── web-client/"
echo ""
echo "启动方式:"
echo "  ./run.sh"
echo ""
