package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-assistant-service/internal/agent"
	"ai-assistant-service/internal/config"
	"ai-assistant-service/internal/logger"
	"ai-assistant-service/internal/memory"
	"ai-assistant-service/internal/session"
	"ai-assistant-service/internal/streaming"

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
		logger:         logger.New(),
	}
}

// RegisterRoutes registers all API routes
func (h *APIHandler) RegisterRoutes(r *mux.Router) {
	// Session endpoints
	r.HandleFunc("/sessions", h.CreateSession).Methods("POST")
	r.HandleFunc("/sessions/{id}", h.GetSession).Methods("GET")
	r.HandleFunc("/sessions/{id}/close", h.CloseSession).Methods("POST")

	// Message endpoints
	r.HandleFunc("/sessions/{id}/messages", h.SendMessage).Methods("POST")
	r.HandleFunc("/sessions/{id}/stream", h.StreamMessage).Methods("POST")

	// Memory endpoints — user_id is derived from session, not passed as a parameter.
	// TODO: once auth is in place, derive user_id from the JWT token instead of session lookup.
	r.HandleFunc("/sessions/{id}/memory", h.GetMemory).Methods("GET")
	r.HandleFunc("/sessions/{id}/memory", h.UpdateMemory).Methods("POST")
	r.HandleFunc("/sessions/{id}/memory/clear", h.ClearMemory).Methods("POST")
	r.HandleFunc("/sessions/{id}/memory/facts/remove", h.RemoveMemoryFact).Methods("POST")

	// Health check
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
}

// userIDFromSession resolves the user_id for a session.
// TODO: once auth middleware is added, extract user_id from the verified JWT token
// in the Authorization header instead, and remove the session lookup dependency.
func (h *APIHandler) userIDFromSession(sessionID string) (string, error) {
	sess, err := h.sessionManager.GetSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %w", err)
	}
	if sess.UserID == "" {
		return "", fmt.Errorf("session has no associated user_id")
	}
	return sess.UserID, nil
}

// ── Session ───────────────────────────────────────────────────────────────────

// CreateSession creates a new session
func (h *APIHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID      string `json:"user_id"`
		PageContext string `json:"page_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	sessionID, err := h.sessionManager.GetOrCreateSession(req.UserID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create session: %v", err), http.StatusInternalServerError)
		return
	}

	logger.FromContext(r.Context()).WithFields(logrus.Fields{
		"session_id": sessionID,
		"user_id":    req.UserID,
		"page":       req.PageContext,
	}).Info("session created")

	writeJSON(w, map[string]interface{}{
		"session_id": sessionID,
		"user_id":    req.UserID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetSession retrieves a session
func (h *APIHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]
	sess, err := h.sessionManager.GetSession(sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, sess)
}

// CloseSession marks a session as closed (POST /sessions/{id}/close)
func (h *APIHandler) CloseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]
	if err := h.sessionManager.DeleteSession(sessionID); err != nil {
		http.Error(w, fmt.Sprintf("failed to close session: %v", err), http.StatusInternalServerError)
		return
	}
	logger.FromContext(r.Context()).WithField("session_id", sessionID).Info("session closed")
	writeJSON(w, map[string]string{"message": "session closed"})
}

// ── Messages ──────────────────────────────────────────────────────────────────

// SendMessage sends a message and returns the full response (non-streaming)
func (h *APIHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]
	ctx := r.Context()

	var req struct {
		Message     string `json:"message"`
		PageContext string `json:"page_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	userID, err := h.userIDFromSession(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	streamChan, err := h.agent.ProcessMessage(ctx, sessionID, userID, req.PageContext, req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to process message: %v", err), http.StatusInternalServerError)
		return
	}

	var response strings.Builder
	for event := range streamChan {
		if event.Type == streaming.TextEvent {
			response.WriteString(event.Content)
		}
	}

	writeJSON(w, map[string]interface{}{
		"session_id": sessionID,
		"response":   response.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// StreamMessage handles SSE streaming responses
func (h *APIHandler) StreamMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["id"]
	ctx := r.Context()

	var req struct {
		Message     string `json:"message"`
		PageContext string `json:"page_context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	userID, err := h.userIDFromSession(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	streaming.HandleStreaming(w, r, func(streamer streaming.Streamer) error {
		streamChan, err := h.agent.ProcessMessage(ctx, sessionID, userID, req.PageContext, req.Message)
		if err != nil {
			return err
		}
		for event := range streamChan {
			if err := streamer.Send(event); err != nil {
				return err
			}
		}
		return nil
	})
}

// ── Memory ────────────────────────────────────────────────────────────────────

// GetMemory returns the current memory for the session's user.
// GET /sessions/{id}/memory
func (h *APIHandler) GetMemory(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromSession(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	mem, err := h.memoryService.GetUserMemory(userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get memory: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, mem)
}

// UpdateMemory updates a specific memory field for the session's user.
// POST /sessions/{id}/memory
// Body: {"type": "profile"|"preferences"|"fact", "data": "..."}
func (h *APIHandler) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromSession(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var req struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	switch req.Type {
	case "profile":
		err = h.memoryService.UpdateProfile(userID, req.Data)
	case "preferences":
		err = h.memoryService.UpdatePreferences(userID, req.Data)
	case "fact":
		err = h.memoryService.AddFact(userID, req.Data)
	default:
		http.Error(w, "type must be profile, preferences, or fact", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update memory: %v", err), http.StatusInternalServerError)
		return
	}

	logger.FromContext(r.Context()).WithFields(logrus.Fields{"user_id": userID, "type": req.Type}).Info("memory updated")
	writeJSON(w, map[string]string{"message": "memory updated"})
}

// ClearMemory wipes all memory for the session's user.
// POST /sessions/{id}/memory/clear
func (h *APIHandler) ClearMemory(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromSession(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if err := h.memoryService.ClearMemory(userID); err != nil {
		http.Error(w, fmt.Sprintf("failed to clear memory: %v", err), http.StatusInternalServerError)
		return
	}

	logger.FromContext(r.Context()).WithField("user_id", userID).Info("memory cleared")
	writeJSON(w, map[string]string{"message": "memory cleared"})
}

// RemoveMemoryFact removes a single fact by zero-based index.
// POST /sessions/{id}/memory/facts/remove
// Body: {"index": 2}
func (h *APIHandler) RemoveMemoryFact(w http.ResponseWriter, r *http.Request) {
	userID, err := h.userIDFromSession(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var req struct {
		Index int `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	index := req.Index

	if err := h.memoryService.RemoveFact(userID, index); err != nil {
		http.Error(w, fmt.Sprintf("failed to remove fact: %v", err), http.StatusInternalServerError)
		return
	}

	logger.FromContext(r.Context()).WithFields(logrus.Fields{"user_id": userID, "index": index}).Info("memory fact removed")
	writeJSON(w, map[string]string{"message": "fact removed"})
}

// ── Health ────────────────────────────────────────────────────────────────────

// HealthCheck handles health check requests
func (h *APIHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// ── helpers ────────��──────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.New().WithError(err).Error("writeJSON error")
	}
}
