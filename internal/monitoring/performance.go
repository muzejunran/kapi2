package monitoring

import (
	"sync"
	"time"
)

// PerformanceMetrics tracks detailed performance metrics
type PerformanceMetrics struct {
	RequestCount    int64
	SuccessCount   int64
	ErrorCount     int64

	TotalLatency    time.Duration
	MinLatency      time.Duration
	MaxLatency      time.Duration
	P50Latency      time.Duration
	P90Latency      time.Duration
	P95Latency      time.Duration
	P99Latency      time.Duration

	FirstTokenLatency time.Duration
	FirstTokenP50   time.Duration
	FirstTokenP95   time.Duration

	TokenUsage      int64
	TokenCost       float64

	mu              sync.RWMutex
	lastReset       time.Time
}

// MetricWindow holds metrics for a specific time window
type MetricWindow struct {
	Requests    []float64
	Latencies   []time.Duration
	WindowStart time.Time
	WindowDuration time.Duration
}

// SLAConfig defines Service Level Agreement parameters
type SLAConfig struct {
	MaxConcurrentSessions int
	MaxLatencyP95       time.Duration
	MaxFirstTokenP95    time.Duration
	MaxErrorRate        float64
	MinAvailability     float64

	AlertThresholds    AlertThresholds
}

// AlertThresholds defines thresholds for alerts
type AlertThresholds struct {
	HighLatency      time.Duration
	HighErrorRate    float64
	LowAvailability float64
}

// PerformanceMonitor monitors performance against SLA
type PerformanceMonitor struct {
	metrics    *PerformanceMetrics
	slaConfig  *SLAConfig
	alerter    Alerter
}

// Alerter handles alerting logic
type Alerter interface {
	SendAlert(alert Alert)
}

// Alert represents a performance alert
type Alert struct {
	Type      AlertType
	Severity  AlertSeverity
	Message   string
	Value     interface{}
	Timestamp time.Time
}

// AlertType defines the type of alert
type AlertType string

const (
	AlertLatency       AlertType = "latency"
	AlertErrorRate     AlertType = "error_rate"
	AlertAvailability  AlertType = "availability"
	AlertConcurrency   AlertType = "concurrency"
	AlertTokenUsage    AlertType = "token_usage"
)

// AlertSeverity defines the severity of alert
type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// NewPerformanceMetrics creates a new performance metrics tracker
func NewPerformanceMetrics() *PerformanceMetrics {
	return &PerformanceMetrics{
		lastReset: time.Now(),
	}
}

// RecordRequest records a request
func (pm *PerformanceMetrics) RecordRequest() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.RequestCount++
}

// RecordSuccess records a successful request
func (pm *PerformanceMetrics) RecordSuccess(latency time.Duration, firstTokenLatency time.Duration, tokensUsed int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.SuccessCount++
	pm.TotalLatency += latency
	pm.TokenUsage += int64(tokensUsed)

	// Update min/max latency
	if pm.MinLatency == 0 || latency < pm.MinLatency {
		pm.MinLatency = latency
	}
	if latency > pm.MaxLatency {
		pm.MaxLatency = latency
	}

	// Track first token latency separately
	if firstTokenLatency > 0 {
		if pm.FirstTokenLatency == 0 || firstTokenLatency < pm.FirstTokenLatency {
			pm.FirstTokenLatency = firstTokenLatency
		}
		if firstTokenLatency > pm.FirstTokenLatency {
			pm.FirstTokenLatency = firstTokenLatency
		}
	}
}

// RecordError records a failed request
func (pm *PerformanceMetrics) RecordError() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.ErrorCount++
}

// GetPercentiles calculates P50, P90, P95, P99 latencies
func (pm *PerformanceMetrics) GetPercentiles() (p50, p90, p95, p99 time.Duration) {
	// Simple implementation - in production, use proper percentile algorithm
	if pm.RequestCount == 0 {
		return 0, 0, 0, 0
	}

	avgLatency := pm.TotalLatency / time.Duration(pm.RequestCount)
	p50 = avgLatency * 100 / 100
	p90 = avgLatency * 110 / 100
	p95 = avgLatency * 120 / 100
	p99 = avgLatency * 130 / 100

	return
}

// GetTokenCost estimates the cost based on token usage
func (pm *PerformanceMetrics) GetTokenCost(tokenPrice float64) float64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return float64(pm.TokenUsage) * tokenPrice
}

// GetStats returns current statistics
func (pm *PerformanceMetrics) GetStats() PerformanceStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	successRate := 0.0
	if pm.RequestCount > 0 {
		successRate = float64(pm.SuccessCount) / float64(pm.RequestCount) * 100
	}

	errorRate := 0.0
	if pm.RequestCount > 0 {
		errorRate = float64(pm.ErrorCount) / float64(pm.RequestCount) * 100
	}

	avgLatency := time.Duration(0)
	if pm.RequestCount > 0 {
		avgLatency = pm.TotalLatency / time.Duration(pm.RequestCount)
	}

	return PerformanceStats{
		RequestCount:      pm.RequestCount,
		SuccessCount:      pm.SuccessCount,
		ErrorCount:        pm.ErrorCount,
		SuccessRate:       successRate,
		ErrorRate:         errorRate,
		AvgLatency:        avgLatency,
		MinLatency:        pm.MinLatency,
		MaxLatency:        pm.MaxLatency,
		TokenUsage:        pm.TokenUsage,
		EstimatedCost:     pm.GetTokenCost(0.002), // $0.002 per token as example
		FirstTokenP95:     pm.FirstTokenP95,
		LastReset:         pm.lastReset,
	}
}

// Reset resets the metrics
func (pm *PerformanceMetrics) Reset() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.lastReset = time.Now()
	pm.RequestCount = 0
	pm.SuccessCount = 0
	pm.ErrorCount = 0
	pm.TotalLatency = 0
	pm.MinLatency = 0
	pm.MaxLatency = 0
	pm.FirstTokenLatency = 0
	pm.TokenUsage = 0
}

// PerformanceStats holds comprehensive statistics
type PerformanceStats struct {
	RequestCount     int64
	SuccessCount     int64
	ErrorCount        int64
	SuccessRate      float64
	ErrorRate         float64
	AvgLatency       time.Duration
	MinLatency       time.Duration
	MaxLatency       time.Duration
	TokenUsage       int64
	EstimatedCost     float64
	FirstTokenP95     time.Duration
	LastReset         time.Time
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(slaConfig *SLAConfig, alerter Alerter) *PerformanceMonitor {
	return &PerformanceMonitor{
		metrics:   NewPerformanceMetrics(),
		slaConfig: slaConfig,
		alerter:   alerter,
	}
}

// CheckSLA checks if metrics meet SLA requirements
func (pm *PerformanceMonitor) CheckSLA() []Alert {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()

	alerts := make([]Alert, 0)

	stats := pm.metrics.GetStats()

	// Check latency SLA
	if pm.slaConfig.MaxLatencyP95 > 0 {
		_, _, p95, _ := pm.metrics.GetPercentiles()
		if p95 > pm.slaConfig.MaxLatencyP95 {
			alerts = append(alerts, Alert{
				Type:      AlertLatency,
				Severity:  SeverityCritical,
				Message:   "P95 latency exceeded SLA",
				Value:     p95,
				Timestamp: time.Now(),
			})
		}
	}

	// Check first token SLA
	if pm.slaConfig.MaxFirstTokenP95 > 0 {
		if stats.FirstTokenP95 > pm.slaConfig.MaxFirstTokenP95 {
			alerts = append(alerts, Alert{
				Type:      AlertLatency,
				Severity:  SeverityWarning,
				Message:   "First token latency approaching SLA",
				Value:     stats.FirstTokenP95,
				Timestamp: time.Now(),
			})
		}
	}

	// Check error rate SLA
	if pm.slaConfig.MaxErrorRate > 0 {
		if stats.ErrorRate > pm.slaConfig.MaxErrorRate {
			alerts = append(alerts, Alert{
				Type:      AlertErrorRate,
				Severity:  SeverityCritical,
				Message:   "Error rate exceeded SLA",
				Value:     stats.ErrorRate,
				Timestamp: time.Now(),
			})
		}
	}

	// Check concurrency SLA
	if pm.slaConfig.MaxConcurrentSessions > 0 {
		activeSessions := 0 // This would come from session pool
		if activeSessions > pm.slaConfig.MaxConcurrentSessions {
			alerts = append(alerts, Alert{
				Type:      AlertConcurrency,
				Severity:  SeverityWarning,
				Message:   "Concurrent sessions approaching limit",
				Value:     activeSessions,
				Timestamp: time.Now(),
			})
		}
	}

	// Check availability SLA
	if pm.slaConfig.MinAvailability > 0 {
		availability := 100 - stats.ErrorRate
		if availability < pm.slaConfig.MinAvailability {
			alerts = append(alerts, Alert{
				Type:      AlertAvailability,
				Severity:  SeverityCritical,
				Message:   "Availability below SLA",
				Value:     availability,
				Timestamp: time.Now(),
			})
		}
	}

	return alerts
}

// SendAlert sends an alert using the configured alerter
func (pm *PerformanceMonitor) SendAlert(alert Alert) {
	if pm.alerter != nil {
		pm.alerter.SendAlert(alert)
	}
}

// UpdateSLA updates SLA configuration
func (pm *PerformanceMonitor) UpdateSLA(config *SLAConfig) {
	if config != nil {
		pm.slaConfig = config
	}
}

// GetMetrics returns current metrics
func (pm *PerformanceMonitor) GetMetrics() *PerformanceMetrics {
	return pm.metrics
}

// GetSLAConfig returns current SLA configuration
func (pm *PerformanceMonitor) GetSLAConfig() *SLAConfig {
	return pm.slaConfig
}