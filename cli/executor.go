package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

	// Get global settings
	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

	var discoveredURL string

	// Skip NF Discovery for NRF - use NRF URL directly
	if strings.ToUpper(targetNF) == "NRF" {
		nrfURL, ok := getCfgString(globalSettings["nrf_url"])
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

	execInfo.DiscoveredURL = discoveredURL

	// Populate headers
	e.populateHeaders(execInfo, targetNF, config)

	// Build and display final URL once
	finalURL := e.buildFinalURL(execInfo)
	fmt.Printf("🔗 Final URL: %s\n", finalURL)

	return execInfo, nil
}

// buildFinalURL constructs the complete URL for the API call
func (e *APIExecutor) buildFinalURL(execInfo *types.APIExecutionInfo) string {
	// Replace path parameters in the URL
	finalPath := execInfo.Path
	queryParams := make(map[string]string)

	// Separate path and query parameters
	for paramName, paramValue := range execInfo.Parameters {
		if paramValue == "" {
			continue // Skip empty parameters
		}

		// Check if this is a path parameter
		placeholder := fmt.Sprintf("{%s}", paramName)
		if strings.Contains(finalPath, placeholder) {
			// Path parameter - replace in URL path
			finalPath = strings.ReplaceAll(finalPath, placeholder, paramValue)
		} else {
			// Query parameter - add to query string
			queryParams[paramName] = paramValue
		}
	}

	// Get service path from api_list.yaml
	servicePath := e.getServicePath(execInfo.NF, execInfo.APIName)

	// Build base URL: NF Discovery URL + Service Path + API Path
	baseURL := strings.TrimSuffix(execInfo.DiscoveredURL, "/")
	apiPath := strings.TrimPrefix(finalPath, "/")
	fullURL := fmt.Sprintf("%s%s/%s", baseURL, servicePath, apiPath)

	// Add query parameters if any
	if len(queryParams) > 0 {
		queryValues := url.Values{}
		for key, value := range queryParams {
			queryValues.Set(key, value)
		}
		fullURL += "?" + queryValues.Encode()
	}

	return fullURL
}

// discoverNFURL discovers NF URL using NRF
func (e *APIExecutor) discoverNFURL(globalCfg map[string]interface{}, targetNF string) (string, error) {
	return NFDiscoveryURL(globalCfg, targetNF)
}

// loadConfiguration loads configuration.yaml
func (e *APIExecutor) loadConfiguration() (map[string]interface{}, error) {
	data, err := os.ReadFile("configuration.yaml")
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
		fmt.Printf("🔍 DEBUG: No user_inputs found in configuration\n")
		return
	}

	nfSettings, ok := userInputs["nf_settings"].(map[string]interface{})
	if !ok {
		fmt.Printf("🔍 DEBUG: No nf_settings found in configuration\n")
		return
	}

	fmt.Printf("🔍 DEBUG: Looking for NF settings for: %s\n", targetNF)

	if nfConfig, exists := nfSettings[targetNF]; exists {
		fmt.Printf("🔍 DEBUG: Found NF config for %s\n", targetNF)

		if nfMap, ok := nfConfig.(map[string]interface{}); ok {
			if customHeaders, exists := nfMap["custom_headers"]; exists {
				fmt.Printf("🔍 DEBUG: Found custom_headers section\n")

				if headersMap, ok := customHeaders.(map[string]interface{}); ok {
					for key, value := range headersMap {
						var headerValue string

						// Handle new configuration format: {value: "..."}
						if valueMap, ok := value.(map[string]interface{}); ok {
							if val, exists := valueMap["value"]; exists {
								headerValue = fmt.Sprintf("%v", val)
							}
						} else {
							// Handle direct string value
							headerValue = fmt.Sprintf("%v", value)
						}

						if headerValue != "" {
							fmt.Printf("🔍 DEBUG: Setting header %s: %s\n", key, headerValue)
							execInfo.Headers[key] = headerValue
						} else {
							fmt.Printf("🔍 DEBUG: Skipping empty header: %s\n", key)
						}
					}
				} else {
					fmt.Printf("🔍 DEBUG: custom_headers is not a map\n")
				}
			} else {
				fmt.Printf("🔍 DEBUG: No custom_headers found for %s\n", targetNF)
			}
		} else {
			fmt.Printf("🔍 DEBUG: NF config is not a map\n")
		}
	} else {
		fmt.Printf("🔍 DEBUG: No configuration found for NF: %s\n", targetNF)
	}

	fmt.Printf("🔍 DEBUG: Final headers: %v\n", execInfo.Headers)
}

// ExecuteHTTPCall performs the actual HTTP call
func (e *APIExecutor) ExecuteHTTPCall(execInfo *types.APIExecutionInfo) (time.Duration, error) {
	start := time.Now()

	// Build full URL using the same logic as buildFinalURL
	fullURL := e.buildFinalURL(execInfo)

	// Prepare request body
	var requestBody []byte
	if execInfo.RequestBody != nil {
		var err error
		requestBody, err = json.Marshal(execInfo.RequestBody)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal request body: %w", err)
		}
		fmt.Printf("🔍 DEBUG: Request Body: %s\n", string(requestBody))
	} else {
		fmt.Printf("🔍 DEBUG: No request body\n")
	}

	// Create HTTP request
	req, err := http.NewRequest(execInfo.Method, fullURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers from execInfo
	fmt.Printf("🔍 DEBUG: Adding headers to request:\n")
	for key, value := range execInfo.Headers {
		req.Header.Set(key, value)
		fmt.Printf("   %s: %s\n", key, value)
	}

	// Debug: Print all request headers (including any defaults added by Go)
	fmt.Printf("🔍 DEBUG: Final request headers:\n")
	for key, values := range req.Header {
		for _, value := range values {
			fmt.Printf("   %s: %s\n", key, value)
		}
	}

	fmt.Printf("🔍 DEBUG: Making %s request to: %s\n", execInfo.Method, fullURL)

	// Execute request
	client := &http.Client{Timeout: e.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("🔍 DEBUG: Request failed with error: %v\n", err)
		return time.Since(start), fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	// Debug: Print response status and headers
	fmt.Printf("🔍 DEBUG: Response Status: %s (%d)\n", resp.Status, resp.StatusCode)
	fmt.Printf("🔍 DEBUG: Response Headers:\n")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("   %s: %s\n", key, value)
		}
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("🔍 DEBUG: Failed to read response body: %v\n", err)
		return duration, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: Print response body
	if len(body) > 0 {
		fmt.Printf("🔍 DEBUG: Response Body (raw): %s\n", string(body))

		// Try to format JSON response for better readability
		var jsonData interface{}
		if err := json.Unmarshal(body, &jsonData); err == nil {
			if prettyJSON, err := json.MarshalIndent(jsonData, "", "  "); err == nil {
				fmt.Printf("🔍 DEBUG: Response Body (formatted):\n%s\n", string(prettyJSON))
			}
		} else {
			fmt.Printf("🔍 DEBUG: Response is not valid JSON: %v\n", err)
		}
	} else {
		fmt.Printf("🔍 DEBUG: Empty response body\n")
	}

	// Check response status
	if resp.StatusCode >= 400 {
		fmt.Printf("🔍 DEBUG: HTTP error detected - Status: %d\n", resp.StatusCode)
		return duration, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("🔍 DEBUG: Request completed successfully in %v\n", duration)
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
