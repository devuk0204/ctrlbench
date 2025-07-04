package types

import "time"

// ServiceMetadata represents metadata for a service
type ServiceMetadata struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	APIs        map[string]APIMetadata `json:"apis"`
	NF          string                 `json:"nf"`
	OpenAPISpec *OpenAPISpec           `json:"-"`
}

// APIMetadata represents metadata for an API with execution details
type APIMetadata struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Methods           []string               `json:"methods"`
	Path              string                 `json:"path"`
	Parameters        []string               `json:"parameters"`
	RequestBody       string                 `json:"request_body"`
	RequestBodySchema map[string]interface{} `json:"request_body_schema,omitempty"`
}

// BenchmarkResult represents the result of a benchmark run
type BenchmarkResult struct {
	TotalRequests int           `json:"total_requests"`
	SuccessCount  int           `json:"success_count"`
	FailureCount  int           `json:"failure_count"`
	TotalTime     time.Duration `json:"total_time"`
	AvgTime       time.Duration `json:"avg_time"`
	MinTime       time.Duration `json:"min_time"`
	MaxTime       time.Duration `json:"max_time"`
}

// APIExecutionInfo contains all information needed to execute an API call
type APIExecutionInfo struct {
	NF            string            `json:"nf"`
	APIName       string            `json:"api_name"`
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	DiscoveredURL string            `json:"discovered_url"`
	Parameters    map[string]string `json:"parameters"`
	RequestBody   interface{}       `json:"request_body"`
	Headers       map[string]string `json:"headers"`
}
