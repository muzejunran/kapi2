# 编译指南

## 快速开始

### 一键编译所有内容

```bash
./scripts/build_all.sh
```

这会自动完成：
1. 编译所有插件（.so 文件）
2. 编译主程序（server 可执行文件）

---

## 详细步骤

### 1. 编译插件

插件位于 `skills/financial/` 目录下，每个 skill 对应一个 `.go` 文件和 `.json` 配置文件。

```bash
# 编译单个插件
./scripts/build_plugin.sh skills/financial/add_bill
./scripts/build_plugin.sh skills/financial/query_bills
./scripts/build_plugin.sh skills/financial/budget_advisor
```

编译后插件会输出到 `bin/skills/` 目录：

```
bin/skills/
├── add_bill.so
├── query_bills.so
└── budget_advisor.so
```

### 2. 编译主程序

```bash
go build -o server ./cmd/server
```

---

## 运行

编译完成后：

```bash
# 启动服务
./server
```

服务会：
1. 自动加载 `bin/skills/` 下的所有插件
2. 启动文件监听，监听 `skills/financial/` 目录的变更
3. 当 `.go` 或 `.json` 文件变更时，自动重新编译并热加载插件

---

## 热更新

修改插件源码后：

```bash
# 方式1: 自动热更新（推荐）
# 修改 skills/financial/add_bill.go
# 服务会自动检测变更并重新编译加载

# 方式2: 手动重新编译
./scripts/build_plugin.sh skills/financial/add_bill
```

---

## 目录结构

```
kapi2/
├── bin/skills/              # 编译后的插件 (.so 文件)
├── cmd/server/main.go       # 主程序入口
├── internal/
│   ├── plugin/              # 插件系统
│   │   ├── skill.go         # 插件接口定义
│   │   ├── adapter.go       # 适配器
│   │   └── loader.go        # 插件加载器 + 热更新
│   └── ...                  # 其他内部包
├── skills/financial/         # 插件源码
│   ├── add_bill.go          # 插件实现
│   ├── add_bill.json        # 插件配置
│   ├── query_bills.go
│   ├── query_bills.json
│   ├── budget_advisor.go
│   └── budget_advisor.json
├── scripts/
│   ├── build_plugin.sh      # 单个插件编译脚本
│   └── build_all.sh         # 完整编译脚本
└── server                   # 编译后的主程序
```

---

## 注意事项

### Go Plugin 限制

- ✅ **支持**: Linux, macOS
- ❌ **不支持**: Windows
- ⚠️ 主程序和插件必须用相同 Go 版本编译

### 检查 Go 版本

```bash
go version
```

### 查看当前编译的 Go 版本

```bash
go env GOVERSION
```
