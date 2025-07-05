package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devuk0204/ctrlbench/types"

	"gopkg.in/yaml.v3"
)

// LoadAPIList loads api_list.yaml file
func LoadAPIList() (types.APIList, error) {
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

// GetAPIInfo finds API information from tree-structured api_list
func GetAPIInfo(apiList types.APIList, nf, apiName string) (*types.APIListEntry, error) {
	if nfServices, exists := apiList[nf]; exists {
		for _, serviceInfo := range nfServices {
			if api, exists := serviceInfo.APIs[apiName]; exists {
				return &api, nil
			}
		}
	}
	return nil, fmt.Errorf("API '%s' not found in NF '%s'", apiName, nf)
}

// PrepareAPIExecution prepares API execution info from api_list and configuration
func PrepareAPIExecution(apiList types.APIList, config map[string]interface{}, nf, apiName string) (*types.APIExecutionInfo, error) {
	fmt.Printf("🔍 DEBUG: Starting PrepareAPIExecution for NF=%s, API=%s\n", nf, apiName)

	apiInfo, err := GetAPIInfo(apiList, nf, apiName)
	if err != nil {
		return nil, err
	}

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
		// Empty optional parameters will be filtered out in buildFinalURL
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

	execInfo := &types.APIExecutionInfo{
		NF:          nf,
		APIName:     apiName,
		Method:      apiInfo.Method,
		Path:        apiInfo.Path,
		Parameters:  parameters,
		RequestBody: requestBody,
		Headers:     make(map[string]string),
	}

	fmt.Printf("✅ Configuration validation passed - ready for execution\n")
	fmt.Printf("🔍 DEBUG: Created execInfo with %d parameters\n", len(execInfo.Parameters))
	return execInfo, nil
}

// getMapKeys returns keys of a map for debugging
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return []string{"<nil>"}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func getParameterValue(paramName string, commonParams, apiSpecificParams map[string]interface{}) string {
	// Check common parameters first
	val, found := lookupParam(commonParams, paramName)
	if found {
		return val
	}

	// Check API-specific parameters
	val, found = lookupParam(apiSpecificParams, paramName)
	if found {
		return val
	}

	return ""
}

// lookupParam looks up parameter value from configuration node
func lookupParam(params map[string]interface{}, key string) (string, bool) {
	node, ok := params[key]
	if !ok {
		return "", false
	}

	// New YAML format: paramName: {value: "..."}
	if nodeMap, ok := node.(map[string]interface{}); ok {
		if val, ok := nodeMap["value"].(string); ok {
			return val, true
		}
	}

	// Old format or direct string value
	if str, ok := node.(string); ok {
		return str, true
	}

	return "", false
}

// getRequestBodyFieldValue gets request body field value from configuration with improved lookup
func getRequestBodyFieldValue(fieldName, schemaName string, commonBodies, apiSpecificBodies map[string]interface{}) interface{} {
	fmt.Printf("🔍 DEBUG: Looking up body field: %s in schema: %s\n", fieldName, schemaName)

	// Check common request bodies first
	if val, ok := lookupBodyField(commonBodies, schemaName, fieldName); ok {
		fmt.Printf("🔍 DEBUG: Found in common bodies: %s = %v\n", fieldName, val)
		return val
	}

	// Check API-specific request bodies
	if val, ok := lookupBodyField(apiSpecificBodies, schemaName, fieldName); ok {
		fmt.Printf("🔍 DEBUG: Found in API-specific bodies: %s = %v\n", fieldName, val)
		return val
	}

	fmt.Printf("🔍 DEBUG: Body field %s not found in configuration\n", fieldName)
	return nil
}

// lookupBodyField looks up request body field value from configuration
func lookupBodyField(bodies map[string]interface{}, schemaName, fieldName string) (interface{}, bool) {
	body, ok := bodies[schemaName].(map[string]interface{})
	if !ok {
		return nil, false
	}

	properties, ok := body["properties"].(map[string]interface{})
	if !ok {
		return nil, false
	}

	fieldNode, ok := properties[fieldName]
	if !ok {
		return nil, false
	}

	// New format: {value: ...}
	if fieldMap, ok := fieldNode.(map[string]interface{}); ok {
		if val, exists := fieldMap["value"]; exists {
			return val, true
		}
	}

	// Old format or direct value
	if fieldNode != nil {
		return fieldNode, true
	}

	return nil, false
}

// GenerateDefaultParameterValue generates a default value for a parameter
func GenerateDefaultParameterValue(paramName string) string {
	paramLower := strings.ToLower(paramName)

	switch {
	case strings.Contains(paramLower, "ueid") || strings.Contains(paramLower, "supi"):
		return "imsi-208930000000001"
	case strings.Contains(paramLower, "suci"):
		return "suci-0-208-93-0000-0-0-0000000001"
	case strings.Contains(paramLower, "authctx"):
		return "auth-ctx-12345"
	case strings.Contains(paramLower, "session"):
		return "1"
	case strings.Contains(paramLower, "subscription"):
		return "sub-12345"
	case strings.Contains(paramLower, "gpsi"):
		return "msisdn-1234567890"
	case strings.Contains(paramLower, "id"):
		return "example-id-12345"
	default:
		return "example-value"
	}
}

// GetDefaultRequestBodyForType returns a default request body for a given type
func GetDefaultRequestBodyForType(bodyType string) interface{} {
	defaults := map[string]interface{}{
		"AuthenticationInfo": map[string]interface{}{
			"servingNetworkName": "5G:mnc093.mcc208.3gppnetwork.org",
			"resynchronizationInfo": map[string]interface{}{
				"rand": "00112233445566778899aabbccddeeff",
				"auts": "fedcba9876543210",
			},
			"supportedFeatures": "example-features",
		},
		"subscription_data": map[string]interface{}{
			"callbackReference": "http://callback.example.com/notifications",
			"monitoringConfigurations": map[string]interface{}{
				"eventType":     "LOCATION_REPORTING",
				"immediateFlag": true,
			},
			"supportedFeatures": "example-features",
		},
		"pdu_session_info": map[string]interface{}{
			"pduSessionType": "IPV4",
			"sscMode":        "SSC_MODE_1",
			"dnn":            "internet",
			"snssai": map[string]interface{}{
				"sst": 1,
				"sd":  "123456",
			},
		},
		"nf_profile": map[string]interface{}{
			"nfInstanceId":  "example-nf-instance-id",
			"nfType":        "AUSF",
			"nfStatus":      "REGISTERED",
			"ipv4Addresses": []string{"192.168.1.100"},
		},
		"patch_request": []map[string]interface{}{
			{
				"op":    "replace",
				"path":  "/nfStatus",
				"value": "REGISTERED",
			},
		},
	}

	if body, exists := defaults[bodyType]; exists {
		return body
	}

	return map[string]interface{}{
		"data":              "example-data",
		"supportedFeatures": "example-features",
	}
}
