package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"ai-assistant-service/internal/config"
)

type Client struct {
	config *config.Config
	client *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type LLMResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func NewClient(llmPort string) *Client {
	return &Client{
		config: &config.Config{LLMEndpoint: "http://localhost:" + llmPort},
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Chat(messages []Message, model string) (*LLMResponse, error) {
	req := LLMRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   1000,
		Temperature: 0.7,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", c.config.LLMEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.LLMApiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			return nil, fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, errorResp.Error.Message)
		}
		return nil, fmt.Errorf("LLM API returned status %d", resp.StatusCode)
	}

	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &llmResp, nil
}

// Mock LLM Service (simulates the actual LLM API)
type MockLLMService struct {
	port string
}

func NewMockService(port string) *MockLLMService {
	return &MockLLMService{
		port: port,
	}
}

func (m *MockLLMService) Start() error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	})

	// Chat endpoint
	mux.HandleFunc("/chat", m.handleChat)

	server := &http.Server{
		Addr:    ":" + m.port,
		Handler: mux,
	}

	fmt.Printf("Mock LLM service starting on port %s\n", m.port)
	return server.ListenAndServe()
}

func (m *MockLLMService) handleChat(w http.ResponseWriter, r *http.Request) {
	var req LLMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Generate mock response based on messages
	response := m.generateMockResponse(req.Messages)

	resp := LLMResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: response,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     len(req.Messages) * 50,
			CompletionTokens: len(response) / 4,
			TotalTokens:      len(req.Messages)*50 + len(response)/4,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (m *MockLLMService) generateMockResponse(messages []Message) string {
	lastMessage := ""
	if len(messages) > 0 {
		lastMessage = messages[len(messages)-1].Content
	}

	// Simple mock responses based on message content
	switch {
	case len(lastMessage) == 0:
		return "Hello! How can I help you today?"
	case len(lastMessage) > 100:
		return "I understand you're asking about something detailed. Let me provide a comprehensive response based on your message."
	default:
		return fmt.Sprintf("I received your message: \"%s\". How else can I assist you?", lastMessage)
	}
}

// Rate limiting wrapper for LLM calls
type RateLimitedLLM struct {
	client    *Client
	rateLimit int
	requests  map[string]int
	mu        sync.Mutex
}

func NewRateLimitedLLM(client *Client, rateLimit int) *RateLimitedLLM {
	return &RateLimitedLLM{
		client:    client,
		rateLimit: rateLimit,
		requests:  make(map[string]int),
	}
}

func (r *RateLimitedLLM) Chat(userID string, messages []Message, model string) (*LLMResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count, exists := r.requests[userID]
	if !exists {
		count = 0
	}

	if count >= r.rateLimit {
		return nil, fmt.Errorf("rate limit exceeded for user %s", userID)
	}

	r.requests[userID] = count + 1

	return r.client.Chat(messages, model)
}

func (r *RateLimitedLLM) ResetRateLimit(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.requests, userID)
}
