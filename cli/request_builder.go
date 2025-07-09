package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/calculate"
	"github.com/devuk0204/ctrlbench/types"
	"gopkg.in/yaml.v3"
)

// RequestBuilder handles API request construction
type RequestBuilder struct{}

var (
	// Global cache for once_before_benchmark chains
	onceBenchmarkCache = make(map[string]*types.ChainExecutionResult)
)

// NewRequestBuilder creates a new request builder
func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{}
}

// PrepareAPIExecution prepares API execution info from configuration with detailed validation
func (rb *RequestBuilder) PrepareAPIExecution(apiList types.APIList, config map[string]interface{}, nf, apiName string) (*types.APIExecutionInfo, error) {
	return rb.prepareAPIExecutionWithChainFlag(apiList, config, nf, apiName, false)
}

// Internal method with chain prevention flag
func (rb *RequestBuilder) prepareAPIExecutionWithChainFlag(apiList types.APIList, config map[string]interface{}, nf, apiName string, skipChain bool) (*types.APIExecutionInfo, error) {
	fmt.Printf("🔍 DEBUG: Starting PrepareAPIExecution for NF=%s, API=%s, skipChain=%v\n", nf, apiName, skipChain)

	apiInfo, servicePath, err := rb.GetAPIInfoWithServicePath(apiList, nf, apiName)
	if err != nil {
		return nil, err
	}

	fmt.Printf("🔍 DEBUG: Found API info - Service path: %s, API path: %s\n", servicePath, apiInfo.Path)

	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("user_inputs not found in configuration")
	}

	var chainConfig *types.APIChainConfig
	var chainResult *types.ChainExecutionResult

	// Check if this API has chain configuration ONLY if not skipping chain
	if !skipChain {
		chainConfig, err = GetAPIChainConfig(config, apiName)
		if err != nil {
			fmt.Printf("❌ Error getting chain config: %v\n", err)
			return nil, fmt.Errorf("failed to get chain config: %w", err)
		}

		// Execute prerequisite API if chain is configured
		if chainConfig != nil {
			fmt.Printf("🔗 API Chain detected for %s\n", apiName)
			fmt.Printf("🔗 Chain type: %s\n", chainConfig.ChainType)
			fmt.Printf("🔗 Prerequisite API: %s\n", chainConfig.PrerequisiteAPI)

			// Check if this is a once_before_benchmark chain and already executed
			cacheKey := fmt.Sprintf("%s_%s", apiName, chainConfig.PrerequisiteAPI)

			if chainConfig.ChainType == "once_before_benchmark" {
				if cachedResult, exists := onceBenchmarkCache[cacheKey]; exists {
					fmt.Printf("🔗 ✅ Using cached result from once_before_benchmark execution\n")
					chainResult = cachedResult
				} else {
					fmt.Printf("🔗 🚀 Executing once_before_benchmark prerequisite API\n")
					// Execute prerequisite API first
					chainResult, err = rb.executePrerequisiteAPI(chainConfig, config, apiList)
					if err != nil {
						return nil, fmt.Errorf("prerequisite API execution failed: %w", err)
					}

					// Cache the result
					onceBenchmarkCache[cacheKey] = chainResult
					fmt.Printf("✅ Prerequisite API completed and cached\n")
				}
			} else {
				// Execute prerequisite API first (normal chains)
				chainResult, err = rb.executePrerequisiteAPI(chainConfig, config, apiList)
				if err != nil {
					return nil, fmt.Errorf("prerequisite API execution failed: %w", err)
				}

				fmt.Printf("✅ Prerequisite API completed successfully\n")
			}
		} else {
			fmt.Printf("🔍 DEBUG: No chain configuration found for API: %s\n", apiName)
		}
	} else {
		fmt.Printf("🔍 DEBUG: Skipping chain check for prerequisite API: %s\n", apiName)
	}

	// Initialize collections
	parameters := make(map[string]string)
	var requestBody interface{}

	// Process parameters
	commonParams, _ := userInputs["common_parameters"].(map[string]interface{})
	apiSpecificParams, _ := userInputs["api_specific_parameters"].(map[string]interface{})

	for _, p := range apiInfo.Parameters {
		paramValue := getParameterValue(p.Name, commonParams, apiSpecificParams)

		// If we have chain result, try to apply response mapping for parameters
		if chainResult != nil && chainConfig.ResponseMapping != nil {
			if mappedValue, exists := chainConfig.ResponseMapping.Parameters[p.Name]; exists && mappedValue != "" {
				var extractedValue interface{}
				var err error

				// First check if it's a special computed value (like resStar)
				if chainResult.ExtractedData != nil {
					if val, exists := chainResult.ExtractedData[mappedValue]; exists {
						extractedValue = val
						err = nil
					} else {
						// Fallback to JSONPath extraction
						extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
					}
				} else {
					// Normal JSONPath extraction
					extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
				}

				if err == nil && extractedValue != "" {
					if strVal, ok := extractedValue.(string); ok {
						paramValue = strVal
						fmt.Printf("🔗 Applied parameter mapping: %s = %s (from %s)\n", p.Name, paramValue, mappedValue)
					} else {
						fmt.Printf("❌ Extracted value for parameter mapping '%s' is not a string\n", p.Name)
					}
				}
			}
		}

		// Required parameter validation
		if p.Required && paramValue == "" {
			fmt.Printf("❌ Required parameter '%s' is empty or missing\n", p.Name)
			fmt.Printf("📋 Please fill the 'value' field for '%s' in configuration.yaml\n", p.Name)
			return nil, fmt.Errorf("required parameter '%s' is empty or missing (check configuration.yaml)", p.Name)
		}

		parameters[p.Name] = paramValue
	}

	// Process request body
	if len(apiInfo.RequestBodySchema.RequiredFields) > 0 {
		commonBodies, _ := userInputs["common_request_bodies"].(map[string]interface{})
		apiSpecificBodies, _ := userInputs["api_specific_request_bodies"].(map[string]interface{})

		bodyMap := make(map[string]interface{})
		schemaName := apiInfo.RequestBodySchema.SchemaName

		for _, fieldName := range apiInfo.RequestBodySchema.RequiredFields {
			fieldValue := getRequestBodyFieldValue(fieldName, schemaName, commonBodies, apiSpecificBodies)

			// If we have chain result, try to apply response mapping for request body
			if chainResult != nil && chainConfig.ResponseMapping != nil {
				if mappedValue, exists := chainConfig.ResponseMapping.RequestBody[fieldName]; exists && mappedValue != "" {
					var extractedValue interface{}
					var err error

					fmt.Printf("🔗 DEBUG: Trying to map field '%s' using mapping '%s'\n", fieldName, mappedValue)

					// First check if it's a special computed value (like resStar)
					if chainResult.ExtractedData != nil {
						fmt.Printf("🔗 DEBUG: ExtractedData available, contents:\n")
						for key, value := range chainResult.ExtractedData {
							fmt.Printf("🔗   %s: %v\n", key, value)
						}

						// Try multiple possible keys for resStar
						possibleKeys := []string{mappedValue, "resStar", "res_star", "XRES*", "xresStar"}

						for _, key := range possibleKeys {
							if val, exists := chainResult.ExtractedData[key]; exists {
								extractedValue = val
								err = nil
								fmt.Printf("🔗 DEBUG: Found value using key '%s': %v\n", key, val)
								break
							}
						}

						if extractedValue == nil {
							fmt.Printf("🔗 DEBUG: No value found in ExtractedData, trying JSONPath extraction\n")
							// Fallback to JSONPath extraction
							extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
						}
					} else {
						fmt.Printf("🔗 DEBUG: ExtractedData is nil, using JSONPath extraction only\n")
						// Normal JSONPath extraction
						extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
					}

					if err == nil && extractedValue != "" {
						fieldValue = extractedValue
						fmt.Printf("🔗 ✅ Applied request body mapping: %s = %s (from %s)\n", fieldName, fieldValue, mappedValue)
					} else {
						fmt.Printf("🔗 ❌ Failed to extract value for mapping '%s': %v\n", mappedValue, err)
					}
				}
			}

			// Required field validation
			if fieldValue == nil || fieldValue == "" {
				fmt.Printf("❌ Required request body field '%s' is empty or missing\n", fieldName)
				fmt.Printf("📋 Please fill the 'value' field for '%s' in configuration.yaml under '%s' schema\n", fieldName, schemaName)
				return nil, fmt.Errorf("required request body field '%s' is empty or missing (check configuration.yaml)", fieldName)
			}

			bodyMap[fieldName] = fieldValue
		}

		if len(bodyMap) > 0 {
			requestBody = bodyMap
		}
	} else if apiInfo.RequestBody != "" {
		requestBody = GetDefaultRequestBodyForType(apiInfo.RequestBody)
	}

	fmt.Printf("✅ Configuration validation passed - ready for execution\n")

	execInfo := &types.APIExecutionInfo{
		NF:          nf,
		APIName:     apiName,
		Method:      apiInfo.Method,
		Path:        servicePath + apiInfo.Path,
		Parameters:  parameters,
		RequestBody: requestBody,
		Headers:     make(map[string]string),
	}

	// Apply header mappings from chain if available
	if chainResult != nil && chainConfig.ResponseMapping != nil {
		for headerName, mappedValue := range chainConfig.ResponseMapping.Headers {
			if mappedValue != "" {
				var extractedValue interface{}
				var err error

				// First check if it's a special computed value (like resStar)
				if chainResult.ExtractedData != nil {
					if val, exists := chainResult.ExtractedData[mappedValue]; exists {
						extractedValue = val
						err = nil
					} else {
						// Fallback to JSONPath extraction
						extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
					}
				} else {
					// Normal JSONPath extraction
					extractedValue, err = ExtractValueFromResponse(chainResult.ResponseBody, mappedValue)
				}

				if err == nil && extractedValue != "" {
					if strVal, ok := extractedValue.(string); ok {
						execInfo.Headers[headerName] = strVal
						fmt.Printf("🔗 Applied header mapping: %s = %s (from %s)\n", headerName, strVal, mappedValue)
					} else {
						fmt.Printf("❌ Extracted value for header mapping '%s' is not a string\n", headerName)
					}
				}
			}
		}
	}

	return execInfo, nil
}

// executePrerequisiteAPI executes prerequisite API and returns the result (simplified version)
func (rb *RequestBuilder) executePrerequisiteAPI(chainConfig *types.APIChainConfig, config map[string]interface{}, apiList types.APIList) (*types.ChainExecutionResult, error) {
	fmt.Printf("🔗 Executing prerequisite API: %s\n", chainConfig.PrerequisiteAPI)

	// Parse prerequisite API name
	prereqNF, prereqAPI := ParseAPIName(chainConfig.PrerequisiteAPI)
	if prereqNF == "" {
		return nil, fmt.Errorf("prerequisite API must include NF: %s", chainConfig.PrerequisiteAPI)
	}

	// Prepare prerequisite API execution WITH CHAIN SKIP FLAG
	prereqExecInfo, err := rb.prepareAPIExecutionWithChainFlag(apiList, config, prereqNF, prereqAPI, true)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare prerequisite API execution: %w", err)
	}

	// Use executor's discovery and setup methods
	executor := NewAPIExecutor(30 * time.Second)

	userInputs := config["user_inputs"].(map[string]interface{})
	globalSettings := userInputs["global_settings"].(map[string]interface{})

	// Discover NF URL
	var discoveredURL string
	if strings.ToUpper(prereqNF) == "NRF" {
		nrfURL, ok := GetConfigString(globalSettings["nrf_url"])
		if !ok || nrfURL == "" {
			return nil, fmt.Errorf("NRF URL is required for prerequisite API")
		}
		discoveredURL = nrfURL
	} else {
		discoveredURL, err = executor.discoverNFURL(globalSettings, prereqNF)
		if err != nil {
			return nil, fmt.Errorf("failed to discover prerequisite NF URL: %w", err)
		}
	}
	// For Debug: replace discovered URL with a specific IP if needed
	if discoveredURL == "http://controlplane-free5gc-ausf-service:80" {
		discoveredURL = "http://10.96.43.148:80"
		fmt.Printf("🔧 DEBUG: Replaced AUSF service URL with IP: %s\n", discoveredURL)
	}
	// Setup execution info
	prereqExecInfo.DiscoveredURL = discoveredURL
	rb.PopulateHeaders(prereqExecInfo, prereqNF, config)

	// Build final URL
	finalURL := rb.BuildFinalURL(prereqExecInfo)
	prereqExecInfo.FinalURL = finalURL

	fmt.Printf("🔗 Prerequisite API URL: %s\n", finalURL)

	// Execute using HTTP client (compatible with http_client.go)
	httpClient := NewHTTPClient() // From http_client.go
	result := httpClient.ExecuteWithResult(prereqExecInfo, 0)

	if result.Error != nil {
		return &types.ChainExecutionResult{
			Error:      result.Error,
			StatusCode: result.StatusCode,
		}, result.Error
	}

	fmt.Printf("✅ Prerequisite API completed: %d\n", result.StatusCode)
	fmt.Printf("🔗 Prerequisite API response: %s\n", string(result.ResponseBody))

	// Special handling for PostUeAuthentications to compute resStar
	chainResult := &types.ChainExecutionResult{
		ResponseBody: result.ResponseBody,
		StatusCode:   result.StatusCode,
		Error:        nil,
	}

	// Check if the prerequisite API is PostUeAuthentications and compute resStar
	if strings.Contains(strings.ToLower(chainConfig.PrerequisiteAPI), "postuea") ||
		strings.Contains(strings.ToLower(chainConfig.PrerequisiteAPI), "authentication") {
		fmt.Printf("🔐 Detected PostUeAuthentications - computing resStar\n")

		resStar, err := rb.computeResStar(result.ResponseBody, config)
		if err != nil {
			fmt.Printf("❌ Failed to compute resStar: %v\n", err)
		} else {
			// Add resStar to the chain result for mapping
			if chainResult.ExtractedData == nil {
				chainResult.ExtractedData = make(map[string]interface{})
			}
			chainResult.ExtractedData["resStar"] = hex.EncodeToString(resStar)
			fmt.Printf("✅ Computed resStar: %s\n", hex.EncodeToString(resStar))
		}
	}

	return chainResult, nil
}

// BuildFinalURL builds the complete URL from discovered URL and path with parameter substitution
func (rb *RequestBuilder) BuildFinalURL(execInfo *types.APIExecutionInfo) string {
	baseURL := strings.TrimSuffix(execInfo.DiscoveredURL, "/")
	path := execInfo.Path

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var queryParams []string

	// Replace path parameters and collect query parameters
	fmt.Printf("🔍 DEBUG: Original path: %s\n", path)
	fmt.Printf("🔍 DEBUG: Available parameters: %v\n", execInfo.Parameters)

	for paramName, paramValue := range execInfo.Parameters {
		placeholder := "{" + paramName + "}"
		if strings.Contains(path, placeholder) {
			// Path parameter
			path = strings.Replace(path, placeholder, paramValue, -1)
			fmt.Printf("🔗 ✅ Replaced path parameter: %s -> %s\n", placeholder, paramValue)
		} else {
			// Query parameter
			queryParams = append(queryParams, paramName+"="+paramValue)
			fmt.Printf("🔗 ✅ Added query parameter: %s=%s\n", paramName, paramValue)
		}
	}

	// Build final URL
	finalURL := baseURL + path

	// Add query parameters if any
	if len(queryParams) > 0 {
		finalURL += "?" + strings.Join(queryParams, "&")
	}

	// Check for any remaining unreplaced parameters
	if strings.Contains(finalURL, "{") && strings.Contains(finalURL, "}") {
		fmt.Printf("⚠️  WARNING: Unreplaced parameters found in URL: %s\n", finalURL)
	}

	fmt.Printf("🔍 DEBUG: BuildFinalURL - Base: %s, Path: %s, Final: %s\n", baseURL, path, finalURL)

	return finalURL
}

// PopulateHeaders populates HTTP headers for the request
func (rb *RequestBuilder) PopulateHeaders(execInfo *types.APIExecutionInfo, targetNF string, config map[string]interface{}) error {
	if execInfo.Headers == nil {
		execInfo.Headers = make(map[string]string)
	}

	// Set default headers
	execInfo.Headers["Content-Type"] = "application/json"
	execInfo.Headers["Accept"] = "application/json"

	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		return nil
	}

	nfSettings, ok := userInputs["nf_settings"].(map[string]interface{})
	if !ok {
		return nil
	}

	targetNFSettings, ok := nfSettings[targetNF].(map[string]interface{})
	if !ok {
		return nil
	}

	customHeaders, ok := targetNFSettings["custom_headers"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Process custom headers
	for headerName, headerConfig := range customHeaders {
		headerConfigMap, ok := headerConfig.(map[string]interface{})
		if !ok {
			continue
		}

		if value, exists := headerConfigMap["value"]; exists {
			if valueStr, ok := value.(string); ok && valueStr != "" {
				execInfo.Headers[headerName] = valueStr
			}
		}
	}

	return nil
}

// LoadAPIList loads api_list.yaml file
func (rb *RequestBuilder) LoadAPIList() (types.APIList, error) {
	filename := filepath.Join("openapi", "api_list.yaml")

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read api_list.yaml: %w", err)
	}

	var apiList types.APIList
	if err := yaml.Unmarshal(data, &apiList); err != nil {
		return nil, fmt.Errorf("failed to parse api_list.yaml: %w", err)
	}

	return apiList, nil
}

// GetAPIInfoWithServicePath finds API information and service path from tree-structured api_list
func (rb *RequestBuilder) GetAPIInfoWithServicePath(apiList types.APIList, nf, apiName string) (*types.APIListEntry, string, error) {
	if nfServices, exists := apiList[nf]; exists {
		for _, serviceInfo := range nfServices {
			if api, exists := serviceInfo.APIs[apiName]; exists {
				return &api, serviceInfo.Path, nil
			}
		}
	}
	return nil, "", fmt.Errorf("API '%s' not found in NF '%s'", apiName, nf)
}

// GetAPIInfo finds API information from tree-structured api_list
func (rb *RequestBuilder) GetAPIInfo(apiList types.APIList, nf, apiName string) (*types.APIListEntry, error) {
	apiInfo, _, err := rb.GetAPIInfoWithServicePath(apiList, nf, apiName)
	return apiInfo, err
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// getParameterValue retrieves parameter value from configuration
func getParameterValue(paramName string, commonParams, apiSpecificParams map[string]interface{}) string {
	// Check API-specific parameters first
	if apiSpecificParams != nil {
		if paramConfig, exists := apiSpecificParams[paramName]; exists {
			if paramMap, ok := paramConfig.(map[string]interface{}); ok {
				if value, exists := paramMap["value"]; exists {
					return GetConfigValue(value)
				}
			}
		}
	}

	// Check common parameters
	if commonParams != nil {
		if paramConfig, exists := commonParams[paramName]; exists {
			if paramMap, ok := paramConfig.(map[string]interface{}); ok {
				if value, exists := paramMap["value"]; exists {
					return GetConfigValue(value)
				}
			}
		}
	}

	return ""
}

// getRequestBodyFieldValue retrieves request body field value from configuration
func getRequestBodyFieldValue(fieldName, schemaName string, commonBodies, apiSpecificBodies map[string]interface{}) interface{} {
	// Check API-specific request bodies first
	if apiSpecificBodies != nil {
		if schemaConfig, exists := apiSpecificBodies[schemaName]; exists {
			if schemaMap, ok := schemaConfig.(map[string]interface{}); ok {
				if properties, exists := schemaMap["properties"]; exists {
					if propertiesMap, ok := properties.(map[string]interface{}); ok {
						if fieldConfig, exists := propertiesMap[fieldName]; exists {
							if fieldMap, ok := fieldConfig.(map[string]interface{}); ok {
								if value, exists := fieldMap["value"]; exists {
									return value
								}
							}
						}
					}
				}
			}
		}
	}

	// Check common request bodies
	if commonBodies != nil {
		if schemaConfig, exists := commonBodies[schemaName]; exists {
			if schemaMap, ok := schemaConfig.(map[string]interface{}); ok {
				if properties, exists := schemaMap["properties"]; exists {
					if propertiesMap, ok := properties.(map[string]interface{}); ok {
						if fieldConfig, exists := propertiesMap[fieldName]; exists {
							if fieldMap, ok := fieldConfig.(map[string]interface{}); ok {
								if value, exists := fieldMap["value"]; exists {
									return value
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// GetDefaultRequestBodyForType returns default request body for a given type
func GetDefaultRequestBodyForType(bodyType string) interface{} {
	switch bodyType {
	case "object":
		return map[string]interface{}{}
	case "array":
		return []interface{}{}
	case "string":
		return ""
	case "number":
		return 0
	case "boolean":
		return false
	default:
		return map[string]interface{}{}
	}
}

func (rb *RequestBuilder) computeResStar(responseBody string, config map[string]interface{}) ([]byte, error) {
	var authResponse map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &authResponse); err != nil {
		return nil, fmt.Errorf("failed to parse authentication response: %w", err)
	}

	// Extract data from response
	authData, ok := authResponse["5gAuthData"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("5gAuthData not found in authentication response")
	}

	randStr, _ := authData["rand"].(string)
	autnStr, _ := authData["autn"].(string)
	hxresStarStr, _ := authData["hxresStar"].(string)
	servingNetworkName, _ := authResponse["servingNetworkName"].(string)

	fmt.Printf("🔐 Extracted from PostUeAuthentications response:\n")
	fmt.Printf("   RAND: %s\n", randStr)
	fmt.Printf("   AUTN: %s\n", autnStr)
	fmt.Printf("   HXRES*: %s\n", hxresStarStr)
	fmt.Printf("   ServingNetworkName: %s\n", servingNetworkName)

	// Convert hex strings to bytes
	rand, err := hex.DecodeString(randStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode RAND: %w", err)
	}

	autn, err := hex.DecodeString(autnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode AUTN: %w", err)
	}

	// Get credentials from config
	userInputs := config["user_inputs"].(map[string]interface{})
	ueCredentials := userInputs["ue_credentials"].(map[string]interface{})

	kStr, _ := GetConfigString(ueCredentials["k"])
	opcStr, _ := GetConfigString(ueCredentials["opc"])

	k, err := hex.DecodeString(kStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode K: %w", err)
	}

	opc, err := hex.DecodeString(opcStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode OPc: %w", err)
	}

	// Create UEAuth instance - let PerformUEAuth handle SQN/AMF extraction
	ueAuth := &calculate.UEAuth{
		K:           k,
		OPc:         opc,
		RAND:        rand,
		AUTN:        autn,
		ServingName: servingNetworkName,
	}

	if err := ueAuth.PerformUEAuth(); err != nil {
		return nil, fmt.Errorf("failed to perform UE authentication: %w", err)
	}

	fmt.Printf("🔐 Computed XRES* (resStar): %s\n", hex.EncodeToString(ueAuth.ResStar))
	ueAuth.DebugCompareWithExpected(hxresStarStr)

	return ueAuth.ResStar, nil
}
