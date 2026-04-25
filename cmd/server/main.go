package main

import (
	"context"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-assistant-service/internal/agent"
	"ai-assistant-service/internal/api"
	"ai-assistant-service/internal/config"
	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/model"
	"ai-assistant-service/internal/repository"
	"ai-assistant-service/internal/session"
	"ai-assistant-service/internal/storage"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

func main() {
	// Setup structured logger (level + timestamp + file:line) before anything else
	logger.Setup()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		stdlog.Fatalf("Failed to load config: %v", err)
	}

	log := logger.New()
	log.SetOutput(os.Stdout)

	// Initialize storage (Redis)
	redisStorage := storage.NewRedisStorage(cfg.RedisAddr)

	// Initialize session manager
	sessionManager := session.NewManager(cfg)

	// Initialize conversation repository (如果MySQL DSN为空，则不使用数据库)
	var convRepo *repository.ConversationRepository
	var billRepo *repository.BillRepository
	var memRepo *repository.MemoryRepository

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

		memRepo, err = repository.NewMemoryRepository(cfg.MySQLDSN)
		if err != nil {
			log.WithError(err).Warn("Failed to create memory repository, memory won't be persisted to MySQL")
			memRepo = nil
		} else {
			defer memRepo.Close()
		}

		log.Info("MySQL repositories initialized")
	} else {
		log.Info("MySQL DSN not configured, using in-memory storage only")
	}

	// Initialize memory service
	memoryService := memory.NewMemoryService(redisStorage, cfg.TokenBudget)

	// Set database save callback for conversation persistence
	memoryService.SetDatabaseCallbacks(func(userID string, turn memory.ConversationTurn) error {
		if convRepo == nil {
			return nil
		}
		now := time.Now()
		if turn.UserMessage != "" {
			if err := convRepo.AddMessage(&model.Conversation{
				UserID:    userID,
				Role:      "user",
				Content:   turn.UserMessage,
				CreatedAt: now.UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		if turn.AssistantMessage != "" {
			if err := convRepo.AddMessage(&model.Conversation{
				UserID:    userID,
				Role:      "assistant",
				Content:   turn.AssistantMessage,
				CreatedAt: now.UTC().Format(time.RFC3339),
			}); err != nil {
				return err
			}
		}
		return nil
	})

	// Wire MySQL persistence for the Memory struct (load on cold-start, save on every update)
	if memRepo != nil {
		memoryService.SetMemoryPersistence(memRepo.Load, memRepo.Save, memRepo.Delete)
	}

	// Initialize memory determiner for async memory extraction
	memDeterminer := memory.NewMemoryDeterminer(cfg.LLMEndpoint, cfg.LLMApiKey, cfg.ModelName)
	memoryService.SetDeterminer(memDeterminer)
	// Initialize agent (skills loaded via skill-server)
	agentConfig := agent.AgentConfig{
		LLMEndpoint:    cfg.LLMEndpoint,
		LLMApiKey:      cfg.LLMApiKey,
		ModelName:      cfg.ModelName,
		MaxTokens:      cfg.MaxTokens,
		Temperature:    cfg.Temperature,
		StreamEnabled:  true,
		TokenBudget:    cfg.TokenBudget,
		SkillTimeout:   cfg.SkillTimeout,
		SkillServerURL: cfg.SkillServerURL,
	}

	aiAgent := agent.NewAgent(agentConfig, memoryService, sessionManager, nil)

	// Initialize API handler
	apiHandler := api.NewAPIHandler(cfg, aiAgent, sessionManager, memoryService)

	// Setup router
	router := mux.NewRouter()

	// Register API routes
	apiHandler.RegisterRoutes(router)

	// Serve static files
	router.PathPrefix("/web-client/").Handler(http.StripPrefix("/web-client/", http.FileServer(http.Dir("web-client"))))

	// Add middleware (trace first so all downstream logs carry trace_id)
	router.Use(traceMiddleware)
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
		log.Infof("Server starting on port %s", cfg.ServerPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	log.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Errorf("Server shutdown error: %v", err)
	}

	log.Info("Server stopped")
}

// Middleware functions

// traceMiddleware reads X-Trace-ID from the request (or generates one) and injects it into the context.
func traceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = logger.NewID()
		}
		w.Header().Set("X-Trace-ID", traceID)
		ctx := logger.WithTraceID(r.Context(), traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

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
