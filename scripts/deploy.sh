#!/bin/bash
set -e

echo "=========================================="
echo "  部署到 bin 目录"
echo "=========================================="

echo ""
echo "[1/4] 复制主程序到 bin..."
# 清理旧文件
rm -f bin/server
rm -rf bin/bin

if [ -f "kapi-server" ]; then
    cp kapi-server bin/
    echo "  ✓ bin/kapi-server"
else
    echo "  ✗ kapi-server 不存在，请先编译"
    exit 1
fi

echo ""
echo "[2/3] 复制配置文件到 bin/skills/financial (仅 .json)..."
rm -rf bin/skills/financial
mkdir -p bin/skills/financial

# 遍历 skills/financial 下的子目录
for skill_dir in skills/financial/*/; do
    if [ -d "$skill_dir" ]; then
        skill_name=$(basename "$skill_dir")
        mkdir -p "bin/skills/financial/$skill_name"
        # 只复制 .json 配置文件
        if [ -f "${skill_dir}${skill_name}.json" ]; then
            cp "${skill_dir}${skill_name}.json" "bin/skills/financial/$skill_name/"
        fi
    fi
done
echo "  ✓ bin/skills/financial/"

echo ""
echo "[3/4] 复制 web-client 到 bin..."
if [ -d "web-client" ]; then
    rm -rf bin/web-client
    cp -r web-client bin/
    echo "  ✓ bin/web-client/"
else
    echo "  ⚠ web-client 目录不存在"
fi

# 遍历 skills/financial 下的子目录
for skill_dir in skills/financial/*/; do
    if [ -d "$skill_dir" ]; then
        skill_name=$(basename "$skill_dir")
        mkdir -p "bin/skills/financial/$skill_name"
        # 只复制 .json 配置文件
        if [ -f "${skill_dir}${skill_name}.json" ]; then
            cp "${skill_dir}${skill_name}.json" "bin/skills/financial/$skill_name/"
        fi
    fi
done
echo "  ✓ bin/skills/financial/"

echo ""
echo "[4/4] 部署文件结构："
find bin -type f | sort

echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
echo ""
echo "运行方式:"
echo "  cd bin && ./kapi-server"
echo ""
echo "目录结构:"
echo "  bin/"
echo "  ├── kapi-server                  # 主程序"
echo "  └── skills/"
echo "      ├── *.so                # 插件 .so 文件"
echo "      └── financial/          # 配置目录 (仅 .json)"
echo "          ├── add_bill/"
echo "          │   └── add_bill.json"
echo "          ├── query_bills/"
echo "          │   └── query_bills.json"
echo "          └── budget_advisor/"
echo "              └── budget_advisor.json"
echo ""
