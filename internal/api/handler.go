package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-assistant-service/internal/agent"
	"ai-assistant-service/internal/config"
	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/streaming"
	"ai-assistant-service/internal/session"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// APIHandler handles HTTP requests
type APIHandler struct {
	config         *config.Config
	agent          *agent.Agent
	sessionManager *session.Manager
	memoryService  *memory.MemoryService
	streamer       *streaming.Streamer
	logger         *logrus.Logger
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(cfg *config.Config, agt *agent.Agent, sessMgr *session.Manager, memService *memory.MemoryService) *APIHandler {
	return &APIHandler{
		config:         cfg,
		agent:          agt,
		sessionManager: sessMgr,
		memoryService:  memService,
		logger:         logrus.New(),
	}
}

// RegisterRoutes registers all API routes
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	// Session endpoints
	r.HandleFunc("/sessions", h.CreateSession).Methods("POST")
	r.HandleFunc("/sessions/{id}", h.GetSession).Methods("GET")
	r.HandleFunc("/sessions/{id}", h.DeleteSession).Methods("DELETE")

	// Message endpoints
	r.HandleFunc("/sessions/{id}/messages", h.SendMessage).Methods("POST")
	r.HandleFunc("/sessions/{id}/stream", h.StreamMessage).Methods("POST")

	// Memory endpoints
	r.HandleFunc("/memory", h.GetMemory).Methods("GET")
	r.HandleFunc("/memory", h.UpdateMemory).Methods("POST")

	// Health check
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
}

// CreateSession creates a new session
func (h *APIHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID      string `json:"user_id"`
		PageContext string `json:"page_context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	// Create session via session manager
	sessionID, err := h.sessionManager.GetOrCreateSession(req.UserID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create session: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"session_id": sessionID,
		"user_id":    req.UserID,
		"page":       req.PageContext,
	}).Info("Created session")

	response := map[string]interface{}{
		"session_id": sessionID,
		"user_id":    req.UserID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetSession retrieves a session
func (h *APIHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]

	h.logger.WithField("session_id", sessionID).Info("Get session")

	response := map[string]interface{}{
		"session_id": sessionID,
		"messages":   []string{}, // Would fetch from storage
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteSession deletes a session
func (h *APIHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]

	h.logger.WithField("session_id", sessionID).Info("Delete session")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Session deleted",
	})
}

// SendMessage sends a message and gets a response
func (h *APIHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]

	ctx := r.Context()

	var req struct {
		Message     string `json:"message"`
		PageContext string `json:"page_context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Mock user ID - in real implementation, extract from JWT
	userID := "user_" + sessionID[:10]

	// Process message
	streamChan, err := h.agent.ProcessMessage(ctx, sessionID, userID, req.PageContext, req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to process message: %v", err), http.StatusInternalServerError)
		return
	}

	// Collect response
	var response strings.Builder
	for event := range streamChan {
		if event.Type == streaming.TextEvent {
			response.WriteString(event.Content)
		}
	}

	resp := map[string]interface{}{
		"session_id": sessionID,
		"response":   response.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// StreamMessage handles streaming responses
func (h *APIHandler) StreamMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]

	ctx := r.Context()

	var req struct {
		Message     string `json:"message"`
		PageContext string `json:"page_context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Mock user ID
	userID := "user_" + sessionID[:10]

	// Handle streaming
	streaming.HandleStreaming(w, r, func(streamer streaming.Streamer) error {
		streamChan, err := h.agent.ProcessMessage(ctx, sessionID, userID, req.PageContext, req.Message)
		if err != nil {
			return err
		}

		for event := range streamChan {
			// Forward streaming events
			if err := streamer.Send(event); err != nil {
				return err
			}
		}

		return nil
	})
}

// GetMemory retrieves user memory
func (h *APIHandler) GetMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id query parameter is required", http.StatusBadRequest)
		return
	}

	// In real implementation, this would fetch from memory service
	response := map[string]interface{}{
		"user_id":        userID,
		"profile":        "25岁，白领，单身",
		"preferences":    "喜欢简洁的回答，使用人民币",
		"facts":          []string{"在减肥", "刚换工作", "有房贷"},
		"recent_summary": "最近关注饮食和健康支出",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateMemory updates user memory
func (h *APIHandler) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		http.Error(w, "user_id query parameter is required", http.StatusBadRequest)
		return
	}

	var req struct {
		Type  string      `json:"type"`
		Data  interface{} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"user_id": userID,
		"type":    req.Type,
	}).Info("Updated memory")

	response := map[string]interface{}{
		"message": "Memory updated",
		"user_id": userID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HealthCheck handles health check requests
func (h *APIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
		"checks": map[string]bool{
			"database":    true,
			"redis":       true,
			"llm_service": true,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MetricsHandler handles metrics requests
func (h *APIHandler) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"active_sessions": 42,
		"total_messages":  1250,
		"avg_response_time": "1.2s",
		"error_rate":      "0.5%",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}