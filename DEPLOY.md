# 部署说明

## 前置要求

- Go 1.21+
- Redis 服务器
- Docker（可选，用于运行Redis）

## 启动方式

### 方式一：使用Docker运行Redis（推荐）

1. 启动Redis容器：
```bash
docker run -d --name redis-assistant -p 6379:6379 redis:7-alpine
```

2. 启动AI助手服务：
```bash
./run.sh
```

### 方式二：使用本地Redis

1. 启动本地Redis服务：
```bash
redis-server
```

2. 启动AI助手服务：
```bash
go run cmd/server/main.go
```

或者使用编译后的二进制文件：
```bash
go build -o ai-assistant ./cmd/server/main.go
./ai-assistant
```

### 方式三：使用Docker Compose

```bash
docker-compose up -d
```

## 访问服务

- **Web客户端**: http://localhost:8080/web-client/index.html
- **健康检查**: http://localhost:8080/health
- **API文档**: 见 README.md

## 停止服务

- 如果使用Docker：
```bash
docker stop redis-assistant
docker rm redis-assistant
```

- 如果使用本地进程：
```bash
Ctrl+C
```

## 环境变量配置

创建 `.env` 文件或设置环境变量：

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

## 故障排查

### 1. Redis连接失败

检查Redis是否运行：
```bash
redis-cli ping
```

应该返回 `PONG`

### 2. 端口被占用

如果8080端口被占用，修改配置：
```bash
SERVER_PORT=8081 go run cmd/server/main.go
```

### 3. 静态文件404

确保在项目根目录下运行：
```bash
pwd
# 应该显示: /Users/farrelmeng/Workspace/kapi2
```

## 开发模式

热重载开发：
```bash
air  # 需要安装 air: go install github.com/cosmtrek/air@latest
```

或者使用：
```bash
go install github.com/bradfitz/go-toolchain/...@latest
go install golang.org/x/tools/cmd/godoc@latest
```

## 生产部署

### 使用systemd

创建 `/etc/systemd/system/ai-assistant.service`:

```ini
[Unit]
Description=AI Assistant Service
After=network.target redis.service

[Service]
Type=simple
User=www-data
WorkingDirectory=/path/to/ai-assistant-service
ExecStart=/path/to/ai-assistant/server
Restart=always
RestartSec=10
Environment=SERVER_PORT=8080
Environment=REDIS_ADDR=localhost:6379

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl enable ai-assistant
sudo systemctl start ai-assistant
```