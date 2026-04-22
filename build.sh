#!/bin/bash
set -e

echo "=========================================="
echo "  AI Assistant Service - 完整编译"
echo "=========================================="

# 1. 编译所有插件
echo ""
echo "[1/2] 编译插件..."
mkdir -p bin/skills

PLUGINS_DIR="skills/financial"

# 遍历所有子目录并编译
for skill_dir in "$PLUGINS_DIR"/*/; do
    if [ -d "$skill_dir" ]; then
        # 查找子目录中的 .go 文件
        go_file=$(find "$skill_dir" -name "*.go" -type f | head -1)
        if [ -z "$go_file" ]; then
            echo "⚠ Warning: No .go file found in $skill_dir, skipping..."
            continue
        fi

        # 提取 skill 目录名
        skill_name=$(basename "$skill_dir")
        json_file="${skill_dir}${skill_name}.json"

        # 检查对应的 JSON 配置文件是否存在
        if [ ! -f "$json_file" ]; then
            echo "⚠ Warning: No JSON config found in $skill_dir, skipping..."
            continue
        fi

        # 从 JSON 中提取 skill ID
        skill_id=$(grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' "$json_file" | sed 's/.*"\([^"]*\)".*/\1/')
        if [ -z "$skill_id" ] || [ "$skill_id" = "null" ]; then
            # Fallback: 使用目录名作为 skill ID
            skill_id="$skill_name"
        fi

        so_file="bin/skills/${skill_id}.so"

        echo "Building: $skill_id"
        echo "  Source: $go_file"
        echo "  Config: $json_file"
        echo "  Output: $so_file"

        # 编译插件
        if go build -buildmode=plugin -o "$so_file" "$go_file"; then
            echo "  ✓ Built: $so_file"
        else
            echo "  ✗ Build failed for $skill_id"
        fi
        echo ""
    fi
done

echo "已生成的插件:"
ls -lh bin/skills/*.so 2>/dev/null || echo "  (无插件文件)"
echo ""

# 2. 编译主程序
echo "[2/3] 编译主程序..."
if go build -o bin/kapi-server ./cmd/server; then
    echo "✓ 主程序编译完成"
    ls -lh bin/kapi-server
else
    echo "✗ 主程序编译失败"
    exit 1
fi

# 3. 复制 web-client
echo ""
echo "[3/3] 复制 web-client..."
if [ -d "web-client" ]; then
    rm -rf bin/web-client
    cp -r web-client bin/
    echo "✓ web-client 已复制到 bin/web-client/"
else
    echo "⚠ web-client 目录不存在"
fi

echo ""
echo "=========================================="
echo "  编译完成！"
echo "=========================================="
echo ""
echo "运行方式:"
echo "  cd bin && ./kapi-server"
echo ""
echo "目录结构:"
echo "  bin/"
echo "  ├── kapi-server"
echo "  ├── skills/"
echo "  └── web-client/"
echo ""
