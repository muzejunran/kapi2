package loadbalancer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type LoadBalancer struct {
	services      []Service
	healthStatus  map[string]bool
	mu           sync.RWMutex
	roundRobinIdx int
}

type Service struct {
	Address string
	Port    string
	Healthy bool
}

type LLMRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	Model    string                    `json:"model"`
}

type LLMResponse struct {
	Content string `json:"content"`
}

func NewLoadBalancer(servicePorts []string) *LoadBalancer {
	lb := &LoadBalancer{
		healthStatus: make(map[string]bool),
	}

	// Initialize services
	for _, port := range servicePorts {
		service := Service{
			Address: "localhost",
			Port:    port,
			Healthy: true,
		}
		lb.services = append(lb.services, service)
		lb.healthStatus[port] = true
	}

	// Start health checks
	go lb.startHealthChecks()

	return lb
}

func (lb *LoadBalancer) ForwardToLLM(messages []map[string]interface{}) (string, error) {
	// Get next healthy service using round-robin
	service, err := lb.getNextHealthyService()
	if err != nil {
		return "", err
	}

	// Prepare request
	req := LLMRequest{
		Messages: messages,
		Model:    "gpt-3.5-turbo",
	}

	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Send request to LLM service
	url := fmt.Sprintf("http://%s:%s/chat", service.Address, service.Port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		lb.markServiceUnhealthy(service.Port)
		return "", fmt.Errorf("service %s unavailable: %v", service.Port, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		lb.markServiceUnhealthy(service.Port)
		return "", fmt.Errorf("service %s returned status %d", service.Port, resp.StatusCode)
	}

	// Parse response
	var llmResp LLMResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	return llmResp.Content, nil
}

func (lb *LoadBalancer) getNextHealthyService() (*Service, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Find next healthy service using round-robin
	startIdx := lb.roundRobinIdx
	for i := 0; i < len(lb.services); i++ {
		idx := (startIdx + i) % len(lb.services)
		service := &lb.services[idx]

		if lb.healthStatus[service.Port] {
			lb.roundRobinIdx = (idx + 1) % len(lb.services)
			return service, nil
		}
	}

	return nil, errors.New("no healthy services available")
}

func (lb *LoadBalancer) markServiceUnhealthy(port string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.healthStatus[port] = false
	fmt.Printf("Marked service %s as unhealthy\n", port)
}

func (lb *LoadBalancer) startHealthChecks() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lb.checkAllServices()
		}
	}
}

func (lb *LoadBalancer) checkAllServices() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for _, service := range lb.services {
		go lb.checkServiceHealth(service)
	}
}

func (lb *LoadBalancer) checkServiceHealth(service Service) {
	url := fmt.Sprintf("http://%s:%s/health", service.Address, service.Port)

	resp, err := http.Get(url)
	if err != nil {
		lb.markServiceUnhealthy(service.Port)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		lb.healthStatus[service.Port] = true
	} else {
		lb.markServiceUnhealthy(service.Port)
	}
}

func (lb *LoadBalancer) GetServiceStats() map[string]interface{} {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	stats := make(map[string]interface{})

	healthyCount := 0
	for _, healthy := range lb.healthStatus {
		if healthy {
			healthyCount++
		}
	}

	stats["total_services"] = len(lb.services)
	stats["healthy_services"] = healthyCount
	stats["unhealthy_services"] = len(lb.services) - healthyCount
	stats["services"] = lb.services

	return stats
}