package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	requestsTotal   prometheus.CounterVec
	responseTime    prometheus.HistogramVec
	errorCount     prometheus.CounterVec
	activeUsers     prometheus.Gauge
	activeSessions prometheus.Gauge
}

type Monitor struct {
	metrics      Metrics
	server       *http.Server
	config       Config
	requestStats map[string]RequestStats
	mu           sync.RWMutex
}

type Config struct {
	Port string
}

type RequestStats struct {
	TotalRequests   int64
	SuccessRequests int64
	ErrorRequests  int64
	AvgResponseTime float64
	LastUpdated     time.Time
}

func NewMonitor(port string) *Monitor {
	config := Config{Port: port}

	// Initialize Prometheus metrics
	metrics := Metrics{
		requestsTotal: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_requests_total",
				Help: "Total number of API requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		responseTime: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "api_response_time_seconds",
				Help:    "API response time in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		errorCount: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "api_errors_total",
				Help: "Total number of API errors",
			},
			[]string{"method", "endpoint", "error_type"},
		),
		activeUsers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "active_users_total",
			Help: "Current number of active users",
		}),
		activeSessions: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "active_sessions_total",
			Help: "Current number of active sessions",
		}),
	}

	// Register metrics
	prometheus.MustRegister(
		&metrics.requestsTotal,
		&metrics.responseTime,
		&metrics.errorCount,
		metrics.activeUsers,
		metrics.activeSessions,
	)

	return &Monitor{
		metrics:      metrics,
		config:       config,
		requestStats: make(map[string]RequestStats),
	}
}

func (m *Monitor) Start() error {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptime": time.Since(time.Now()),
		})
	})

	// Stats endpoint
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		m.mu.RLock()
		stats := m.requestStats
		m.mu.RUnlock()

		json.NewEncoder(w).Encode(stats)
	})

	m.server = &http.Server{
		Addr:    ":" + m.config.Port,
		Handler: mux,
	}

	fmt.Printf("Monitoring server starting on port %s\n", m.config.Port)
	return m.server.ListenAndServe()
}

func (m *Monitor) Stop() error {
	if m.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return m.server.Shutdown(ctx)
	}
	return nil
}

func (m *Monitor) RecordRequest(endpoint string, status int, duration time.Duration) {
	method := "POST"
	if status >= 200 && status < 300 {
		statusStr := "success"
		m.metrics.requestsTotal.WithLabelValues(method, endpoint, statusStr).Inc()
	} else {
		statusStr := "error"
		m.metrics.requestsTotal.WithLabelValues(method, endpoint, statusStr).Inc()
		m.metrics.errorCount.WithLabelValues(method, endpoint, fmt.Sprintf("status_%d", status)).Inc()
	}

	m.metrics.responseTime.WithLabelValues(method, endpoint).Observe(duration.Seconds())

	// Update request stats
	m.mu.Lock()
	defer m.mu.Unlock()

	key := endpoint
	stats, exists := m.requestStats[key]
	if !exists {
		stats = RequestStats{}
	}

	stats.TotalRequests++
	if status >= 200 && status < 300 {
		stats.SuccessRequests++
	} else {
		stats.ErrorRequests++
	}

	// Calculate average response time
	stats.AvgResponseTime = (stats.AvgResponseTime*float64(stats.TotalRequests-1) + duration.Seconds()) / float64(stats.TotalRequests)
	stats.LastUpdated = time.Now()

	m.requestStats[key] = stats
}

func (m *Monitor) SetActiveUsers(count int) {
	m.metrics.activeUsers.Set(float64(count))
}

func (m *Monitor) SetActiveSessions(count int) {
	m.metrics.activeSessions.Set(float64(count))
}

func (m *Monitor) GetRequestStats() map[string]RequestStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statsCopy := make(map[string]RequestStats)
	for k, v := range m.requestStats {
		statsCopy[k] = v
	}

	return statsCopy
}

// Health check helper
func (m *Monitor) CheckServiceHealth(serviceURL string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serviceURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// Service Registry
type ServiceRegistry struct {
	services map[string]ServiceInfo
	mu       sync.RWMutex
}

type ServiceInfo struct {
	Name        string
	URL         string
	HealthURL   string
	Healthy     bool
	LastChecked time.Time
}

func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]ServiceInfo),
	}
}

func (sr *ServiceRegistry) RegisterService(name, url, healthURL string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.services[name] = ServiceInfo{
		Name:      name,
		URL:       url,
		HealthURL: healthURL,
		Healthy:   true,
	}
}

func (sr *ServiceRegistry) CheckAllServices(monitor *Monitor) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	for name := range sr.services {
		service := sr.services[name]
		healthy := monitor.CheckServiceHealth(service.HealthURL)
		service.Healthy = healthy
		service.LastChecked = time.Now()
		sr.services[name] = service
	}
}

func (sr *ServiceRegistry) GetServiceStatus(name string) (ServiceInfo, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	service, exists := sr.services[name]
	return service, exists
}

func (sr *ServiceRegistry) GetAllServicesStatus() map[string]ServiceInfo {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	status := make(map[string]ServiceInfo)
	for name, service := range sr.services {
		status[name] = service
	}

	return status
}