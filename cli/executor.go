package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/types"

	"gopkg.in/yaml.v3"
)

// APIExecutor handles API execution and benchmarking
type APIExecutor struct {
	Timeout time.Duration
}

// NewAPIExecutor creates a new API executor
func NewAPIExecutor(timeout time.Duration) *APIExecutor {
	return &APIExecutor{
		Timeout: timeout,
	}
}

// ExecuteAPI executes a specific API call using api_list.yaml
func (e *APIExecutor) ExecuteAPI(targetNF, apiName string) (*types.APIExecutionInfo, error) {
	// Load API list
	apiList, err := LoadAPIList()
	if err != nil {
		return nil, fmt.Errorf("failed to load API list: %w", err)
	}

	// Load configuration
	config, err := e.loadConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Prepare execution info from api_list and configuration (with required validation)
	execInfo, err := PrepareAPIExecution(apiList, config, targetNF, apiName)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare API execution: %w", err)
	}

	// Get global settings for NF discovery
	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

	// Discover NF URL
	discoveredURL, err := e.discoverNFURL(globalSettings, targetNF)
	if err != nil {
		return nil, fmt.Errorf("NF discovery failed: %w", err)
	}
	// for testing purposes, replace discovered URL
	if discoveredURL == "http://controlplane-free5gc-ausf-service:80" {
		discoveredURL = "http://10.96.43.148:80"
	}
	fmt.Printf("✅ Discovered %s URL: %s\n", targetNF, discoveredURL)
	execInfo.DiscoveredURL = discoveredURL

	// Populate headers
	e.populateHeaders(execInfo, targetNF, config)

	return execInfo, nil
}

// discoverNFURL discovers NF URL using NRF
func (e *APIExecutor) discoverNFURL(globalCfg map[string]interface{}, targetNF string) (string, error) {
	return NFDiscoveryURL(globalCfg, targetNF)
}

// loadConfiguration loads configuration.yaml
func (e *APIExecutor) loadConfiguration() (map[string]interface{}, error) {
	data, err := ioutil.ReadFile("configuration.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration.yaml: %w", err)
	}

	var config map[string]interface{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration.yaml: %w", err)
	}

	return config, nil
}

// populateHeaders populates HTTP headers for the request
func (e *APIExecutor) populateHeaders(execInfo *types.APIExecutionInfo, targetNF string, config map[string]interface{}) {
	// Set default headers
	execInfo.Headers["Content-Type"] = "application/json"
	execInfo.Headers["Accept"] = "application/json"

	// Add NF-specific headers from configuration
	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		return
	}

	nfSettings, ok := userInputs["nf_settings"].(map[string]interface{})
	if !ok {
		return
	}

	if nfConfig, exists := nfSettings[targetNF]; exists {
		if nfMap, ok := nfConfig.(map[string]interface{}); ok {
			if customHeaders, exists := nfMap["custom_headers"]; exists {
				if headersMap, ok := customHeaders.(map[string]interface{}); ok {
					for key, value := range headersMap {
						var headerValue string
						if valueMap, ok := value.(map[string]interface{}); ok {
							if val, exists := valueMap["value"]; exists {
								headerValue = fmt.Sprintf("%v", val)
							}
						} else {
							headerValue = fmt.Sprintf("%v", value)
						}

						if headerValue != "" {
							execInfo.Headers[key] = headerValue
						}
					}
				}
			}
		}
	}
}

// ExecuteHTTPCall performs the actual HTTP call
func (e *APIExecutor) ExecuteHTTPCall(execInfo *types.APIExecutionInfo) (time.Duration, error) {
	start := time.Now()

	// Replace path parameters in the URL
	finalPath := execInfo.Path
	for paramName, paramValue := range execInfo.Parameters {
		placeholder := fmt.Sprintf("{%s}", paramName)
		finalPath = strings.ReplaceAll(finalPath, placeholder, paramValue)
	}

	// Get service path from api_list.yaml
	servicePath := e.getServicePath(execInfo.NF, execInfo.APIName)

	// Build full URL: NF Discovery URL + Service Path + API Path
	baseURL := strings.TrimSuffix(execInfo.DiscoveredURL, "/")
	apiPath := strings.TrimPrefix(finalPath, "/")

	fullURL := fmt.Sprintf("%s%s/%s", baseURL, servicePath, apiPath)

	fmt.Printf("🔗 Final URL: %s\n", fullURL)

	// Prepare request body
	var requestBody []byte
	if execInfo.RequestBody != nil {
		var err error
		requestBody, err = json.Marshal(execInfo.RequestBody)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Create HTTP request
	req, err := http.NewRequest(execInfo.Method, fullURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range execInfo.Headers {
		req.Header.Set(key, value)
	}

	// Execute request
	client := &http.Client{Timeout: e.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return time.Since(start), fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Check response status
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return duration, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return duration, nil
}

// getServicePath retrieves service path from api_list.yaml
func (e *APIExecutor) getServicePath(nf, apiName string) string {
	apiList, err := LoadAPIList()
	if err != nil {
		fmt.Printf("⚠️  Failed to load API list: %v\n", err)
		return ""
	}

	if nfServices, exists := apiList[nf]; exists {
		for _, serviceInfo := range nfServices {
			if _, exists := serviceInfo.APIs[apiName]; exists {
				return serviceInfo.Path
			}
		}
	}

	// Fallback: generate default service path
	nfLower := strings.ToLower(nf)
	return fmt.Sprintf("/n%s-auth/v1", nfLower)
}

// RunBenchmark runs benchmark for specified iterations
func (e *APIExecutor) RunBenchmark(execInfo *types.APIExecutionInfo, iterations int) (*types.BenchmarkResult, error) {
	result := &types.BenchmarkResult{
		TotalRequests: iterations,
	}

	var totalTime time.Duration
	var minTime, maxTime time.Duration

	for i := 1; i <= iterations; i++ {
		duration, err := e.ExecuteHTTPCall(execInfo)
		totalTime += duration

		if err != nil {
			result.FailureCount++
			fmt.Printf("❌ Request %d failed: %v\n", i, err)
		} else {
			result.SuccessCount++
			fmt.Printf("✅ Request %d completed in %v\n", i, duration)
		}

		// Track min/max times
		if i == 1 || duration < minTime {
			minTime = duration
		}
		if i == 1 || duration > maxTime {
			maxTime = duration
		}
	}

	result.TotalTime = totalTime
	result.MinTime = minTime
	result.MaxTime = maxTime

	if result.SuccessCount > 0 {
		result.AvgTime = totalTime / time.Duration(result.SuccessCount)
	}

	return result, nil
}
