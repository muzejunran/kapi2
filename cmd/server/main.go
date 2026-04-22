package main

import (
	"context"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ai-assistant-service/internal/agent"
	"ai-assistant-service/internal/api"
	"ai-assistant-service/internal/config"
	"ai-assistant-service/internal/llm"
	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/plugin"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/session"
	"ai-assistant-service/internal/skill"
	"ai-assistant-service/internal/storage"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		stdlog.Fatalf("Failed to load config: %v", err)
	}

	// 检测当前工作目录，如果是 bin/ 目录则使用生产模式配置
	wd, err := os.Getwd()
	if err == nil {
		if filepath.Base(wd) == "bin" {
			// 生产模式：从 bin 目录启动
			cfg.SkillsDir = ""
			cfg.SkillsBinDir = "skills"
			stdlog.Println("Running in production mode (bin directory)")
		} else {
			// 开发模式：从项目根目录启动
			// 使用配置文件中的默认值或环境变量
			stdlog.Println("Running in development mode (project root)")
		}
	}

	// Setup logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)

	// Initialize storage (Redis)
	redisStorage := storage.NewRedisStorage(cfg.RedisAddr)

	// Initialize session manager
	sessionManager := session.NewManager(cfg)

	// Initialize conversation repository (如果MySQL DSN为空，则不使用数据库)
	var convRepo *repository.ConversationRepository
	var billRepo *repository.BillRepository

	if cfg.MySQLDSN != "" {
		convRepo, err = repository.NewConversationRepository(cfg.MySQLDSN)
		if err != nil {
			stdlog.Fatalf("Failed to create conversation repository: %v", err)
		}
		defer convRepo.Close()

		billRepo, err = repository.NewBillRepository(cfg.MySQLDSN)
		if err != nil {
			stdlog.Fatalf("Failed to create bill repository: %v", err)
		}
		defer billRepo.Close()

		logger.Info("MySQL repositories initialized")
	} else {
		logger.Info("MySQL DSN not configured, using in-memory storage only")
		convRepo = nil
		billRepo = nil
	}

	// Initialize memory service
	memoryService := memory.NewMemoryService(redisStorage, cfg.TokenBudget)

	// Set database callbacks for memory service
	memoryService.SetDatabaseCallbacks(
		func(userID string, limit int) ([]memory.ConversationTurn, error) {
			// Load from MySQL
			if convRepo == nil {
				return []memory.ConversationTurn{}, nil
			}
			convs, dbErr := convRepo.GetRecentMessages(userID, limit)
			if dbErr != nil {
				return nil, dbErr
			}

			// Convert to ConversationTurn pairs
			turns := make([]memory.ConversationTurn, 0)
			for i := 0; i < len(convs); i += 2 {
				turn := memory.ConversationTurn{}
				if i < len(convs) && convs[i].Role == "user" {
					turn.UserMessage = convs[i].Content
				}
				if i+1 < len(convs) && convs[i+1].Role == "assistant" {
					turn.AssistantMessage = convs[i+1].Content
				}
				if turn.UserMessage != "" || turn.AssistantMessage != "" {
					turns = append(turns, turn)
				}
			}
			return turns, nil
		},
		func(userID string, turn memory.ConversationTurn) error {
			// Save to MySQL
			if convRepo == nil {
				return nil
			}
			now := time.Now()
			if turn.UserMessage != "" {
				if err := convRepo.AddMessage(&model.Conversation{
					UserID:    userID,
					Role:      "user",
					Content:   turn.UserMessage,
					CreatedAt: now.Format(time.RFC3339),
				}); err != nil {
					return err
				}
			}
			if turn.AssistantMessage != "" {
				if err := convRepo.AddMessage(&model.Conversation{
					UserID:    userID,
					Role:      "assistant",
					Content:   turn.AssistantMessage,
					CreatedAt: now.Format(time.RFC3339),
				}); err != nil {
					return err
				}
			}
			return nil
		},
	)
	// Initialize tool registry
	toolRegistry := skill.NewToolRegistry()
	skillRegistry := skill.NewSkillRegistry()

	// ========== 使用插件加载器加载 Skills ==========
	// 创建 LLM 客户端（如果插件需要调用 LLM）
	llmClient := llm.NewClient(cfg.LLMServicePort)

	// 创建插件依赖
	pluginDeps := &plugin.Dependencies{
		LLMService: plugin.NewLLMServiceAdapter(llmClient),
		BillRepo:   plugin.NewBillRepoAdapter(billRepo),
	}

	// 创建插件加载器
	pluginLoader := plugin.NewLoader(cfg.SkillsDir, cfg.SkillsBinDir, pluginDeps, logger)

	// 设置热加载回调
	pluginLoader.OnReload = func(skillID string, skill plugin.Skill) {
		// 适配插件 skill
		adapter := plugin.NewPluginSkillAdapter(skill)
		// 重新注册到 skillRegistry
		if err := skillRegistry.RegisterSkill(adapter); err != nil {
			logger.Warnf("Failed to re-register plugin skill %s: %v", adapter.GetID(), err)
		} else {
			logger.Infof("✓ Re-registered plugin skill: %s", adapter.GetID())
			// 注册工具
			for _, tool := range adapter.GetTools() {
				toolRegistry.RegisterTool(tool)
			}
		}
	}

	// 加载所有插件
	if err := pluginLoader.LoadAll(); err != nil {
		logger.Warnf("Failed to load some plugins: %v", err)
	}

	// 将插件适配后注册到现有的 skillRegistry
	for _, pluginSkill := range pluginLoader.GetAllSkills() {
		adapter := plugin.NewPluginSkillAdapter(pluginSkill)
		if err := skillRegistry.RegisterSkill(adapter); err != nil {
			logger.Warnf("Failed to register plugin skill %s: %v", adapter.GetID(), err)
		} else {
			logger.Infof("✓ Registered plugin skill: %s", adapter.GetID())

			// 注册工具
			for _, tool := range adapter.GetTools() {
				if err := toolRegistry.RegisterTool(tool); err != nil {
					logger.Warnf("Failed to register tool %s: %v", tool.ID, err)
				}
			}
		}
	}

	// 启动文件监听（热更新）
	if err := pluginLoader.StartWatcher(); err != nil {
		logger.Warnf("Failed to start file watcher: %v", err)
	}

	// Initialize tool executor
	toolExecutor := skill.NewToolExecutor(cfg.SkillTimeout)

	// Initialize agent
	agentConfig := agent.AgentConfig{
		LLMEndpoint:   cfg.LLMEndpoint,
		LLMApiKey:     cfg.LLMApiKey,
		ModelName:     cfg.ModelName,
		MaxTokens:     cfg.MaxTokens,
		Temperature:   cfg.Temperature,
		StreamEnabled: true,
		TokenBudget:   cfg.TokenBudget,
		SkillTimeout:  cfg.SkillTimeout,
	}

	aiAgent := agent.NewAgent(agentConfig, skillRegistry, toolRegistry, toolExecutor, memoryService, sessionManager, nil)

	// Initialize API handler
	apiHandler := api.NewAPIHandler(cfg, aiAgent, sessionManager, memoryService)

	// Setup router
	router := mux.NewRouter()

	// Register API routes
	apiHandler.RegisterRoutes(router)

	// Serve static files
	router.PathPrefix("/web-client/").Handler(http.StripPrefix("/web-client/", http.FileServer(http.Dir("web-client"))))

	// Add middleware
	router.Use(loggingMiddleware)
	router.Use(corsMiddleware)
	router.Use(rateLimitMiddleware)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  600 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  600 * time.Second,
	}

	// Start server
	go func() {
		logger.Infof("Server starting on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("Server shutdown error: %v", err)
	}

	logger.Info("Server stopped")
}

// wireSkillHandlers wires skill handlers to the registry

// Middleware functions
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		logrus.WithFields(logrus.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"remote_addr": r.RemoteAddr,
			"duration":    time.Since(start),
			"user_agent":  r.UserAgent(),
		}).Info("HTTP request")
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func rateLimitMiddleware(next http.Handler) http.Handler {
	// Simple rate limiting implementation
	requests := make(map[string][]time.Time)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		now := time.Now()

		// Clean old requests
		if times, exists := requests[ip]; exists {
			cleanTimes := []time.Time{}
			for _, t := range times {
				if now.Sub(t) < time.Minute {
					cleanTimes = append(cleanTimes, t)
				}
			}
			requests[ip] = cleanTimes
		}

		// Check rate limit
		if len(requests[ip]) > 100 { // 100 requests per minute
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Add current request
		requests[ip] = append(requests[ip], now)

		next.ServeHTTP(w, r)
	})
}
