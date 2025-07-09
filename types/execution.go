package types

import "time"

// BenchmarkResult represents benchmark execution results
type BenchmarkResult struct {
	TotalRequests     int                      `json:"total_requests"`
	SuccessCount      int                      `json:"success_count"`
	FailureCount      int                      `json:"failure_count"`
	TotalTime         time.Duration            `json:"total_time"`
	AvgTime           time.Duration            `json:"avg_time"`
	MinTime           time.Duration            `json:"min_time"`
	MaxTime           time.Duration            `json:"max_time"`
	RequestsPerSecond float64                  `json:"requests_per_second"`
	Percentiles       map[string]time.Duration `json:"percentiles,omitempty"`
	TrimmedMeans      map[string]time.Duration `json:"trimmed_means,omitempty"` // 추가
	ErrorDistribution map[string]int           `json:"error_distribution,omitempty"`
}

// ConcurrentConfig represents concurrent benchmark configuration
type ConcurrentConfig struct {
	Concurrency int
	Duration    time.Duration
	RateLimit   int
}

// APIExecutionInfo contains all information needed to execute an API call
type APIExecutionInfo struct {
	NF            string            `json:"nf"`
	APIName       string            `json:"api_name"`
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	DiscoveredURL string            `json:"discovered_url"`
	FinalURL      string            `json:"final_url"`
	Parameters    map[string]string `json:"parameters"`
	RequestBody   interface{}       `json:"request_body"`
	Headers       map[string]string `json:"headers"`
}

// ExecutionResult represents the result of an API execution
type ExecutionResult struct {
	Success     bool                   `json:"success"`
	StatusCode  int                    `json:"status_code"`
	Response    map[string]interface{} `json:"response,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Latency     time.Duration          `json:"latency"`
	Timestamp   time.Time              `json:"timestamp"`
	RequestInfo APIExecutionInfo       `json:"request_info"`
}

// HTTPResult represents the result of an HTTP request execution
type HTTPResult struct {
	Duration     time.Duration
	StatusCode   int
	ResponseBody string
	Error        error
}

// APIChainConfig represents API chain configuration
type APIChainConfig struct {
	Enabled         bool                `yaml:"enabled"`
	PrerequisiteAPI string              `yaml:"prerequisite_api"` // Format: NF_API
	ChainType       string              `yaml:"chain_type"`       // "once_before_benchmark" or "before_each_call"
	ResponseMapping *APIResponseMapping `yaml:"response_mapping,omitempty"`
	MainNF          string              `yaml:"main_nf,omitempty"`         // Main API's NF
	PrerequisiteNF  string              `yaml:"prerequisite_nf,omitempty"` // Prerequisite API's NF
}

// APIResponseMapping defines how to map prerequisite API response to main API
type APIResponseMapping struct {
	Parameters  map[string]string `yaml:"parameters"`   // "paramName": "$.response.field"
	Headers     map[string]string `yaml:"headers"`      // "headerName": "$.response.field"
	RequestBody map[string]string `yaml:"request_body"` // "bodyField": "$.response.field"
}

// ChainExecutionResult stores prerequisite API execution result
type ChainExecutionResult struct {
	ResponseBody  string
	StatusCode    int
	Duration      time.Duration
	Error         error
	ExtractedData map[string]interface{} // Extracted values from response
}
