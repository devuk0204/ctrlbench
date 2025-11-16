package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"

	"github.com/devuk0204/ctrlbench/types"
)

func ExtractServicePath(service types.ServiceMetadata) string {
	if service.OpenAPISpec != nil && len(service.OpenAPISpec.Servers) > 0 {
		serverURL := service.OpenAPISpec.Servers[0].Url

		if strings.Contains(serverURL, "{") {
			if idx := strings.Index(serverURL, "}"); idx != -1 && len(serverURL) > idx+1 {
				path := serverURL[idx+1:]
				if strings.HasPrefix(path, "/") {
					return path
				}
				return "/" + path
			}
		}

		if strings.Contains(serverURL, "://") {
			parts := strings.Split(serverURL, "://")
			if len(parts) > 1 {
				hostAndPath := parts[1]
				if idx := strings.Index(hostAndPath, "/"); idx != -1 {
					return hostAndPath[idx:]
				}
			}
		}
	}

	return ExtractServicePathFromAPI(service)
}

func ExtractServicePathFromAPI(service types.ServiceMetadata) string {
	if len(service.APIs) == 0 {
		return "/"
	}

	var firstPath string
	for _, api := range service.APIs {
		if api.Path != "" {
			firstPath = api.Path
			break
		}
	}

	if firstPath == "" {
		return "/"
	}

	segments := strings.Split(strings.Trim(firstPath, "/"), "/")
	if len(segments) > 0 {
		if len(segments) >= 2 && (segments[1] == "v1" || segments[1] == "v2") {
			return "/" + strings.Join(segments[:2], "/")
		}
		return "/" + segments[0]
	}

	return "/"
}

func ExtractVersionFromPath(servicePath string) string {
	segments := strings.Split(strings.Trim(servicePath, "/"), "/")
	for _, segment := range segments {
		if strings.HasPrefix(segment, "v") && len(segment) <= 3 {
			return segment
		}
	}
	return "v1"
}

func ExtractMethodFromAPI(apiName string) string {
	// Check for method prefixes in API name
	apiUpper := strings.ToUpper(apiName)

	if strings.HasPrefix(apiUpper, "POST") {
		return "POST"
	} else if strings.HasPrefix(apiUpper, "GET") {
		return "GET"
	} else if strings.HasPrefix(apiUpper, "PUT") {
		return "PUT"
	} else if strings.HasPrefix(apiUpper, "DELETE") {
		return "DELETE"
	} else if strings.HasPrefix(apiUpper, "PATCH") {
		return "PATCH"
	}

	// Check for bracketed method format like [POST]
	methods := []string{"POST", "GET", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		if strings.Contains(apiName, "["+method+"]") {
			return method
		}
	}

	return "GET" // default
}

func matchesAPI(operation *types.Operation, api types.APIMetadata) bool {
	return matchesOperation(operation, api.Name)
}

// GenerateUniqueAPIName generates unique API name from path and method
func GenerateUniqueAPIName(path, method, operationId string) string {
	// If operationId exists, use it
	if operationId != "" {
		return operationId
	}

	// toTitle converts the first character of a string to uppercase
	toTitle := func(s string) string {
		if len(s) == 0 {
			return s
		}
		r, size := utf8.DecodeRuneInString(s)
		return string(unicode.ToUpper(r)) + s[size:]
	}

	// Generate from method + path
	// Remove leading slash and replace path parameters
	cleanPath := strings.TrimPrefix(path, "/")

	// Replace path parameters {param} with capitalized name
	re := regexp.MustCompile(`\{([^}]+)\}`)
	cleanPath = re.ReplaceAllStringFunc(cleanPath, func(match string) string {
		param := strings.Trim(match, "{}")
		return toTitle(param)
	})

	// Replace slashes and hyphens with nothing, capitalize words
	parts := strings.FieldsFunc(cleanPath, func(c rune) bool {
		return c == '/' || c == '-'
	})

	var result strings.Builder
	result.WriteString(toTitle(strings.ToLower(method)))

	for _, part := range parts {
		if part != "" {
			result.WriteString(toTitle(part))
		}
	}

	return result.String()
}

func CleanServiceName(serviceName string) string {
	if strings.HasSuffix(serviceName, "Service") {
		return strings.TrimSuffix(serviceName, "Service")
	}
	return serviceName
}

// NF 관련 유틸리티 함수들
func GroupServicesByNF(services map[string]types.ServiceMetadata) map[string][]types.ServiceMetadata {
	nfServices := make(map[string][]types.ServiceMetadata)
	for _, service := range services {
		nf := service.NF
		if nf == "" {
			nf = "UNKNOWN"
		}
		nfServices[nf] = append(nfServices[nf], service)
	}
	return nfServices
}

// IsPathParameter - Check if parameter is a path parameter
func IsPathParameter(paramName, path string) bool {
	return strings.Contains(path, "{"+paramName+"}")
}

// Configuration value extraction functions

// GetConfigString extracts string value from configuration node
// Handles both new format (map with "value" key) and old format (direct string)
func GetConfigString(value interface{}) (string, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		if val, ok := v["value"]; ok {
			switch valType := val.(type) {
			case string:
				return valType, true
			case bool:
				if valType {
					return "true", true
				} else {
					return "false", true
				}
			case int, int64, float64:
				return fmt.Sprintf("%v", valType), true
			case nil:
				return "", false
			default:
				return fmt.Sprintf("%v", valType), true
			}
		}
		return "", false
	case string:
		return v, true
	case bool:
		if v {
			return "true", true
		} else {
			return "false", true
		}
	case int, int64, float64:
		return fmt.Sprintf("%v", v), true
	case nil:
		return "", false
	default:
		return fmt.Sprintf("%v", v), true
	}
}

// GetConfigValue extracts any value from configuration node and converts to string
func GetConfigValue(node interface{}) string {
	if node == nil {
		return ""
	}

	// If it's a map with 'value' field (new configuration format)
	if nodeMap, ok := node.(map[string]interface{}); ok {
		if val, exists := nodeMap["value"]; exists {
			return fmt.Sprintf("%v", val)
		}
	}

	// If it's a direct value (old format or simple value)
	return fmt.Sprintf("%v", node)
}

// YAML file loading functions

// LoadYAMLFile loads and parses a YAML file into the provided interface
func LoadYAMLFile(filename string, target interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filename, err)
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	return nil
}

// LoadConfiguration loads configuration.yaml file
func LoadConfiguration() (map[string]interface{}, error) {
	var config map[string]interface{}
	err := LoadYAMLFile("configuration.yaml", &config)
	return config, err
}

// Map and slice utility functions

// GetSortedKeys returns sorted keys from any map[string]T
func GetSortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String utility functions

// TrimSlashRight removes trailing slash from string
func TrimSlashRight(s string) string {
	return strings.TrimSuffix(s, "/")
}

// TrimSlashLeft removes leading slash from string
func TrimSlashLeft(s string) string {
	return strings.TrimPrefix(s, "/")
}

// NormalizeURL ensures URL has proper format (no trailing slash)
func NormalizeURL(url string) string {
	return TrimSlashRight(url)
}

// getMethodFromOpenAPISpec - Extract HTTP method from OpenAPI spec for specific API
func GetMethodFromOpenAPISpec(api types.APIMetadata, service types.ServiceMetadata, apiName string) string {
	if service.OpenAPISpec == nil {
		return ExtractMethodFromAPI(apiName)
	}

	// Find the operation in OpenAPI spec that matches this API
	for path, pathItem := range service.OpenAPISpec.Paths {
		if path != api.Path {
			continue
		}

		// Check each HTTP method in the path
		if pathItem.Get != nil && matchesOperation(pathItem.Get, apiName) {
			return "GET"
		}
		if pathItem.Post != nil && matchesOperation(pathItem.Post, apiName) {
			return "POST"
		}
		if pathItem.Put != nil && matchesOperation(pathItem.Put, apiName) {
			return "PUT"
		}
		if pathItem.Delete != nil && matchesOperation(pathItem.Delete, apiName) {
			return "DELETE"
		}
		if pathItem.Patch != nil && matchesOperation(pathItem.Patch, apiName) {
			return "PATCH"
		}
	}

	// Fallback to extracting from API name
	return ExtractMethodFromAPI(apiName)
}

// matchesOperation - Check if operation matches the API name
func matchesOperation(operation *types.Operation, apiName string) bool {
	// Direct operationId match
	if operation.OperationID == apiName {
		return true
	}

	// Remove method suffix and try again
	cleanName := strings.TrimSuffix(apiName, " [GET]")
	cleanName = strings.TrimSuffix(cleanName, " [POST]")
	cleanName = strings.TrimSuffix(cleanName, " [PUT]")
	cleanName = strings.TrimSuffix(cleanName, " [DELETE]")
	cleanName = strings.TrimSuffix(cleanName, " [PATCH]")

	return operation.OperationID == cleanName
}

// ExtractValueFromResponse extracts value from JSON response using JSONPath
func ExtractValueFromResponse(responseBody, jsonPath string) (interface{}, error) {
	if responseBody == "" || jsonPath == "" {
		return nil, fmt.Errorf("empty response body or json path")
	}

	// Use gjson for JSONPath extraction
	result := gjson.Get(responseBody, jsonPath)
	if !result.Exists() {
		return nil, fmt.Errorf("path %s not found in response", jsonPath)
	}

	return result.Value(), nil
}

// ApplyResponseMapping applies prerequisite API response to main API execution info
func ApplyResponseMapping(execInfo *types.APIExecutionInfo, chainResult *types.ChainExecutionResult, mapping *types.APIResponseMapping) error {
	if mapping == nil || chainResult == nil || chainResult.ResponseBody == "" {
		return nil // No mapping or no response to map
	}

	fmt.Printf("Applying response mapping from prerequisite API\n")

	// Extract values from response
	extractedData := make(map[string]interface{})

	// Map parameters
	for paramName, jsonPath := range mapping.Parameters {
		value, err := ExtractValueFromResponse(chainResult.ResponseBody, jsonPath)
		if err != nil {
			fmt.Printf("Failed to extract parameter %s from path %s: %v\n", paramName, jsonPath, err)
			continue
		}

		// Apply to parameters
		if execInfo.Parameters == nil {
			execInfo.Parameters = make(map[string]string)
		}
		execInfo.Parameters[paramName] = fmt.Sprintf("%v", value)
		extractedData[paramName] = value

		fmt.Printf("Mapped parameter %s: %v\n", paramName, value)
	}

	// Map headers
	for headerName, jsonPath := range mapping.Headers {
		value, err := ExtractValueFromResponse(chainResult.ResponseBody, jsonPath)
		if err != nil {
			fmt.Printf("Failed to extract header %s from path %s: %v\n", headerName, jsonPath, err)
			continue
		}

		// Apply to headers
		if execInfo.Headers == nil {
			execInfo.Headers = make(map[string]string)
		}
		execInfo.Headers[headerName] = fmt.Sprintf("%v", value)
		extractedData[headerName] = value

		fmt.Printf("Mapped header %s: %v\n", headerName, value)
	}

	// Map request body fields
	if len(mapping.RequestBody) > 0 && execInfo.RequestBody != nil {
		var bodyMap map[string]interface{}

		// Handle different types of RequestBody
		switch rb := execInfo.RequestBody.(type) {
		case string:
			if rb != "" {
				if err := json.Unmarshal([]byte(rb), &bodyMap); err != nil {
					fmt.Printf("Failed to parse existing request body string: %v\n", err)
					return nil
				}
			} else {
				bodyMap = make(map[string]interface{})
			}
		case map[string]interface{}:
			bodyMap = rb
		default:
			// Try to marshal and unmarshal to get a map
			if bodyBytes, err := json.Marshal(execInfo.RequestBody); err == nil {
				if err := json.Unmarshal(bodyBytes, &bodyMap); err != nil {
					fmt.Printf("Failed to convert request body to map: %v\n", err)
					return nil
				}
			} else {
				bodyMap = make(map[string]interface{})
			}
		}

		// Apply mappings to request body
		for bodyField, jsonPath := range mapping.RequestBody {
			value, err := ExtractValueFromResponse(chainResult.ResponseBody, jsonPath)
			if err != nil {
				fmt.Printf("Failed to extract request body field %s from path %s: %v\n", bodyField, jsonPath, err)
				continue
			}

			// Set nested field using dot notation
			setNestedField(bodyMap, bodyField, value)
			extractedData[bodyField] = value

			fmt.Printf("Mapped request body field %s: %v\n", bodyField, value)
		}

		// Update request body back to the original type
		switch execInfo.RequestBody.(type) {
		case string:
			if updatedBody, err := json.Marshal(bodyMap); err == nil {
				execInfo.RequestBody = string(updatedBody)
			}
		default:
			execInfo.RequestBody = bodyMap
		}
	}

	// Store extracted data for reference
	chainResult.ExtractedData = extractedData

	return nil
}

// setNestedField sets a nested field in map using dot notation (e.g., "user.id")
func setNestedField(m map[string]interface{}, fieldPath string, value interface{}) {
	parts := strings.Split(fieldPath, ".")
	current := m

	// Navigate to parent
	for i := 0; i < len(parts)-1; i++ {
		if current[parts[i]] == nil {
			current[parts[i]] = make(map[string]interface{})
		}
		if next, ok := current[parts[i]].(map[string]interface{}); ok {
			current = next
		} else {
			return // Cannot set nested field
		}
	}

	// Set final field
	current[parts[len(parts)-1]] = value
}

// GetAPIChainConfig reads API chain configuration for a specific API
func GetAPIChainConfig(config map[string]interface{}, apiName string) (*types.APIChainConfig, error) {
	userInputs, ok := config["user_inputs"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	chainConfig, ok := userInputs["api_chain_configuration"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	// Check if chain configuration is enabled
	enabledNode := chainConfig["enabled"]
	enabled, ok := GetConfigString(enabledNode)
	if !ok || enabled != "true" {
		return nil, nil
	}

	chains, ok := chainConfig["chains"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	apiChain, ok := chains[apiName].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	// Extract main API NF
	mainNF, ok := GetConfigString(apiChain["NF"])
	if !ok || mainNF == "" {
		return nil, fmt.Errorf("main API NF not specified for chain %s", apiName)
	}

	// Extract prerequisite_api configuration
	prerequisiteAPIConfig, ok := apiChain["prerequisite_api"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("prerequisite_api configuration not found for chain %s", apiName)
	}

	// Extract prerequisite API NF
	prerequisiteNF, ok := GetConfigString(prerequisiteAPIConfig["NF"])
	if !ok || prerequisiteNF == "" {
		return nil, fmt.Errorf("prerequisite API NF not specified for chain %s", apiName)
	}

	// Extract prerequisite API name
	prerequisiteAPI, ok := GetConfigString(prerequisiteAPIConfig["value"])
	if !ok || prerequisiteAPI == "" {
		return nil, fmt.Errorf("prerequisite_api value not specified for chain %s", apiName)
	}

	// Format prerequisite API as NF_API for consistency
	fullPrerequisiteAPI := fmt.Sprintf("%s_%s", prerequisiteNF, prerequisiteAPI)

	// Extract chain_type value
	chainType := "once_before_benchmark" // default
	if chainTypeConfig, ok := apiChain["chain_type"].(map[string]interface{}); ok {
		if typeValue, ok := GetConfigString(chainTypeConfig["value"]); ok && typeValue != "" {
			chainType = typeValue
		}
	}

	// Parse response mapping if exists
	var responseMapping *types.APIResponseMapping
	if mappingConfig, ok := apiChain["response_mapping"].(map[string]interface{}); ok {
		responseMapping = &types.APIResponseMapping{
			Parameters:  make(map[string]string),
			Headers:     make(map[string]string),
			RequestBody: make(map[string]string),
		}

		// Parse parameter mappings
		if params, ok := mappingConfig["parameters"].(map[string]interface{}); ok {
			for key, paramConfig := range params {
				if paramMap, ok := paramConfig.(map[string]interface{}); ok {
					if value, ok := GetConfigString(paramMap["value"]); ok && value != "" {
						responseMapping.Parameters[key] = value
					}
				} else if strValue, ok := paramConfig.(string); ok {
					responseMapping.Parameters[key] = strValue
				}
			}
		}

		// Parse header mappings
		if headers, ok := mappingConfig["headers"].(map[string]interface{}); ok {
			for key, headerConfig := range headers {
				if headerMap, ok := headerConfig.(map[string]interface{}); ok {
					if value, ok := GetConfigString(headerMap["value"]); ok && value != "" {
						responseMapping.Headers[key] = value
					}
				} else if strValue, ok := headerConfig.(string); ok {
					responseMapping.Headers[key] = strValue
				}
			}
		}

		// Parse request body mappings
		if body, ok := mappingConfig["request_body"].(map[string]interface{}); ok {
			for key, bodyConfig := range body {
				if bodyMap, ok := bodyConfig.(map[string]interface{}); ok {
					if value, ok := GetConfigString(bodyMap["value"]); ok && value != "" {
						responseMapping.RequestBody[key] = value
					}
				} else if strValue, ok := bodyConfig.(string); ok {
					responseMapping.RequestBody[key] = strValue
				}
			}
		}
	}

	return &types.APIChainConfig{
		Enabled:         true,
		PrerequisiteAPI: fullPrerequisiteAPI,
		ChainType:       chainType,
		ResponseMapping: responseMapping,
	}, nil
}

// HasAPIChain checks if an API has chain configuration (for skipping required checks)
func HasAPIChain(config map[string]interface{}, apiName string) bool {
	chainConfig, err := GetAPIChainConfig(config, apiName)
	return err == nil && chainConfig != nil
}

// ParseAPIName parses API name in format "NF_SERVICE_API" or just "API"
func ParseAPIName(apiName string) (nf, api string) {
	parts := strings.Split(apiName, "_")
	if len(parts) >= 3 {
		// Format: NF_SERVICE_API
		nf = parts[0]
		api = strings.Join(parts[2:], "_") // In case API name has underscores
	} else if len(parts) == 2 {
		// Format: NF_API
		nf = parts[0]
		api = parts[1]
	} else {
		// Just API name
		api = apiName
	}
	return nf, api
}
