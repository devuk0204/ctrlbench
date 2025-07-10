package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
		fmt.Printf("Using direct NRF URL: %s\n", discoveredURL)
	} else {
		// Discover NF URL for other NFs
		var err error
		discoveredURL, err = e.discoverNFURL(globalSettings, targetNF)
		if err != nil {
			return nil, fmt.Errorf("NF discovery failed: %w", err)
		}

		fmt.Printf("Discovered %s URL: %s\n", targetNF, discoveredURL)
	}

	e.requestBuilder.PopulateHeaders(execInfo, targetNF, config)

	// Build and store final URL
	execInfo.DiscoveredURL = discoveredURL
	finalURL := e.requestBuilder.BuildFinalURL(execInfo)
	execInfo.FinalURL = finalURL
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

// RunBenchmark with improved caching - PrepareAPIExecution called only once
func (e *APIExecutor) RunBenchmark(execInfo *types.APIExecutionInfo, iterations int, duration time.Duration, rateLimit int) (*types.BenchmarkResult, error) {
	fmt.Printf("Starting sequential benchmark:\n")
	if duration > 0 {
		fmt.Printf("   Duration: %v\n", duration)
	} else {
		fmt.Printf("   Iterations: %d\n", iterations)
	}
	if rateLimit > 0 {
		fmt.Printf("   Rate Limit: %d req/s\n", rateLimit)
	}

	config, err := LoadConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Get global settings for NF discovery
	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

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
	var allDurations []time.Duration
	var totalTime time.Duration
	var minTime, maxTime time.Duration
	var requestCount int
	var successCount, failureCount int
	errorDistribution := make(map[string]int)

	// Benchmark start time
	benchmarkStartTime := time.Now()

	currentExecInfo := execInfo // 이미 준비된 execInfo 사용

	if currentExecInfo.DiscoveredURL == "" {
		if strings.ToUpper(execInfo.NF) == "NRF" {
			nrfURL, ok := GetConfigString(globalSettings["nrf_url"])
			if !ok || nrfURL == "" {
				return nil, fmt.Errorf("NRF URL is missing")
			}
			currentExecInfo.DiscoveredURL = nrfURL
		} else {
			discoveredURL, err := e.discoverNFURL(globalSettings, execInfo.NF)
			if err != nil {
				return nil, fmt.Errorf("discovery failed: %w", err)
			}
			fmt.Printf("Discovered %s URL: %s\n", execInfo.NF, discoveredURL)
			currentExecInfo.DiscoveredURL = discoveredURL
		}
	}

	e.requestBuilder.PopulateHeaders(currentExecInfo, execInfo.NF, config)
	for key, value := range currentExecInfo.Headers {
		fmt.Printf("Header[%s] = %s\n", key, value)
	}

	currentExecInfo.FinalURL = e.requestBuilder.BuildFinalURL(currentExecInfo)

	fmt.Printf("Final URL: %s\n", currentExecInfo.FinalURL)
	fmt.Printf("Execution Details:\n")
	fmt.Printf("   NF: %s\n", currentExecInfo.NF)
	fmt.Printf("   API: %s\n", currentExecInfo.APIName)
	fmt.Printf("   Method: %s\n", currentExecInfo.Method)
	fmt.Printf("   Path: %s\n", currentExecInfo.Path)
	fmt.Printf("   Discovered URL: %s\n", currentExecInfo.DiscoveredURL)
	fmt.Printf("   Parameters: %v\n", currentExecInfo.Parameters)

	// 요청 본문 출력
	if reqBody, ok := currentExecInfo.RequestBody.(map[string]interface{}); ok {
		bodyBytes, _ := json.Marshal(reqBody)
		fmt.Printf("Request Body: %s\n", string(bodyBytes))
	} else if currentExecInfo.RequestBody != nil {
		fmt.Printf("Request Body: %v\n", currentExecInfo.RequestBody)
	}
	fmt.Printf("\n")

	for {
		// Rate limiting
		if rateLimiter != nil {
			<-rateLimiter
		}

		// Check termination conditions
		if duration > 0 {
			if time.Now().After(endTime) {
				break
			}
		} else {
			if requestCount >= iterations {
				break
			}
		}

		requestCount++

		execDuration, err := e.ExecuteHTTPCall(currentExecInfo)
		allDurations = append(allDurations, execDuration)
		totalTime += execDuration

		// Track min/max times
		if requestCount == 1 || execDuration < minTime {
			minTime = execDuration
		}
		if requestCount == 1 || execDuration > maxTime {
			maxTime = execDuration
		}

		// Track success/failure and errors
		if err != nil {
			failureCount++
			errorType := fmt.Sprintf("Error: %v", err)
			errorDistribution[errorType]++
			fmt.Printf("Request %d failed: %v\n", requestCount, err)
			os.Exit(1)
		} else {
			successCount++
			fmt.Printf("Request %d completed in %v\n", requestCount, execDuration)
		}
	}

	actualDuration := time.Since(benchmarkStartTime)
	requestsPerSecond := float64(requestCount) / actualDuration.Seconds()

	var avgTime time.Duration
	if requestCount > 0 {
		avgTime = totalTime / time.Duration(requestCount)
	}

	trimmedMeans := calculateTrimmedMeans(allDurations)

	result = types.BenchmarkResult{
		TotalRequests:     requestCount,
		SuccessCount:      successCount,
		FailureCount:      failureCount,
		TotalTime:         actualDuration,
		AvgTime:           avgTime,
		MinTime:           minTime,
		MaxTime:           maxTime,
		RequestsPerSecond: requestsPerSecond,
		Percentiles:       nil,
		TrimmedMeans:      trimmedMeans,
		ErrorDistribution: errorDistribution,
	}

	return &result, nil
}

// RunConcurrentBenchmark with before_each_call support - each worker prepares its own execution
func (e *APIExecutor) RunConcurrentBenchmark(execInfo *types.APIExecutionInfo, concurrency int, duration time.Duration, rateLimit int) (*types.BenchmarkResult, error) {
	fmt.Printf("Starting concurrent benchmark:\n")
	fmt.Printf("Concurrent Connections: %d\n", concurrency)
	if duration > 0 {
		fmt.Printf("Duration: %v\n", duration)
	}
	if rateLimit > 0 {
		fmt.Printf("Rate Limit: %d req/s\n", rateLimit)
	}

	config, err := LoadConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

	for key, value := range execInfo.Headers {
		fmt.Printf("Header[%s] = %s\n", key, value)
	}
	fmt.Printf("Final URL: %s\n", execInfo.FinalURL)
	fmt.Printf("Execution Details:\n")
	fmt.Printf("   NF: %s\n", execInfo.NF)
	fmt.Printf("   API: %s\n", execInfo.APIName)
	fmt.Printf("   Method: %s\n", execInfo.Method)
	fmt.Printf("   Path: %s\n", execInfo.Path)
	fmt.Printf("   Discovered URL: %s\n", execInfo.DiscoveredURL)
	fmt.Printf("   Parameters: %v\n", execInfo.Parameters)
	if reqBody, ok := execInfo.RequestBody.(map[string]interface{}); ok {
		bodyBytes, _ := json.Marshal(reqBody)
		fmt.Printf("Request Body: %s\n", string(bodyBytes))
	} else if execInfo.RequestBody != nil {
		fmt.Printf("Request Body: %v\n", execInfo.RequestBody)
	}
	fmt.Printf("\n")

	// Results collection
	results := make(chan *RequestResult, concurrency*100)
	var wg sync.WaitGroup

	// Rate limiter
	var rateLimiter <-chan time.Time
	if rateLimit > 0 {
		rateLimiter = time.Tick(time.Second / time.Duration(rateLimit))
	}

	// Start and end time
	benchmarkStartTime := time.Now()
	var endTime time.Time
	if duration > 0 {
		endTime = benchmarkStartTime.Add(duration)
	} else {
		endTime = benchmarkStartTime.Add(time.Hour)
	}

	// Worker pool
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			client := &http.Client{
				Timeout: e.Timeout,
				Transport: &http.Transport{
					MaxIdleConns:        10,
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     30 * time.Second,
				},
			}

			for time.Now().Before(endTime) {
				if rateLimiter != nil {
					<-rateLimiter
				}

				currentExecInfo := execInfo

				// Setup URL and headers for current request
				var discoveredURL string
				if strings.ToUpper(execInfo.NF) == "NRF" {
					nrfURL, ok := GetConfigString(globalSettings["nrf_url"])
					if !ok || nrfURL == "" {
						results <- &RequestResult{
							Duration:     0,
							StatusCode:   0,
							Error:        fmt.Errorf("NRF URL missing"),
							WorkerID:     workerID,
							Timestamp:    time.Now(),
							ResponseBody: "",
						}
						continue
					}
					discoveredURL = nrfURL
				} else {
					discoveredURL, err = e.discoverNFURL(globalSettings, execInfo.NF)
					if err != nil {
						results <- &RequestResult{
							Duration:     0,
							StatusCode:   0,
							Error:        fmt.Errorf("discovery failed: %w", err),
							WorkerID:     workerID,
							Timestamp:    time.Now(),
							ResponseBody: "",
						}
						continue
					}
				}

				currentExecInfo.DiscoveredURL = discoveredURL
				e.requestBuilder.PopulateHeaders(currentExecInfo, execInfo.NF, config)
				currentExecInfo.FinalURL = e.requestBuilder.BuildFinalURL(currentExecInfo)

				// Execute request
				result := e.executeRequestWithClient(currentExecInfo, client, workerID)
				results <- result

				if time.Now().After(endTime) {
					break
				}

				if result.Error != nil {
					fmt.Printf("Worker %d: %v\n", result.WorkerID, result.Error)
				} else {
					fmt.Printf("Worker %d: %d (%v)\n", result.WorkerID, result.StatusCode, result.Duration)
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	benchmarkResult, err := e.collectConcurrentResults(results, benchmarkStartTime)
	if err != nil {
		return nil, err
	}

	return benchmarkResult, nil
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

	// Add headers
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
		StatusCode:   resp.StatusCode,
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
	trimmedMeans := calculateTrimmedMeans(allDurations)

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
		TrimmedMeans:      trimmedMeans,
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

// calculateTrimmedMeans calculates trimmed means by removing outliers
func calculateTrimmedMeans(durations []time.Duration) map[string]time.Duration {
	if len(durations) == 0 {
		return nil
	}

	trimmedMeans := make(map[string]time.Duration)

	// Sort durations for trimmed mean calculation
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate trimmed means
	trimmedMeans["90%"] = calculateTrimmedMean(durations, 0.90)
	trimmedMeans["95%"] = calculateTrimmedMean(durations, 0.95)
	trimmedMeans["99%"] = calculateTrimmedMean(durations, 0.99)

	return trimmedMeans
}

// calculateTrimmedMean 함수에 디버그 추가
func calculateTrimmedMean(sortedDurations []time.Duration, percentage float64) time.Duration {
	n := len(sortedDurations)
	if n == 0 {
		return 0
	}

	// Calculate how many values to trim from each end
	trimCount := int(float64(n) * (1.0 - percentage) / 2.0)

	// Ensure we don't trim everything
	if trimCount >= n/2 {
		trimCount = n / 4
	}

	start := trimCount
	end := n - trimCount

	if start >= end {
		return calculateAverage(sortedDurations)
	}

	var sum time.Duration
	count := 0
	for i := start; i < end; i++ {
		sum += sortedDurations[i]
		count++
	}

	if count == 0 {
		return calculateAverage(sortedDurations)
	}

	result := sum / time.Duration(count)
	return result
}

// calculateAverage calculates simple average of durations
func calculateAverage(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}

	return sum / time.Duration(len(durations))
}
