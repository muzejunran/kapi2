# 部署指南

## 部署文件结构

### 编译产物

```
kapi2/
├── server              # 主程序二进制
└── bin/skills/         # 编译后的插件
    ├── add_bill.so
    ├── query_bills.so
    └── budget_advisor.so
```

### 部署目录结构

```
deploy/
├── server                    # 主程序 (必须)
└── bin/skills/               # 插件目录 (必须)
    ├── add_bill.so
    ├── query_bills.so
    └── budget_advisor.so
```

### 可选文件（用于热更新）

```
deploy/
└── skills/financial/         # 配置目录 (可选)
    ├── add_bill.json
    ├── query_bills.json
    └── budget_advisor.json
```

---

## 快速部署

### 1. 编译

```bash
./scripts/build_all.sh
```

### 2. 打包部署文件

```bash
./scripts/deploy.sh
```

### 3. 部署到目标服务器

```bash
# 打包
tar -czf kapi2-deploy.tar.gz deploy/

# 传输到服务器
scp kapi2-deploy.tar.gz user@server:/path/to/deploy/

# 在服务器上解压
ssh user@server "cd /path/to/deploy && tar -xzf kapi2-deploy.tar.gz"
```

### 4. 启动服务

```bash
cd deploy/
./server
```

---

## 部署模式

### 模式1: 最小部署（推荐生产环境）

仅部署运行时必需文件，不包含源码和热更新功能。

```
部署内容:
- server
- bin/skills/*.so

启动时需要修改代码中的路径，或通过配置文件指定:
- bin/skills/ 改为绝对路径
- 禁用文件监听
```

### 模式2: 完整部署（包含热更新）

包含配置文件，支持文件监听和热更新。

```
部署内容:
- server
- bin/skills/*.so
- skills/financial/*.json

特点:
- 支持热更新（监听 skills/financial/ 变化）
- 需要 go 编译环境在服务器上
```

---

## 注意事项

1. **Go 版本一致性**: 主程序和插件必须用相同 Go 版本编译

2. **平台限制**: Go Plugin 不支持 Windows，仅支持 Linux/macOS

3. **路径问题**: 
   - 当前代码使用相对路径 `skills/financial` 和 `bin/skills`
   - 部署后可能需要改为绝对路径或通过配置文件指定

4. **文件权限**: 确保 server 可执行权限 `chmod +x server`

---

## 运行时依赖

- 无需 Go 环境（如果使用预编译的 .so 文件）
- 无需源码文件（.go 文件仅用于开发时编译）
