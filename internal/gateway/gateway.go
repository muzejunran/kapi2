package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ai-assistant-service/internal/auth"
	"ai-assistant-service/internal/loadbalancer"
	"ai-assistant-service/internal/llm"
	"ai-assistant-service/internal/monitoring"
	"ai-assistant-service/internal/session"
	"ai-assistant-service/internal/config"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

type Gateway struct {
	config     *config.Config
	authClient *auth.Client
	llmClient  *llm.Client
	sessionMgr *session.Manager
	loadBalancer *loadbalancer.LoadBalancer
	monitor    *monitoring.Monitor
	logger     *logrus.Logger
}

func NewGateway(cfg *config.Config, monitor *monitoring.Monitor) *Gateway {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	return &Gateway{
		config: cfg,
		authClient: auth.NewClient(cfg.AuthPort),
		llmClient:  llm.NewClient(cfg.LLMServicePort),
		sessionMgr: session.NewManager(cfg),
		loadBalancer: loadbalancer.NewLoadBalancer([]string{cfg.LLMServicePort}),
		monitor:    monitor,
		logger:     logger,
	}
}

func (g *Gateway) GetRouter() *mux.Router {
	r := mux.NewRouter()

	// Middleware
	r.Use(g.loggingMiddleware)
	r.Use(g.corsMiddleware)

	// Public routes
	r.HandleFunc("/health", g.healthCheck).Methods("GET")
	r.HandleFunc("/login", g.login).Methods("POST")
	r.HandleFunc("/register", g.register).Methods("POST")

	// Protected routes (requires authentication)
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(g.authMiddleware)
	protected.HandleFunc("/chat", g.chat).Methods("POST")
	protected.HandleFunc("/session", g.getSession).Methods("GET")
	protected.HandleFunc("/sessions", g.listSessions).Methods("GET")

	return r
}

func (g *Gateway) healthCheck(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Check service health
	status := map[string]string{
		"status": "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version": "1.0.0",
	}

	g.monitor.RecordRequest("health", 200, time.Since(start))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (g *Gateway) login(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		g.monitor.RecordRequest("login", 400, time.Since(start))
		return
	}

	// Authenticate user
	token, err := g.authClient.Authenticate(req.Username, req.Password)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		g.monitor.RecordRequest("login", 401, time.Since(start))
		return
	}

	response := map[string]string{
		"token": token,
		"message": "Login successful",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	g.monitor.RecordRequest("login", 200, time.Since(start))
}

func (g *Gateway) register(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		g.monitor.RecordRequest("register", 400, time.Since(start))
		return
	}

	// Register user
	err := g.authClient.Register(req.Username, req.Password, req.Email)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		g.monitor.RecordRequest("register", 500, time.Since(start))
		return
	}

	response := map[string]string{
		"message": "Registration successful",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	g.monitor.RecordRequest("register", 201, time.Since(start))
}

func (g *Gateway) chat(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get session from context
	sessionID := r.Context().Value("session_id").(string)

	var req struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		g.monitor.RecordRequest("chat", 400, time.Since(start))
		return
	}

	// Get session data
	sessionData, err := g.sessionMgr.GetSession(sessionID)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		g.monitor.RecordRequest("chat", 404, time.Since(start))
		return
	}

	// Update session with new message
	sessionData.Messages = append(sessionData.Messages, map[string]interface{}{
		"role":    "user",
		"content": req.Message,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	// Load balance to LLM service
	llmResponse, err := g.loadBalancer.ForwardToLLM(sessionData.Messages)
	if err != nil {
		http.Error(w, "LLM service unavailable", http.StatusServiceUnavailable)
		g.monitor.RecordRequest("chat", 503, time.Since(start))
		return
	}

	// Update session with AI response
	sessionData.Messages = append(sessionData.Messages, map[string]interface{}{
		"role":    "assistant",
		"content": llmResponse,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	// Save session
	g.sessionMgr.UpdateSession(sessionID, sessionData)

	response := map[string]interface{}{
		"response": llmResponse,
		"session_id": sessionID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
	g.monitor.RecordRequest("chat", 200, time.Since(start))
}

func (g *Gateway) getSession(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get session from context
	sessionID := r.Context().Value("session_id").(string)

	sessionData, err := g.sessionMgr.GetSession(sessionID)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		g.monitor.RecordRequest("getSession", 404, time.Since(start))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionData)
	g.monitor.RecordRequest("getSession", 200, time.Since(start))
}

func (g *Gateway) listSessions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// In a real implementation, this would get user sessions
	sessions, err := g.sessionMgr.ListSessions()
	if err != nil {
		http.Error(w, "Failed to list sessions", http.StatusInternalServerError)
		g.monitor.RecordRequest("listSessions", 500, time.Since(start))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
	g.monitor.RecordRequest("listSessions", 200, time.Since(start))
}

// Middleware functions
func (g *Gateway) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		g.logger.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
			"remote": r.RemoteAddr,
			"start":  start,
		}).Info("Request started")

		next.ServeHTTP(w, r)

		g.logger.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
			"remote": r.RemoteAddr,
			"duration": time.Since(start),
		}).Info("Request completed")
	})
}

func (g *Gateway) corsMiddleware(next http.Handler) http.Handler {
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

func (g *Gateway) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		token := authHeader[len("Bearer "):]
		if token == "" {
			http.Error(w, "Bearer token required", http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := g.authClient.ValidateToken(token)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Get or create session
		sessionID, err := g.sessionMgr.GetOrCreateSession(claims.UserID)
		if err != nil {
			http.Error(w, "Session management error", http.StatusInternalServerError)
			return
		}

		// Add session to context
		ctx := context.WithValue(r.Context(), "session_id", sessionID)
		ctx = context.WithValue(ctx, "user_id", claims.UserID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}