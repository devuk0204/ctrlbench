package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devuk0204/ctrlbench/types"
	"gopkg.in/yaml.v3"
)

// RequestBuilder handles API request construction
type RequestBuilder struct{}

// NewRequestBuilder creates a new request builder
func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{}
}

// PrepareAPIExecution prepares API execution info from configuration with detailed validation
func (rb *RequestBuilder) PrepareAPIExecution(apiList types.APIList, config map[string]interface{}, nf, apiName string) (*types.APIExecutionInfo, error) {
	fmt.Printf("🔍 DEBUG: Starting PrepareAPIExecution for NF=%s, API=%s\n", nf, apiName)

	apiInfo, servicePath, err := rb.GetAPIInfoWithServicePath(apiList, nf, apiName)
	if err != nil {
		return nil, err
	}

	fmt.Printf("🔍 DEBUG: Found API info - Service path: %s, API path: %s\n", servicePath, apiInfo.Path)
	fmt.Printf("🔍 DEBUG: Found API info - Parameters count: %d\n", len(apiInfo.Parameters))
	for i, p := range apiInfo.Parameters {
		fmt.Printf("🔍 DEBUG: Parameter[%d]: Name=%s, Required=%t, Type=%s\n", i, p.Name, p.Required, p.Type)
	}

	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("user_inputs not found in configuration")
	}

	// Prepare parameters - both required and optional
	parameters := make(map[string]string)
	commonParams, _ := userInputs["common_parameters"].(map[string]interface{})
	apiSpecificParams, _ := userInputs["api_specific_parameters"].(map[string]interface{})

	fmt.Printf("🔍 DEBUG: Common parameters keys: %v\n", getMapKeys(commonParams))
	fmt.Printf("🔍 DEBUG: API-specific parameters keys: %v\n", getMapKeys(apiSpecificParams))

	// Process all parameters (required and optional)
	for _, p := range apiInfo.Parameters {
		fmt.Printf("🔍 DEBUG: Processing parameter: %s (required: %t, in: %s)\n", p.Name, p.Required, p.In)

		// Get parameter value from configuration
		paramValue := getParameterValue(p.Name, commonParams, apiSpecificParams)
		fmt.Printf("🔍 DEBUG: Parameter %s value: '%s'\n", p.Name, paramValue)

		// Required parameter validation
		if p.Required && paramValue == "" {
			fmt.Printf("❌ Required parameter '%s' is empty or missing\n", p.Name)
			fmt.Printf("📋 Please fill the 'value' field for '%s' in configuration.yaml\n", p.Name)
			fmt.Printf("🛑 Execution stopped - configuration incomplete\n")
			return nil, fmt.Errorf("required parameter '%s' is empty or missing (check configuration.yaml)", p.Name)
		}

		// Add parameter to map (even if empty for optional parameters)
		parameters[p.Name] = paramValue
	}

	fmt.Printf("🔍 DEBUG: Final parameters map: %v\n", parameters)

	// Prepare request body - only required fields
	var requestBody interface{}
	if len(apiInfo.RequestBodySchema.RequiredFields) > 0 {
		fmt.Printf("🔍 DEBUG: Processing request body with %d required fields: %v\n",
			len(apiInfo.RequestBodySchema.RequiredFields), apiInfo.RequestBodySchema.RequiredFields)

		commonBodies, _ := userInputs["common_request_bodies"].(map[string]interface{})
		apiSpecificBodies, _ := userInputs["api_specific_request_bodies"].(map[string]interface{})

		fmt.Printf("🔍 DEBUG: Common bodies keys: %v\n", getMapKeys(commonBodies))
		fmt.Printf("🔍 DEBUG: API-specific bodies keys: %v\n", getMapKeys(apiSpecificBodies))

		bodyMap := make(map[string]interface{})
		schemaName := apiInfo.RequestBodySchema.SchemaName
		fmt.Printf("🔍 DEBUG: Schema name: %s\n", schemaName)

		for _, fieldName := range apiInfo.RequestBodySchema.RequiredFields {
			fmt.Printf("🔍 DEBUG: Processing required field: %s\n", fieldName)

			fieldValue := getRequestBodyFieldValue(fieldName, schemaName, commonBodies, apiSpecificBodies)
			fmt.Printf("🔍 DEBUG: Field %s value: %v\n", fieldName, fieldValue)
			if fieldValue == nil || fieldValue == "" {
				fmt.Printf("❌ Required request body field '%s' is empty or missing\n", fieldName)
				fmt.Printf("📋 Please fill the 'value' field for '%s' in configuration.yaml under '%s' schema\n", fieldName, schemaName)
				fmt.Printf("🛑 Execution stopped - configuration incomplete\n")
				return nil, fmt.Errorf("required request body field '%s' is empty or missing (check configuration.yaml)", fieldName)
			}

			bodyMap[fieldName] = fieldValue
		}

		if len(bodyMap) > 0 {
			requestBody = bodyMap
		}
		fmt.Printf("🔍 DEBUG: Final request body: %v\n", requestBody)
	} else if apiInfo.RequestBody != "" {
		fmt.Printf("🔍 DEBUG: Using default request body for type: %s\n", apiInfo.RequestBody)
		// Fallback to default request body if no schema available
		requestBody = GetDefaultRequestBodyForType(apiInfo.RequestBody)
	}

	// Combine service path and API path
	fullPath := servicePath + apiInfo.Path

	execInfo := &types.APIExecutionInfo{
		NF:          nf,
		APIName:     apiName,
		Method:      apiInfo.Method,
		Path:        fullPath,
		Parameters:  parameters,
		RequestBody: requestBody,
		Headers:     make(map[string]string),
	}

	fmt.Printf("✅ Configuration validation passed - ready for execution\n")
	fmt.Printf("🔍 DEBUG: Created execInfo with full path: %s (service: %s + api: %s)\n", fullPath, servicePath, apiInfo.Path)
	fmt.Printf("🔍 DEBUG: Created execInfo with %d parameters\n", len(execInfo.Parameters))
	return execInfo, nil
}

// BuildFinalURL builds the complete URL from discovered URL and path
func (rb *RequestBuilder) BuildFinalURL(execInfo *types.APIExecutionInfo) string {
	// Make sure we use the discovered URL as base
	baseURL := execInfo.DiscoveredURL
	path := execInfo.Path

	// Remove trailing slash from base URL if exists
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Ensure path starts with slash
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	finalURL := baseURL + path

	// Debug output
	fmt.Printf("🔍 DEBUG: BuildFinalURL - Base: %s, Path: %s, Final: %s\n", baseURL, path, finalURL)

	return finalURL
}

// PopulateHeaders populates HTTP headers for the request
func (rb *RequestBuilder) PopulateHeaders(execInfo *types.APIExecutionInfo, targetNF string, config map[string]interface{}) error {
	fmt.Printf("🔍 DEBUG: PopulateHeaders - targetNF: %s\n", targetNF)

	// Initialize headers map if nil
	if execInfo.Headers == nil {
		execInfo.Headers = make(map[string]string)
	}

	// Get user inputs and NF settings
	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ DEBUG: user_inputs not found in configuration\n")
		// Set default headers and return
		execInfo.Headers["Content-Type"] = "application/json"
		execInfo.Headers["Accept"] = "application/json"
		return nil
	}

	nfSettings, ok := userInputs["nf_settings"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ DEBUG: nf_settings not found in configuration\n")
		// Set default headers and return
		execInfo.Headers["Content-Type"] = "application/json"
		execInfo.Headers["Accept"] = "application/json"
		return nil
	}

	fmt.Printf("🔍 DEBUG: Available NF settings: %v\n", getMapKeys(nfSettings))

	// Get specific NF settings
	targetNFSettings, ok := nfSettings[targetNF].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ DEBUG: %s settings not found, using defaults\n", targetNF)
		// Set default headers and return
		execInfo.Headers["Content-Type"] = "application/json"
		execInfo.Headers["Accept"] = "application/json"
		return nil
	}

	fmt.Printf("🔍 DEBUG: Found %s settings: %v\n", targetNF, getMapKeys(targetNFSettings))

	// Get custom headers for this NF
	customHeaders, ok := targetNFSettings["custom_headers"].(map[string]interface{})
	if !ok {
		fmt.Printf("❌ DEBUG: custom_headers not found for %s, using defaults\n", targetNF)
		// Set default headers and return
		execInfo.Headers["Content-Type"] = "application/json"
		execInfo.Headers["Accept"] = "application/json"
		return nil
	}

	fmt.Printf("🔍 DEBUG: Found custom headers: %v\n", getMapKeys(customHeaders))

	// Process each custom header
	for headerName, headerConfig := range customHeaders {
		fmt.Printf("🔍 DEBUG: Processing header: %s\n", headerName)

		headerConfigMap, ok := headerConfig.(map[string]interface{})
		if !ok {
			fmt.Printf("❌ DEBUG: Invalid header config for %s\n", headerName)
			continue
		}

		// Extract the header value
		if value, exists := headerConfigMap["value"]; exists {
			if valueStr, ok := value.(string); ok && valueStr != "" {
				execInfo.Headers[headerName] = valueStr
				fmt.Printf("✅ DEBUG: Set header %s = %s\n", headerName, valueStr)
			} else {
				fmt.Printf("❌ DEBUG: Invalid value for header %s: %v (type: %T)\n", headerName, value, value)
			}
		} else {
			fmt.Printf("❌ DEBUG: No value found for header %s\n", headerName)
		}
	}

	// Set default headers if not already set by custom headers
	if _, exists := execInfo.Headers["Content-Type"]; !exists {
		execInfo.Headers["Content-Type"] = "application/json"
		fmt.Printf("🔍 DEBUG: Set default Content-Type\n")
	}
	if _, exists := execInfo.Headers["Accept"]; !exists {
		execInfo.Headers["Accept"] = "application/json"
		fmt.Printf("🔍 DEBUG: Set default Accept\n")
	}

	fmt.Printf("🔍 DEBUG: Final headers count: %d\n", len(execInfo.Headers))
	for k, v := range execInfo.Headers {
		fmt.Printf("🔍 DEBUG: Final header[%s] = %s\n", k, v)
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
	if nfServices, exists := apiList[nf]; exists {
		for _, serviceInfo := range nfServices {
			if api, exists := serviceInfo.APIs[apiName]; exists {
				return &api, nil
			}
		}
	}
	return nil, fmt.Errorf("API '%s' not found in NF '%s'", apiName, nf)
}

// Helper functions for configuration processing

// getMapKeys returns keys from a map as a slice
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

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
