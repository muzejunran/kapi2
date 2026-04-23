# 在线AI助手服务系统

# 1. 复制并配置环境变量
cp .env.example .env
vim .env  # 修改 LLM_API_KEY 等配置

# 2. 启动服务
docker-compose up -d

# 3. 查看日志
docker-compose logs -f kapi-server

# 4. 停止服务
docker-compose down

# 5.前端展示
http://localhost:8080/web-client/