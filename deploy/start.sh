#!/bin/bash

# 设置环境变量
# SKILLS_DIR: 源码/配置目录（生产环境设为空，不使用）
# SKILLS_BIN_DIR: .so 文件目录
export SKILLS_DIR=""
export SKILLS_BIN_DIR="skills"

echo "Starting server..."
echo "  SKILLS_DIR=$SKILLS_DIR"
echo "  SKILLS_BIN_DIR=$SKILLS_BIN_DIR"
echo ""

./server
