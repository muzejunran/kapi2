# 在线AI助手服务系统

基于Go语言实现的在线AI助手服务系统，支持Skill体系、Memory管理、流式响应等功能。

## 系统特性

### 核心功能
1. **三层Skill体系**
   - built-in：平台内置技能（如web_search）
   - org：业务级技能（如add_bill、query_bills、budget_advisor）
   - user：用户个性化技能

2. **Memory系统**
   - profile：用户长期画像
   - preferences：交互偏好
   - facts：关键事实
   - recent_summary：最近对话摘要

3. **流式响应**
   - 使用SSE协议
   - 首token延迟P95 ≤ 2s
   - 整轮响应P95 ≤ 5s

4. **页面感知**
   - 根据页面动态选择技能
   - 支持技能裁剪

5. **并发支持**
   - 30-100路并发会话
   - 限流保护

## 快速开始

### 1. 环境要求
- Go 1.21+
- Redis
- Docker (可选)

### 2. 安装依赖
```bash
go mod download
```

### 3. 启动Redis

**方式A：使用Docker（推荐）**
```bash
docker run -d --name redis-assistant -p 6379:6379 redis:7-alpine
```

**方式B：使用本地Redis**
```bash
redis-server
```

### 4. 启动服务

**方式A：使用启动脚本**
```bash
./start-simple.sh
```

**方式B：直接运行**
```bash
go run cmd/server/main.go
```

### 5. 访问Web客户端
打开浏览器访问：http://localhost:8080/web-client/index.html

## API文档

### 会话管理
- `POST /sessions` - 创建会话
- `GET /sessions/{id}` - 获取会话
- `DELETE /sessions/{id}` - 删除会话

### 消息处理
- `POST /sessions/{id}/messages` - 发送消息
- `POST /sessions/{id}/stream` - 流式消息（SSE）

### Memory管理
- `GET /memory?user_id={id}` - 获取用户记忆
- `POST /memory?user_id={id}` - 更新用户记忆

### 健康检查
- `GET /health` - 服务健康状态

## Skill开发

### Skill定义格式
```json
{
  "id": "skill_id",
  "name": "skill_name",
  "description": "技能描述",
  "type": "org|builtin|user",
  "version": "1.0.0",
  "pages": ["支持的页面"],
  "schema": {
    "input": { ... },
    "output": { ... }
  },
  "metadata": {
    "timeout": 5000,
    "retry_count": 2
  }
}
```

### 示例Skill
1. **add_bill** - 自然语言转结构化账单
2. **query_bills** - 聚合查询账单
3. **budget_advisor** - 预算建议

## 配置

### 环境变量
```bash
# 服务器配置
SERVER_PORT=8080
REDIS_ADDR=localhost:6379

# LLM配置
LLM_ENDPOINT=https://api.openai.com/v1/chat/completions
LLM_API_KEY=your-api-key
MODEL_NAME=gpt-3.5-turbo
MAX_TOKENS=1000
TEMPERATURE=0.7

# 性能配置
TOKEN_BUDGET=1500
SKILL_TIMEOUT=5s
RATE_LIMIT=100
```

## 系统架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │────│   Agent Core    │────│   Skill Registry│
│   (SSE)        │    │                 │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Memory Service│    │   Streamer     │    │   Skill Exec   │
│                 │    │   (SSE)        │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────────────────────────────────────────────┐
│                    Redis Storage                       │
└─────────────────────────────────────────────────────────┘
```

## 使用场景

### 场景1：添加账单
- 页面：记一笔
- 用户说："昨天星巴克38元"
- 系统调用add_bill技能
- 返回结构化账单确认

### 场景2：查询账单
- 页面：账单详情
- 用户问："这个月外卖花了多少？"
- 系统调用query_bills技能
- 返回聚合数据和图表

### 场景3：预算建议
- 页面：预算
- 用户说："娱乐预算调到800"
- 系统调用budget_advisor技能
- 返回修改建议和影响分析

## 开发指南

### 添加新Skill
1. 在skills/目录下创建JSON文件
2. 定义输入输出schema
3. 实现执行逻辑
4. 系统自动热加载

### 扩展Memory
1. 在MemoryService中添加新的Memory类型
2. 实现提取逻辑
3. 更新Memory上下文

### 性能优化
- 使用Prompt Caching
- 上下文裁剪
- Skill懒加载
- 模型分级路由

## 监控和观测

### 关键指标
- QPS（每秒请求数）
- P95响应时间
- Token使用量
- Skill调用成功率
- 错误率

### 监控端点
- `/health` - 健康检查
- `/metrics` - Prometheus指标

## 部署

### Docker Compose
```yaml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
  
  api-gateway:
    build: .
    ports:
      - "8080:8080"
    depends_on:
      - redis
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ai-assistant
spec:
  replicas: 3
  selector:
    matchLabels:
      app: ai-assistant
  template:
    metadata:
      labels:
        app: ai-assistant
    spec:
      containers:
      - name: ai-assistant
        image: ai-assistant:latest
        ports:
        - containerPort: 8080
        env:
        - name: REDIS_ADDR
          value: "redis-service:6379"
```

## 故障排查

### 常见问题
1. **连接Redis失败** - 检查Redis服务状态
2. **Skill加载失败** - 检查JSON格式和文件路径
3. **流式响应中断** - 检查客户端连接

### 日志级别
```bash
# 设置日志级别
export LOG_LEVEL=debug
```

## 许可证

MIT License