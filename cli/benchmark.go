package cli

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/devuk0204/ctrlbench/types"
)

// BenchmarkRunner handles benchmark execution
type BenchmarkRunner struct {
	httpClient *HTTPClient
}

// NewBenchmarkRunner creates a new benchmark runner
func NewBenchmarkRunner() *BenchmarkRunner {
	return &BenchmarkRunner{
		httpClient: NewHTTPClient(),
	}
}

// RunSequentialBenchmark runs benchmark with sequential requests
func (b *BenchmarkRunner) RunSequentialBenchmark(execInfo *types.APIExecutionInfo, iterations int) (*types.BenchmarkResult, error) {
	fmt.Printf("Starting sequential benchmark with %d iterations...\n", iterations)
	fmt.Printf("Target: %s\n", execInfo.DiscoveredURL+execInfo.Path)
	fmt.Printf("Method: %s\n", execInfo.Method)
	fmt.Println()

	var durations []time.Duration
	var successCount, failureCount int
	errorDistribution := make(map[string]int)

	for i := 0; i < iterations; i++ {
		// execInfo는 이미 완전히 세팅된 상태여야 함
		result := b.httpClient.ExecuteWithResult(execInfo, i+1)
		durations = append(durations, result.Duration)

		if result.Error != nil {
			failureCount++
			errorDistribution[result.Error.Error()]++
			fmt.Printf("Request %d failed: %v\n", i+1, result.Error)
		} else if result.StatusCode >= 200 && result.StatusCode < 300 {
			successCount++
			fmt.Printf("Request %d: %d - %v\n", i+1, result.StatusCode, result.Duration)
		} else {
			failureCount++
			errorMsg := fmt.Sprintf("HTTP %d", result.StatusCode)
			errorDistribution[errorMsg]++
			fmt.Printf("Request %d: %d - %v\n", i+1, result.StatusCode, result.Duration)
		}
	}

	return b.calculateResults(durations, successCount, failureCount, errorDistribution), nil
}

// RunConcurrentBenchmark runs benchmark with concurrent connections
func (b *BenchmarkRunner) RunConcurrentBenchmark(execInfo *types.APIExecutionInfo, concurrency int, duration time.Duration, rateLimit int) (*types.BenchmarkResult, error) {
	fmt.Printf("Starting concurrent benchmark:\n")
	fmt.Printf("Target: %s\n", execInfo.DiscoveredURL+execInfo.Path)
	fmt.Printf("Method: %s\n", execInfo.Method)
	fmt.Printf("Concurrency: %d\n", concurrency)
	fmt.Printf("Duration: %v\n", duration)
	if rateLimit > 0 {
		fmt.Printf("Rate Limit: %d req/sec\n", rateLimit)
	}
	fmt.Println()

	results := make(chan *RequestResult, concurrency*100)
	var wg sync.WaitGroup

	// Rate limiting
	var ticker *time.Ticker
	if rateLimit > 0 {
		interval := time.Second / time.Duration(rateLimit)
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	}

	startTime := time.Now()
	endTime := startTime.Add(duration)

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for time.Now().Before(endTime) {
				// Rate limiting
				if ticker != nil {
					<-ticker.C
				}

				httpResult := b.httpClient.ExecuteWithResult(execInfo, workerID)
				// Convert *HTTPResult to *RequestResult
				requestResult := &RequestResult{
					Duration:   httpResult.Duration,
					Error:      httpResult.Error,
					StatusCode: httpResult.StatusCode,
				}
				results <- requestResult
			}
		}(i)
	}

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	return b.collectConcurrentResults(results, startTime)
}

// calculateResults calculates benchmark statistics
func (b *BenchmarkRunner) calculateResults(durations []time.Duration, successCount, failureCount int, errorDistribution map[string]int) *types.BenchmarkResult {
	if len(durations) == 0 {
		return &types.BenchmarkResult{
			TotalRequests:     0,
			SuccessCount:      successCount,
			FailureCount:      failureCount,
			ErrorDistribution: errorDistribution,
		}
	}

	// Sort durations for percentile calculation
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate total time and average
	var totalTime time.Duration
	for _, d := range durations {
		totalTime += d
	}
	avgTime := totalTime / time.Duration(len(durations))

	// Calculate percentiles
	percentiles := calculatePercentiles(durations)

	return &types.BenchmarkResult{
		TotalRequests:     len(durations),
		SuccessCount:      successCount,
		FailureCount:      failureCount,
		TotalTime:         totalTime,
		AvgTime:           avgTime,
		MinTime:           durations[0],
		MaxTime:           durations[len(durations)-1],
		RequestsPerSecond: float64(len(durations)) / totalTime.Seconds(),
		Percentiles:       percentiles,
		ErrorDistribution: errorDistribution,
	}
}

// collectConcurrentResults collects and processes concurrent benchmark results
func (b *BenchmarkRunner) collectConcurrentResults(results <-chan *RequestResult, startTime time.Time) (*types.BenchmarkResult, error) {
	var durations []time.Duration
	var successCount, failureCount int
	errorDistribution := make(map[string]int)
	totalRequests := 0

	for result := range results {
		totalRequests++
		durations = append(durations, result.Duration)

		if result.Error != nil {
			failureCount++
			errorDistribution[result.Error.Error()]++
		} else if result.StatusCode >= 200 && result.StatusCode < 300 {
			successCount++
		} else {
			failureCount++
			errorMsg := fmt.Sprintf("HTTP %d", result.StatusCode)
			errorDistribution[errorMsg]++
		}

		// Progress indicator
		if totalRequests%100 == 0 {
			elapsed := time.Since(startTime)
			rps := float64(totalRequests) / elapsed.Seconds()
			fmt.Printf("Progress: %d requests, %.1f req/sec\n", totalRequests, rps)
		}
	}

	elapsed := time.Since(startTime)

	if len(durations) == 0 {
		return &types.BenchmarkResult{
			TotalRequests:     totalRequests,
			SuccessCount:      successCount,
			FailureCount:      failureCount,
			TotalTime:         elapsed,
			ErrorDistribution: errorDistribution,
		}, nil
	}

	// Sort durations for percentile calculation
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})

	// Calculate average
	var totalDuration time.Duration
	for _, d := range durations {
		totalDuration += d
	}
	avgTime := totalDuration / time.Duration(len(durations))

	// Calculate percentiles
	percentiles := calculatePercentiles(durations)

	return &types.BenchmarkResult{
		TotalRequests:     totalRequests,
		SuccessCount:      successCount,
		FailureCount:      failureCount,
		TotalTime:         elapsed,
		AvgTime:           avgTime,
		MinTime:           durations[0],
		MaxTime:           durations[len(durations)-1],
		RequestsPerSecond: float64(totalRequests) / elapsed.Seconds(),
		Percentiles:       percentiles,
		ErrorDistribution: errorDistribution,
	}, nil
}
