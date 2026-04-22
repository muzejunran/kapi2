package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type TestRequest struct {
	Message     string `json:"message"`
	PageContext string `json:"page_context,omitempty"`
}

type TestResult struct {
	Success           bool
	Latency           time.Duration
	FirstTokenLatency time.Duration
	TokenUsage        int
	ErrorMessage      string
	StreamEvents      int
}

type Statistics struct {
	TotalRequests      int64
	SuccessCount       int64
	ErrorCount         int64
	Latencies          []time.Duration
	FirstTokenLatencies []time.Duration
	TokenUsages        []int
	mu                 sync.Mutex
}

type Wordlist struct {
	Scenarios []struct {
		Name        string   `json:"name"`
		PageContext string   `json:"page_context"`
		Messages    []string `json:"messages"`
	} `json:"scenarios"`
}

var (
	serverURL  = flag.String("url", "http://localhost:8080", "Server URL")
	concurrent = flag.Int("c", 10, "Concurrent requests (also total requests)")
	timeout    = flag.Duration("timeout", 5*time.Minute, "Request timeout")
	wordlist   = flag.String("wordlist", "test/loadtest/wordlist.json", "Wordlist file path")
	scenario   = flag.String("scenario", "", "Test only specific scenario (e.g., add_bill, query_bills), empty = all scenarios")
)

func main() {
	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("         Load Test Tool")
	fmt.Println("========================================")
	fmt.Printf("Server:       %s\n", *serverURL)
	fmt.Printf("Concurrent:   %d\n", *concurrent)
	fmt.Printf("Timeout:      %v\n", *timeout)
	fmt.Println("========================================\n")

	// Load wordlist
	wordlistData, err := loadWordlist(*wordlist)
	if err != nil {
		fmt.Printf("Failed to load wordlist: %v\n", err)
		os.Exit(1)
	}

	// Prepare test cases
	testCases := prepareTestCases(wordlistData, *concurrent, *scenario)

	// Run load test
	stats := runLoadTest(*serverURL, testCases, *concurrent, *timeout)

	// Print results
	printStatistics(stats)
}

func loadWordlist(path string) (*Wordlist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wl Wordlist
	if err := json.Unmarshal(data, &wl); err != nil {
		return nil, err
	}
	return &wl, nil
}

func prepareTestCases(wl *Wordlist, total int, scenarioFilter string) []TestRequest {
	cases := make([]TestRequest, 0, total)
	allMessages := []struct {
		msg  string
		ctx  string
	}{}

	for _, scenario := range wl.Scenarios {
		// Filter by scenario if specified
		if scenarioFilter != "" && scenario.Name != scenarioFilter {
			continue
		}

		for _, msg := range scenario.Messages {
			allMessages = append(allMessages, struct {
				msg  string
				ctx  string
			}{msg, scenario.PageContext})
		}
	}

	if len(allMessages) == 0 {
		// Fallback test cases
		allMessages = []struct {
			msg  string
			ctx  string
		}{
			{"帮我记录下昨天星巴克消费23元", "add_bill"},
			{"帮我查询下2026年四月消费了多少", "query_bills"},
		}
	}

	for i := 0; i < total; i++ {
		msg := allMessages[i%len(allMessages)]
		cases = append(cases, TestRequest{
			Message:     msg.msg,
			PageContext: msg.ctx,
		})
	}

	return cases
}

func runLoadTest(serverURL string, testCases []TestRequest, concurrent int, timeout time.Duration) *Statistics {
	stats := &Statistics{
		Latencies:   make([]time.Duration, 0, len(testCases)),
		TokenUsages: make([]int, 0, len(testCases)),
	}

	semaphore := make(chan struct{}, concurrent)
	var wg sync.WaitGroup

	startTime := time.Now()

	for i, tc := range testCases {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, req TestRequest) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result := makeRequest(serverURL, req, timeout)

			atomic.AddInt64(&stats.TotalRequests, 1)
			if result.Success {
				atomic.AddInt64(&stats.SuccessCount, 1)
			} else {
				atomic.AddInt64(&stats.ErrorCount, 1)
			}

			stats.mu.Lock()
			stats.Latencies = append(stats.Latencies, result.Latency)
			stats.FirstTokenLatencies = append(stats.FirstTokenLatencies, result.FirstTokenLatency)
			if result.TokenUsage > 0 {
				stats.TokenUsages = append(stats.TokenUsages, result.TokenUsage)
			}
			stats.mu.Unlock()

			// Print error details
			if !result.Success {
				fmt.Printf("[Request %d] Error: %s\n", idx+1, result.ErrorMessage)
			}
		}(i, tc)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	fmt.Printf("Test completed in: %v\n", totalDuration)
	fmt.Printf("Throughput: %.2f req/s\n\n", float64(stats.TotalRequests)/totalDuration.Seconds())

	return stats
}

func makeRequest(serverURL string, req TestRequest, timeout time.Duration) TestResult {
	result := TestResult{}
	requestStart := time.Now()

	// Create request
	reqBody, _ := json.Marshal(req)
	sessionID := fmt.Sprintf("test-session-%d", time.Now().UnixNano())
	url := fmt.Sprintf("%s/sessions/%s/stream?hint_code=1", serverURL, sessionID)

	// Debug: print request details
	fmt.Printf("[DEBUG] Request URL: %s\n", url)
	fmt.Printf("[DEBUG] Request Body: %s\n", string(reqBody))

	client := &http.Client{Timeout: timeout}
	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(httpReq)
	if err != nil {
		result.Success = false
		result.ErrorMessage = err.Error()
		fmt.Printf("[DEBUG] Request failed: %v\n", err)
		return result
	}
	defer resp.Body.Close()

	fmt.Printf("[DEBUG] Response Status: %d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return result
	}

	// Read SSE stream with single goroutine
	scanner := bufio.NewScanner(resp.Body)
	tokenUsage := 0
	firstTokenReceived := false
	streamEnded := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				streamEnded = true
				break
			}

			// Try to extract token usage from response
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				if usage, ok := event["token_usage"].(float64); ok {
					tokenUsage = int(usage)
					fmt.Printf("[DEBUG] Got token_usage: %d\n", tokenUsage)
				}
				// Also check for usage object
				if usageObj, ok := event["usage"].(map[string]interface{}); ok {
					if total, ok := usageObj["total_tokens"].(float64); ok {
						tokenUsage = int(total)
						fmt.Printf("[DEBUG] Got usage.total_tokens: %d\n", tokenUsage)
					}
				}
			}

			// Record first token latency
			if !firstTokenReceived {
				result.FirstTokenLatency = time.Since(requestStart)
				firstTokenReceived = true
				fmt.Printf("[DEBUG] First token received in: %v\n", result.FirstTokenLatency)
			}
		}
		result.StreamEvents++
	}

	if err := scanner.Err(); err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Stream error: %v", err)
		return result
	}

	if !firstTokenReceived {
		result.Success = false
		result.ErrorMessage = "No data received from stream"
		return result
	}

	// Record total latency
	result.Latency = time.Since(requestStart)
	result.Success = true
	result.TokenUsage = tokenUsage

	fmt.Printf("[DEBUG] Stream complete. Events: %d, TokenUsage: %d, StreamEnded: %v\n",
		result.StreamEvents, result.TokenUsage, streamEnded)

	return result
}

func printStatistics(stats *Statistics) {
	fmt.Println("========================================")
	fmt.Println("         Test Results")
	fmt.Println("========================================\n")

	fmt.Printf("Total Requests:  %d\n", stats.TotalRequests)
	fmt.Printf("Success:         %d\n", stats.SuccessCount)
	fmt.Printf("Errors:          %d\n", stats.ErrorCount)
	fmt.Printf("Error Rate:      %.2f%%\n\n", float64(stats.ErrorCount)/float64(stats.TotalRequests)*100)

	if len(stats.Latencies) > 0 {
		sort.Slice(stats.Latencies, func(i, j int) bool {
			return stats.Latencies[i] < stats.Latencies[j]
		})

		fmt.Println("Total Latency (ms):")
		fmt.Printf("  Min:   %d\n", stats.Latencies[0].Milliseconds())
		fmt.Printf("  Max:   %d\n", stats.Latencies[len(stats.Latencies)-1].Milliseconds())
		fmt.Printf("  Avg:   %d\n", avg(stats.Latencies).Milliseconds())
		fmt.Printf("  P50:   %d\n", percentile(stats.Latencies, 50).Milliseconds())
		fmt.Printf("  P95:   %d\n", percentile(stats.Latencies, 95).Milliseconds())
		fmt.Printf("  P99:   %d\n\n", percentile(stats.Latencies, 99).Milliseconds())
	}

	if len(stats.FirstTokenLatencies) > 0 {
		sort.Slice(stats.FirstTokenLatencies, func(i, j int) bool {
			return stats.FirstTokenLatencies[i] < stats.FirstTokenLatencies[j]
		})

		fmt.Println("First Token Latency (ms):")
		fmt.Printf("  Min:   %d\n", stats.FirstTokenLatencies[0].Milliseconds())
		fmt.Printf("  Max:   %d\n", stats.FirstTokenLatencies[len(stats.FirstTokenLatencies)-1].Milliseconds())
		fmt.Printf("  Avg:   %d\n", avg(stats.FirstTokenLatencies).Milliseconds())
		fmt.Printf("  P50:   %d\n", percentile(stats.FirstTokenLatencies, 50).Milliseconds())
		fmt.Printf("  P95:   %d\n", percentile(stats.FirstTokenLatencies, 95).Milliseconds())
		fmt.Printf("  P99:   %d\n\n", percentile(stats.FirstTokenLatencies, 99).Milliseconds())
	}

	if len(stats.TokenUsages) > 0 {
		sort.Ints(stats.TokenUsages)

		fmt.Println("Token Usage:")
		fmt.Printf("  Min:   %d\n", stats.TokenUsages[0])
		fmt.Printf("  Max:   %d\n", stats.TokenUsages[len(stats.TokenUsages)-1])
		fmt.Printf("  Avg:   %.0f\n", avgInt(stats.TokenUsages))
		fmt.Printf("  Total: %d\n\n", sumInt(stats.TokenUsages))

		// Print token usage curve
		printTokenCurve(stats.TokenUsages)
	}

	fmt.Println("========================================")
}

func avg(latencies []time.Duration) time.Duration {
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	return sum / time.Duration(len(latencies))
}

func avgInt(values []int) float64 {
	sum := sumInt(values)
	return float64(sum) / float64(len(values))
}

func sumInt(values []int) int {
	sum := 0
	for _, v := range values {
		sum += v
	}
	return sum
}

func percentile(latencies []time.Duration, p int) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(latencies))*float64(p)/100) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(latencies) {
		idx = len(latencies) - 1
	}
	return latencies[idx]
}

func printTokenCurve(usages []int) {
	if len(usages) == 0 {
		return
	}

	fmt.Println("Token Usage Distribution (per 100 tokens):")
	buckets := make(map[int]int)
	bucketSize := 100

	for _, u := range usages {
		bucket := (u / bucketSize) * bucketSize
		buckets[bucket]++
	}

	keys := make([]int, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, bucket := range keys {
		count := buckets[bucket]
		bar := strings.Repeat("█", count)
		fmt.Printf("  %d-%d: %s %d\n", bucket, bucket+bucketSize-1, bar, count)
	}
	fmt.Println()
}
