# 插件系统文档

## 概述

本项目使用 Go Plugin 实现技能热更新功能。每个技能是一个独立的 Go 插件（.so 文件），修改源码后无需重启服务即可自动重新加载。

## 目录结构

```
skills/
├── financial/
│   ├── add_bill.go           # 源码
│   ├── add_bill.json         # 配置
│   ├── query_bills.go
│   ├── query_bills.json
│   ├── budget_advisor.go
│   └── budget_advisor.json

bin/skills/                   # 编译输出的 .so 文件
├── add_bill.so
├── query_bills.so
└── budget_advisor.so
```

## 开发新技能

### 1. 创建源码文件

```go
package main

import (
    "ai-assistant-service/internal/plugin"
)

// MySkill 技能实现
type MySkill struct {
    deps *plugin.Dependencies
}

// NewSkill 必须导出，主程序会调用这个函数
func NewSkill(deps *plugin.Dependencies) (plugin.Skill, error) {
    return &MySkill{deps: deps}, nil
}

// 实现 plugin.Skill 接口
func (s *MySkill) GetID() string { return "my_skill" }
func (s *MySkill) GetName() string { return "我的技能" }
func (s *MySkill) GetDescription() string { return "技能描述" }
func (s *MySkill) CanHandle(ctx plugin.AgentContext) float64 { ... }
func (s *MySkill) GetTools() []plugin.ToolDef { ... }
func (s *MySkill) Execute(ctx context.Context, toolName string, params map[string]interface{}, contextData map[string]interface{}) (map[string]interface{}, error) { ... }
func (s *MySkill) Cleanup() error { return nil }
```

### 2. 创建配置文件

```json
{
  "id": "my_skill",
  "name": "我的技能",
  "description": "技能描述",
  "version": "1.0.0",
  "enabled": true,
  "supported_pages": ["my_page"],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "my_tool",
        "description": "工具描述",
        "parameters": {
          "type": "object",
          "properties": {...},
          "required": [...],
          "additionalProperties": false
        }
      }
    }
  ],
  "runtime_config": {
    "default_timeout_ms": 5000,
    "error_message": "执行失败"
  },
  "metadata": {
    "author": "your_name",
    "tags": ["tag1", "tag2"]
  }
}
```

### 3. 编译插件

```bash
./scripts/build_plugin.sh skills/financial/add_bill
```

编译成功后会在 `bin/skills/` 目录生成 `.so` 文件。

## 使用依赖

插件可以通过 `Dependencies` 访问以下服务：

| 服务 | 接口 | 用途 |
|------|------|------|
| LLMService | `plugin.LLMService` | 调用 LLM |
| BillRepo | `plugin.BillRepo` | 账单数据访问 |

### 调用 LLM 示例

```go
func (s *MySkill) Execute(ctx context.Context, ...) (map[string]interface{}, error) {
    messages := []plugin.Message{
        {Role: "user", Content: "帮我分析一下..."},
    }

    resp, err := s.deps.LLMService.Chat(ctx, messages)
    if err != nil {
        return nil, err
    }

    return map[string]interface{}{
        "message": resp.Content,
    }, nil
}
```

### 访问数据库示例

```go
func (s *MySkill) Execute(ctx context.Context, ...) (map[string]interface{}, error) {
    bill := &plugin.Bill{
        UserID:   "user123",
        Amount:   100.0,
        Category: "餐饮",
    }

    if err := s.deps.BillRepo.AddBill(bill); err != nil {
        return nil, err
    }

    return map[string]interface{}{
        "success": true,
    }, nil
}
```

## 热更新流程

1. 修改 `skills/xxx/yyy.go` 源码
2. 文件监听器检测到变更
3. 自动重新编译生成新的 `.so` 文件
4. 加载新插件
5. 替换旧插件（无需重启服务）

## 限制

- 只支持 Linux/macOS
- 主程序和插件必须用相同 Go 版本编译
- 插件加载后无法卸载（但可以替换）
- 不能跨平台编译

## 调试

如果插件加载失败，可以：

1. 检查日志中的错误信息
2. 手动运行编译脚本：`./scripts/build_plugin.sh skills/xxx/yyy`
3. 检查 `.so` 文件是否生成：`ls -lh bin/skills/`
4. 使用 `nm` 命令检查符号：`nm bin/skills/yyy.so | grep NewSkill`
