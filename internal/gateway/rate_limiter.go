package gateway

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	mu       sync.Mutex
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)

		if !rl.Allow(clientIP) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Get requests for this client
	requests, exists := rl.requests[clientIP]
	if !exists {
		rl.requests[clientIP] = []time.Time{now}
		return true
	}

	// Remove old requests outside the window
	validRequests := make([]time.Time, 0)
	for _, reqTime := range requests {
		if now.Sub(reqTime) <= rl.window {
			validRequests = append(validRequests, reqTime)
		}
	}

	// Add current request
	validRequests = append(validRequests, now)
	rl.requests[clientIP] = validRequests

	// Check if limit exceeded
	if len(validRequests) > rl.limit {
		return false
	}

	return true
}

func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for clientIP, requests := range rl.requests {
		validRequests := make([]time.Time, 0)
		for _, reqTime := range requests {
			if now.Sub(reqTime) <= rl.window {
				validRequests = append(validRequests, reqTime)
			}
		}
		if len(validRequests) > 0 {
			rl.requests[clientIP] = validRequests
		} else {
			delete(rl.requests, clientIP)
		}
	}
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}

	// Check X-Real-IP header
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Remote address
	ip := r.RemoteAddr
	return ip
}

// Token Bucket Rate Limiter
type TokenBucket struct {
	tokens      int
	maxTokens   int
	refillRate  time.Duration
	lastRefill  time.Time
	mu          sync.Mutex
}

func NewTokenBucket(maxTokens int, refillRate time.Duration) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Refill tokens
	elapsed := now.Sub(tb.lastRefill)
	tokensToAdd := int(elapsed / tb.refillRate)
	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.maxTokens {
			tb.tokens = tb.maxTokens
		}
		tb.lastRefill = now
	}

	// Check if we have tokens
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	return false
}

func (tb *TokenBucket) Refill() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}