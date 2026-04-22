#!/bin/bash

# 编译单个 skill 插件
# 用法: ./scripts/build_plugin.sh skills/financial/add_bill

set -e

SKILL_DIR=$1
if [ -z "$SKILL_DIR" ]; then
    echo "Usage: $0 <skill_dir>"
    echo "Example: $0 skills/financial/add_bill"
    exit 1
fi

# 必须是目录
if [ ! -d "$SKILL_DIR" ]; then
    echo "Error: $SKILL_DIR is not a valid directory"
    exit 1
fi

# 查找子目录中的 .go 文件
GO_FILE=$(find "$SKILL_DIR" -name "*.go" -type f | head -1)
if [ -z "$GO_FILE" ]; then
    echo "Error: No .go file found in $SKILL_DIR"
    exit 1
fi

# 提取目录名作为 skill 名称
SKILL_NAME=$(basename "$SKILL_DIR")
JSON_FILE="${SKILL_DIR}/${SKILL_NAME}.json"

if [ ! -f "$JSON_FILE" ]; then
    echo "Error: No matching JSON config found: $JSON_FILE"
    exit 1
fi

# 读取 skill ID
# 使用 grep + sed 提取 JSON 中的 id 字段
SKILL_ID=$(grep -o '"id"[[:space:]]*:[[:space:]]*"[^"]*"' "$JSON_FILE" | sed 's/.*"\([^"]*\)".*/\1/')
if [ -z "$SKILL_ID" ] || [ "$SKILL_ID" = "null" ]; then
    echo "Error: Failed to read skill ID from config"
    echo "  Trying to use filename as skill ID..."
    # Fallback: 使用 .go 文件名（去掉 .go 后缀）
    SKILL_ID=$(basename "$GO_FILE" .go)
fi

# 输出目录
BIN_DIR="bin/skills"
mkdir -p "$BIN_DIR"

# 输出文件
SO_FILE="$BIN_DIR/${SKILL_ID}.so"

echo "Building skill: $SKILL_ID"
echo "Source: $GO_FILE"
echo "Config: $JSON_FILE"
echo "Output: $SO_FILE"

# 编译插件
# -buildmode=plugin: 编译为插件
# -gcflags="all=-N -l": 禁用优化，便于调试（可选）
go build -buildmode=plugin -o "$SO_FILE" "$GO_FILE"

if [ $? -eq 0 ]; then
    echo "✓ Built: $SO_FILE"
    ls -lh "$SO_FILE"
else
    echo "✗ Build failed"
    exit 1
fi
