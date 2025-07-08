package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/devuk0204/ctrlbench/types"
)

// APIExecutor handles API execution and benchmarking
type APIExecutor struct {
	Timeout         time.Duration
	httpClient      *HTTPClient
	requestBuilder  *RequestBuilder
	benchmarkRunner *BenchmarkRunner
}

// RequestResult represents individual request result for concurrent benchmarking
type RequestResult struct {
	Duration     time.Duration
	StatusCode   int
	Error        error
	WorkerID     int
	Timestamp    time.Time
	ResponseBody string
}

// NewAPIExecutor creates a new API executor
func NewAPIExecutor(timeout time.Duration) *APIExecutor {
	return &APIExecutor{
		Timeout:         timeout,
		httpClient:      NewHTTPClient(),
		requestBuilder:  NewRequestBuilder(),
		benchmarkRunner: NewBenchmarkRunner(),
	}
}

// ExecuteAPI executes a specific API call using api_list.yaml
func (e *APIExecutor) ExecuteAPI(targetNF, apiName string) (*types.APIExecutionInfo, error) {
	// Load API list
	apiList, err := e.requestBuilder.LoadAPIList()
	if err != nil {
		return nil, fmt.Errorf("failed to load API list: %w", err)
	}

	// Load configuration
	config, err := LoadConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Prepare execution info from api_list and configuration (with required validation)
	execInfo, err := e.requestBuilder.PrepareAPIExecution(apiList, config, targetNF, apiName)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare API execution: %w", err)
	}

	// Get global settings
	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

	var discoveredURL string

	// Skip NF Discovery for NRF - use NRF URL directly
	if strings.ToUpper(targetNF) == "NRF" {
		nrfURL, ok := GetConfigString(globalSettings["nrf_url"])
		if !ok || nrfURL == "" {
			return nil, fmt.Errorf("NRF URL is required in configuration for NRF target")
		}
		discoveredURL = nrfURL
		fmt.Printf("✅ Using direct NRF URL: %s\n", discoveredURL)
	} else {
		// Discover NF URL for other NFs
		var err error
		discoveredURL, err = e.discoverNFURL(globalSettings, targetNF)
		if err != nil {
			return nil, fmt.Errorf("NF discovery failed: %w", err)
		}

		// For testing purposes, replace discovered URL
		if discoveredURL == "http://controlplane-free5gc-ausf-service:80" {
			discoveredURL = "http://10.96.43.148:80"
		}

		fmt.Printf("✅ Discovered %s URL: %s\n", targetNF, discoveredURL)
	}

	e.requestBuilder.PopulateHeaders(execInfo, targetNF, config)

	// Print header debug info once here
	fmt.Printf("🔍 DEBUG: Request headers:\n")
	fmt.Printf("🔍 DEBUG: Headers count: %d\n", len(execInfo.Headers))
	for key, value := range execInfo.Headers {
		fmt.Printf("🔍 DEBUG: Header[%s] = %s\n", key, value)
	}

	// Build and store final URL
	execInfo.DiscoveredURL = discoveredURL // 이 라인이 누락되었을 수 있음
	finalURL := e.requestBuilder.BuildFinalURL(execInfo)
	execInfo.FinalURL = finalURL
	fmt.Printf("🔗 Final URL: %s\n", finalURL)
	if !strings.HasPrefix(finalURL, "http://") && !strings.HasPrefix(finalURL, "https://") {
		return nil, fmt.Errorf("invalid URL format: %s", finalURL)
	}

	return execInfo, nil
}

// discoverNFURL discovers NF URL using NRF
func (e *APIExecutor) discoverNFURL(globalCfg map[string]interface{}, targetNF string) (string, error) {
	return BuildNFDiscoveryURL(globalCfg, targetNF)
}

// ExecuteHTTPCall performs the actual HTTP call
func (e *APIExecutor) ExecuteHTTPCall(execInfo *types.APIExecutionInfo) (time.Duration, error) {
	// Use the new HTTPClient to execute the request
	result := e.httpClient.ExecuteWithResult(execInfo, 0)

	if result.Error != nil {
		return 0, result.Error
	}

	return result.Duration, nil

}

// RunBenchmark runs benchmark for specified iterations or duration
func (e *APIExecutor) RunBenchmark(execInfo *types.APIExecutionInfo, iterations int, duration time.Duration, rateLimit int) (*types.BenchmarkResult, error) {
	fmt.Printf("🚀 Starting sequential benchmark:\n")
	if duration > 0 {
		fmt.Printf("   Duration: %v\n", duration)
	} else {
		fmt.Printf("   Iterations: %d\n", iterations)
	}
	if rateLimit > 0 {
		fmt.Printf("   Rate Limit: %d req/s\n", rateLimit)
	}

	// Rate limiter
	var rateLimiter <-chan time.Time
	if rateLimit > 0 {
		rateLimiter = time.Tick(time.Second / time.Duration(rateLimit))
	}

	var endTime time.Time
	if duration > 0 {
		endTime = time.Now().Add(duration)
	}

	var result types.BenchmarkResult
	var totalTime time.Duration
	var minTime, maxTime time.Duration

	requestCount := 0
	startTime := time.Now()

	for {
		// 종료 조건 체크
		if duration > 0 {
			if time.Now().After(endTime) {
				break
			}
		} else {
			if requestCount >= iterations {
				break
			}
		}

		// Rate limiting
		if rateLimiter != nil {
			<-rateLimiter
		}

		requestCount++

		// HTTP 요청 실행
		duration, err := e.ExecuteHTTPCall(execInfo)

		if err != nil {
			result.FailureCount++
			fmt.Printf("❌ Request %d failed: %v\n", requestCount, err)
		} else {
			result.SuccessCount++
			totalTime += duration
			fmt.Printf("✅ Request %d completed in %v\n", requestCount, duration)

			// Track min/max times
			if result.SuccessCount == 1 || duration < minTime {
				minTime = duration
			}
			if result.SuccessCount == 1 || duration > maxTime {
				maxTime = duration
			}
		}
	}

	actualDuration := time.Since(startTime)
	result.TotalRequests = requestCount
	result.TotalTime = actualDuration
	result.MinTime = minTime
	result.MaxTime = maxTime

	if result.SuccessCount > 0 {
		result.AvgTime = totalTime / time.Duration(result.SuccessCount)
	}

	result.RequestsPerSecond = float64(result.TotalRequests) / actualDuration.Seconds()

	return &result, nil
}

// RunConcurrentBenchmark runs benchmark with concurrent connections like wrk
func (e *APIExecutor) RunConcurrentBenchmark(execInfo *types.APIExecutionInfo, concurrency int, duration time.Duration, rateLimit int) (*types.BenchmarkResult, error) {
	fmt.Printf("🚀 Starting concurrent benchmark:\n")
	fmt.Printf("   Concurrent Connections: %d\n", concurrency)
	if duration > 0 {
		fmt.Printf("   Duration: %v\n", duration)
	}
	if rateLimit > 0 {
		fmt.Printf("   Rate Limit: %d req/s\n", rateLimit)
	}

	// Results collection
	results := make(chan *RequestResult, concurrency*100)
	var wg sync.WaitGroup

	// Rate limiter
	var rateLimiter <-chan time.Time
	if rateLimit > 0 {
		rateLimiter = time.Tick(time.Second / time.Duration(rateLimit))
	}

	// Start and end time
	startTime := time.Now()
	var endTime time.Time
	if duration > 0 {
		endTime = startTime.Add(duration)
	} else {
		endTime = startTime.Add(time.Hour) // Long enough time
	}

	// Worker pool
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// HTTP client per worker for connection reuse
			client := &http.Client{
				Timeout: e.Timeout,
				Transport: &http.Transport{
					MaxIdleConns:        10,
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     30 * time.Second,
				},
			}

			for time.Now().Before(endTime) {
				// Rate limiting
				if rateLimiter != nil {
					<-rateLimiter
				}

				result := e.executeRequestWithClient(execInfo, client, workerID)
				results <- result

				if time.Now().After(endTime) {
					break
				}
				fmt.Printf("%+v\n", result)
			}
		}(i)
	}

	// Wait for completion
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	return e.collectConcurrentResults(results, startTime)
}

// executeRequestWithClient executes HTTP request with provided client
func (e *APIExecutor) executeRequestWithClient(execInfo *types.APIExecutionInfo, client *http.Client, workerID int) *RequestResult {
	start := time.Now()

	// Use pre-built final URL from execInfo
	fullURL := execInfo.FinalURL

	var requestBody []byte
	if execInfo.RequestBody != nil {
		var err error
		requestBody, err = json.Marshal(execInfo.RequestBody)
		if err != nil {
			return &RequestResult{
				Duration:     0,
				StatusCode:   0,
				Error:        err,
				WorkerID:     workerID,
				Timestamp:    start,
				ResponseBody: "",
			}
		}
	}

	req, err := http.NewRequest(execInfo.Method, fullURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return &RequestResult{
			Duration:     0,
			StatusCode:   0,
			Error:        err,
			WorkerID:     workerID,
			Timestamp:    start,
			ResponseBody: "",
		}
	}

	for key, value := range execInfo.Headers {
		req.Header.Set(key, value)
	}

	httpStart := time.Now()
	resp, err := client.Do(req)
	httpDuration := time.Since(httpStart)

	if err != nil {
		return &RequestResult{
			Duration:     httpDuration,
			StatusCode:   0,
			Error:        err,
			WorkerID:     workerID,
			Timestamp:    start,
			ResponseBody: "",
		}
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, readErr := io.ReadAll(resp.Body)
	responseBody := ""
	if readErr == nil {
		responseBody = string(bodyBytes)
	}

	return &RequestResult{
		Duration:     httpDuration,
		StatusCode:   resp.StatusCode, // Status -> StatusCode
		Error:        readErr,
		WorkerID:     workerID,
		Timestamp:    start,
		ResponseBody: responseBody,
	}
}

// collectConcurrentResults processes concurrent benchmark results
func (e *APIExecutor) collectConcurrentResults(results <-chan *RequestResult, startTime time.Time) (*types.BenchmarkResult, error) {
	var allDurations []time.Duration
	var successCount, failureCount int
	var totalTime time.Duration
	var minTime, maxTime time.Duration
	errorDistribution := make(map[string]int)

	for result := range results {
		allDurations = append(allDurations, result.Duration)
		totalTime += result.Duration

		// Print response body for each worker result
		if result.ResponseBody != "" {
			fmt.Printf("Worker %d Response (Status: %d): %s\n", result.WorkerID, result.StatusCode, result.ResponseBody)
		}

		if result.Error != nil {
			failureCount++
			errorType := fmt.Sprintf("Error: %v", result.Error)
			errorDistribution[errorType]++
		} else if result.StatusCode >= 400 {
			failureCount++
			errorType := fmt.Sprintf("HTTP %d", result.StatusCode)
			errorDistribution[errorType]++
		} else {
			successCount++
		}

		if len(allDurations) == 1 || result.Duration < minTime {
			minTime = result.Duration
		}
		if len(allDurations) == 1 || result.Duration > maxTime {
			maxTime = result.Duration
		}
	}

	totalRequests := successCount + failureCount
	if totalRequests == 0 {
		return nil, fmt.Errorf("no requests completed")
	}

	actualDuration := time.Since(startTime)
	requestsPerSecond := float64(totalRequests) / actualDuration.Seconds()
	avgTime := totalTime / time.Duration(totalRequests)

	percentiles := calculatePercentiles(allDurations)

	return &types.BenchmarkResult{
		TotalRequests:     totalRequests,
		SuccessCount:      successCount,
		FailureCount:      failureCount,
		TotalTime:         actualDuration,
		AvgTime:           avgTime,
		MinTime:           minTime,
		MaxTime:           maxTime,
		RequestsPerSecond: requestsPerSecond,
		Percentiles:       percentiles,
		ErrorDistribution: errorDistribution,
	}, nil
}

// calculatePercentiles calculates response time percentiles
func calculatePercentiles(durations []time.Duration) map[string]time.Duration {
	if len(durations) == 0 {
		return nil
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	return map[string]time.Duration{
		"50th": durations[len(durations)*50/100],
		"75th": durations[len(durations)*75/100],
		"90th": durations[len(durations)*90/100],
		"95th": durations[len(durations)*95/100],
		"99th": durations[len(durations)*99/100],
	}
}
