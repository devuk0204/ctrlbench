package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/devuk0204/ctrlbench/types"
	"gopkg.in/yaml.v3"
)

// BuildConfiguration creates configuration.yaml and api_list.yaml
func BuildConfiguration(services map[string]types.ServiceMetadata, nfFilter string) error {
	nfServices := groupServicesByNF(services, nfFilter)

	if len(nfServices) == 0 {
		return fmt.Errorf("no services found for NF filter: %s", nfFilter)
	}

	// Build and write configuration
	config := types.ConfigurationFile{
		UserInputs: buildUserInputSection(nfServices),
	}

	if err := writeConfigurationFile(config, nfFilter); err != nil {
		return err
	}

	// Build and write API list
	apiList := buildAPIList(nfServices)
	return writeAPIListFile(apiList)
}

// groupServicesByNF groups services by NF type
func groupServicesByNF(services map[string]types.ServiceMetadata, nfFilter string) map[string][]types.ServiceMetadata {
	nfServices := GroupServicesByNF(services)

	if nfFilter != "" {
		filteredNFs := make(map[string][]types.ServiceMetadata)
		for nf, serviceList := range nfServices {
			if strings.EqualFold(nf, nfFilter) {
				filteredNFs[nf] = serviceList
			}
		}
		return filteredNFs
	}

	return nfServices
}

// buildAPIList creates API list with tree structure
func buildAPIList(nfServices map[string][]types.ServiceMetadata) types.APIList {
	apiList := make(types.APIList)

	for nf, serviceList := range nfServices {
		nfAPIList := make(map[string]types.ServiceAPIList)

		for _, service := range serviceList {
			serviceName := CleanServiceName(service.Name)
			servicePath := ExtractServicePath(service)
			serviceVersion := ExtractVersionFromPath(servicePath)

			serviceAPIs := make(map[string]types.APIListEntry)

			for apiName, api := range service.APIs {
				method := ExtractMethodFromAPIName(apiName)
				cleanName := CleanAPIName(apiName)

				parameterInfos := buildParameterInfos(api, service)

				requestBodyInfo := buildRequestBodyInfo(api)

				serviceAPIs[cleanName] = types.APIListEntry{
					Path:              api.Path,
					Method:            method,
					Parameters:        parameterInfos,
					RequestBody:       api.RequestBody,
					RequestBodySchema: requestBodyInfo,
				}
			}

			nfAPIList[serviceName] = types.ServiceAPIList{
				Path:    servicePath,
				Version: serviceVersion,
				APIs:    serviceAPIs,
			}
		}

		if len(nfAPIList) > 0 {
			apiList[nf] = nfAPIList
		}
	}

	return apiList
}

// buildUserInputSection - Build user-friendly structure
func buildUserInputSection(nfServices map[string][]types.ServiceMetadata) types.UserInputSection {
	userInputs := types.UserInputSection{
		GlobalSettings:           buildGlobalSettingsSection(),
		NFSettings:               buildNFSettingsSection(nfServices),
		CommonParameters:         buildCommonParametersSection(nfServices),
		CommonRequestBodies:      buildCommonRequestBodiesSection(nfServices),
		APISpecificParameters:    buildAPISpecificParametersSection(nfServices),
		APISpecificRequestBodies: buildAPISpecificRequestBodiesSection(nfServices),
	}

	return userInputs
}

// buildGlobalSettingsSection - Build global settings in user-friendly format
func buildGlobalSettingsSection() map[string]interface{} {
	return map[string]interface{}{
		"# GLOBAL_SETTINGS_HEADER": map[string]interface{}{
			"description": "GLOBAL SETTINGS - Global Configuration",
		},
		"# NRF Configuration": nil,
		"nrf_url": map[string]interface{}{
			"value":       "http://localhost:8000",
			"description": "NRF server URL (e.g., http://nrf-nnrf:8000)",
			"required":    true,
		},
		"requester_nf_type": map[string]interface{}{
			"value":       "AF",
			"description": "Type of requesting NF (AF, AMF, SMF, etc.)",
			"required":    true,
		},
		"timeout_seconds": map[string]interface{}{
			"value":       30,
			"description": "HTTP request timeout in seconds",
			"type":        "integer",
		},
		"retry_count": map[string]interface{}{
			"value":       3,
			"description": "Number of retries on failure",
			"type":        "integer",
		},
		"concurrent_requests": map[string]interface{}{
			"value":       1,
			"description": "Number of concurrent requests",
			"type":        "integer",
		},
		"use_https": map[string]interface{}{
			"value":       false,
			"description": "Whether to use HTTPS",
			"type":        "boolean",
		},
	}
}

// buildNFSettingsSection - Build NF-specific settings in user-friendly format
func buildNFSettingsSection(nfServices map[string][]types.ServiceMetadata) map[string]map[string]interface{} {
	nfSettings := make(map[string]map[string]interface{})

	// Add comments with unique keys
	nfSettings["# NF_SETTINGS_HEADER"] = map[string]interface{}{
		"description": "NF SETTINGS - Individual settings for each NF",
	}

	nfNames := getSortedNFNames(nfServices)
	for _, nf := range nfNames {
		nfSettings[nf] = map[string]interface{}{
			"enabled": map[string]interface{}{
				"value":       true,
				"description": fmt.Sprintf("Enable %s NF", nf),
				"type":        "boolean",
			},
			"custom_headers": map[string]interface{}{
				"Content-Type": map[string]interface{}{
					"value":       "application/json",
					"description": "Request Content-Type header",
				},
				"Accept": map[string]interface{}{
					"value":       "application/json",
					"description": "Response Accept header",
				},
			},
		}
	}

	return nfSettings
}

// buildCommonParametersSection - Build common parameters in user-friendly format
func buildCommonParametersSection(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	extractedParams := extractParametersFromSpecs(nfServices)
	commonParams, _ := separateCommonParameters(extractedParams)

	result := map[string]interface{}{
		"# COMMON_PARAMETERS_HEADER": map[string]interface{}{
			"description": "COMMON PARAMETERS - Parameters used across multiple APIs",
		},
		"# Instructions": map[string]interface{}{
			"description": "Please modify the values below according to your environment",
		},
	}

	// Group parameters by category
	categories := groupParametersByCategory(commonParams)

	categoryIndex := 1
	for category, params := range categories {
		categoryKey := fmt.Sprintf("# Category_%d_%s", categoryIndex, strings.ReplaceAll(category, " ", "_"))
		result[categoryKey] = map[string]interface{}{
			"description": category,
		}

		for paramName, paramInfo := range params {
			result[paramName] = formatParameterForUser(paramInfo)
		}

		categoryIndex++
	}

	return result
}

// buildCommonRequestBodiesSection - Build common request bodies in user-friendly format
func buildCommonRequestBodiesSection(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	extractedBodies := extractRequestBodiesFromSpecs(nfServices)
	commonBodies, _ := separateCommonRequestBodies(extractedBodies)

	result := map[string]interface{}{
		"# COMMON_REQUEST_BODIES_HEADER": map[string]interface{}{
			"description": "COMMON REQUEST BODIES - Request bodies used across multiple APIs",
		},
		"# Instructions": map[string]interface{}{
			"description": "Please enter actual values in the 'value' field of each property",
		},
	}

	for bodyName, bodyInfo := range commonBodies {
		result[bodyName] = formatRequestBodyForUser(bodyInfo)
	}

	return result
}

// buildAPISpecificParametersSection - Build API-specific parameters
func buildAPISpecificParametersSection(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	extractedParams := extractParametersFromSpecs(nfServices)
	_, apiSpecificParams := separateCommonParameters(extractedParams)

	result := map[string]interface{}{
		"# API_SPECIFIC_PARAMETERS_HEADER": map[string]interface{}{
			"description": "API-SPECIFIC PARAMETERS - Parameters used by specific APIs",
		},
	}

	for paramName, paramInfo := range apiSpecificParams {
		result[paramName] = formatParameterForUser(paramInfo)
	}

	return result
}

// buildAPISpecificRequestBodiesSection - Build API-specific request bodies
func buildAPISpecificRequestBodiesSection(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	extractedBodies := extractRequestBodiesFromSpecs(nfServices)
	_, apiSpecificBodies := separateCommonRequestBodies(extractedBodies)

	result := map[string]interface{}{
		"# API_SPECIFIC_REQUEST_BODIES_HEADER": map[string]interface{}{
			"description": "API-SPECIFIC REQUEST BODIES - Request bodies used by specific APIs",
		},
	}

	for bodyName, bodyInfo := range apiSpecificBodies {
		result[bodyName] = formatRequestBodyForUser(bodyInfo)
	}

	return result
}

// buildParameterInfos - Build parameter information with required flags and location
func buildParameterInfos(api types.APIMetadata, service types.ServiceMetadata) []types.ParamMeta {
	var paramInfos []types.ParamMeta

	if service.OpenAPISpec != nil {
		// Extract parameter info from OpenAPI spec
		for _, pathItem := range service.OpenAPISpec.Paths {
			if pathItem.Get != nil && matchesAPI(pathItem.Get, api) {
				paramInfos = extractParameterInfosFromOperation(pathItem.Get)
			} else if pathItem.Post != nil && matchesAPI(pathItem.Post, api) {
				paramInfos = extractParameterInfosFromOperation(pathItem.Post)
			} else if pathItem.Put != nil && matchesAPI(pathItem.Put, api) {
				paramInfos = extractParameterInfosFromOperation(pathItem.Put)
			} else if pathItem.Delete != nil && matchesAPI(pathItem.Delete, api) {
				paramInfos = extractParameterInfosFromOperation(pathItem.Delete)
			} else if pathItem.Patch != nil && matchesAPI(pathItem.Patch, api) {
				paramInfos = extractParameterInfosFromOperation(pathItem.Patch)
			}
		}
	}

	// If not found in OpenAPI, create default parameter info
	if len(paramInfos) == 0 {
		for _, paramName := range api.Parameters {
			paramInfos = append(paramInfos, types.ParamMeta{
				Name:     paramName,
				Required: IsPathParameter(paramName, api.Path), // path parameters are required
				Type:     "string",
				In:       inferParameterLocation(paramName, api.Path),
			})
		}
	}

	return paramInfos
}

// buildRequestBodyInfo - request body에 required 정보 포함
func buildRequestBodyInfo(api types.APIMetadata) types.BodyMeta {
	if api.RequestBody == "" {
		return types.BodyMeta{}
	}

	requestBodyInfo := types.BodyMeta{
		SchemaName: api.RequestBody,
	}

	// RequestBodySchema에서 required 필드 추출
	if api.RequestBodySchema != nil {
		if required, exists := api.RequestBodySchema["required"]; exists {
			if requiredSlice, ok := required.([]string); ok {
				requestBodyInfo.RequiredFields = requiredSlice
			} else if requiredInterface, ok := required.([]interface{}); ok {
				var requiredFields []string
				for _, field := range requiredInterface {
					if fieldStr, ok := field.(string); ok {
						requiredFields = append(requiredFields, fieldStr)
					}
				}
				requestBodyInfo.RequiredFields = requiredFields
			}
		}

		// 스키마 정보도 포함 (선택사항)
		requestBodyInfo.Schema = api.RequestBodySchema
	}

	return requestBodyInfo
}

// extractParameterInfosFromOperation - Operation에서 파라미터 정보 추출
func extractParameterInfosFromOperation(operation *types.Operation) []types.ParamMeta {
	var paramInfos []types.ParamMeta

	for _, param := range operation.Parameters {
		paramInfos = append(paramInfos, types.ParamMeta{
			Name:     param.Name,
			Required: param.Required,
			Type:     getSchemaType(param.Schema),
			In:       param.In,
		})
	}

	return paramInfos
}

// matchesAPI - Operation이 API와 일치하는지 확인
func matchesAPI(operation *types.Operation, api types.APIMetadata) bool {
	// API 이름에서 메서드 부분 제거하고 비교
	cleanAPIName := CleanAPIName(api.Name)
	return operation.OperationID == cleanAPIName
}

// inferParameterLocation - Infer parameter location (path or query)
func inferParameterLocation(paramName, path string) string {
	if IsPathParameter(paramName, path) {
		return "path"
	}
	return "query"
}

// groupParametersByCategory - Group parameters by category
func groupParametersByCategory(params map[string]interface{}) map[string]map[string]interface{} {
	categories := map[string]map[string]interface{}{
		"User Identifiers":       make(map[string]interface{}),
		"Authentication Related": make(map[string]interface{}),
		"Session Related":        make(map[string]interface{}),
		"Others":                 make(map[string]interface{}),
	}

	for paramName, paramInfo := range params {
		paramLower := strings.ToLower(paramName)

		switch {
		case strings.Contains(paramLower, "ue") || strings.Contains(paramLower, "supi") || strings.Contains(paramLower, "gpsi"):
			categories["User Identifiers"][paramName] = paramInfo
		case strings.Contains(paramLower, "auth"):
			categories["Authentication Related"][paramName] = paramInfo
		case strings.Contains(paramLower, "session"):
			categories["Session Related"][paramName] = paramInfo
		default:
			categories["Others"][paramName] = paramInfo
		}
	}

	// Remove empty categories
	for category, params := range categories {
		if len(params) == 0 {
			delete(categories, category)
		}
	}

	return categories
}

// formatParameterForUser - Format parameter in user-friendly way
func formatParameterForUser(paramInfo interface{}) map[string]interface{} {
	if paramMap, ok := paramInfo.(map[string]interface{}); ok {
		return map[string]interface{}{
			"value":       "", // Empty field for user input
			"description": paramMap["description"],
			"type":        paramMap["type"],
			"required":    paramMap["required"],
			"example":     paramMap["example"],
		}
	}

	return map[string]interface{}{
		"value":       "",
		"description": "Parameter information",
		"type":        "string",
		"required":    false,
		"example":     "example-value",
	}
}

// formatRequestBodyForUser - Format request body in user-friendly way
func formatRequestBodyForUser(bodyInfo interface{}) map[string]interface{} {
	if bodyMap, ok := bodyInfo.(map[string]interface{}); ok {
		result := map[string]interface{}{
			"description":     bodyMap["description"],
			"type":            bodyMap["type"],
			"required_fields": bodyMap["required"],
		}

		if properties, exists := bodyMap["properties"].(map[string]interface{}); exists {
			userProperties := make(map[string]interface{})

			for propName, propInfo := range properties {
				if propMap, ok := propInfo.(map[string]interface{}); ok {
					userProperties[propName] = map[string]interface{}{
						"value":       "", // Empty field for user input
						"description": propMap["description"],
						"type":        propMap["type"],
						"required":    propMap["required"],
						"example":     propMap["example"],
					}
				}
			}

			result["properties"] = userProperties
		}

		return result
	}

	return map[string]interface{}{
		"description": "Request body information",
		"type":        "object",
		"properties": map[string]interface{}{
			"data": map[string]interface{}{
				"value":       "",
				"description": "Data field",
				"type":        "string",
				"required":    false,
				"example":     "example-data",
			},
		},
	}
}

// extractParametersFromSpecs - Extract actual parameter information from OpenAPI schemas
func extractParametersFromSpecs(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	params := make(map[string]interface{})

	for _, serviceList := range nfServices {
		for _, service := range serviceList {
			if service.OpenAPISpec != nil {
				extractParametersFromOpenAPISpec(service.OpenAPISpec, params)
			}
		}
	}

	return params
}

// extractParametersFromOpenAPISpec - Extract parameters from OpenAPI spec
func extractParametersFromOpenAPISpec(spec *types.OpenAPISpec, params map[string]interface{}) {
	for _, pathItem := range spec.Paths {
		operations := []*types.Operation{
			pathItem.Get, pathItem.Post, pathItem.Put,
			pathItem.Delete, pathItem.Patch, pathItem.Head, pathItem.Options,
		}

		for _, operation := range operations {
			if operation != nil {
				for _, param := range operation.Parameters {
					if _, exists := params[param.Name]; !exists {
						params[param.Name] = map[string]interface{}{
							"description": getParameterDescription(param),
							"required":    param.Required,
							"type":        getSchemaType(param.Schema),
							"in":          param.In,
							"example":     generateExampleFromSchema(param.Schema),
							"value":       "", // User input field
						}
					}
				}
			}
		}
	}
}

// getParameterDescription - Get parameter description
func getParameterDescription(param types.Parameter) string {
	if param.Description != "" {
		return param.Description
	}
	return fmt.Sprintf("Parameter: %s", param.Name)
}

// getSchemaType - Extract type from schema
func getSchemaType(schema types.Schema) string {
	if schema.Type != "" {
		return schema.Type
	}
	if schema.Ref != "" {
		return "string" // Default for $ref
	}
	return "string"
}

// generateExampleFromSchema - Generate example value from schema
func generateExampleFromSchema(schema types.Schema) interface{} {
	switch schema.Type {
	case "string":
		if schema.Format == "uuid" {
			return "550e8400-e29b-41d4-a716-446655440000"
		}
		return "example-string"
	case "integer":
		return 0
	case "boolean":
		return false
	case "array":
		return []interface{}{"example-item"}
	case "object":
		return map[string]interface{}{"example": "value"}
	default:
		return "example-value"
	}
}

// extractRequestBodiesFromSpecs - Extract actual request body schemas from OpenAPI
func extractRequestBodiesFromSpecs(nfServices map[string][]types.ServiceMetadata) map[string]interface{} {
	bodies := make(map[string]interface{})

	for _, serviceList := range nfServices {
		for _, service := range serviceList {
			if service.OpenAPISpec != nil {
				extractRequestBodiesFromOpenAPISpec(service.OpenAPISpec, bodies)
			}
		}
	}

	return bodies
}

// extractRequestBodiesFromOpenAPISpec - Extract request bodies from OpenAPI spec
func extractRequestBodiesFromOpenAPISpec(spec *types.OpenAPISpec, bodies map[string]interface{}) {
	for _, pathItem := range spec.Paths {
		operations := []*types.Operation{
			pathItem.Post, pathItem.Put, pathItem.Patch, // Methods with request body
		}

		for _, operation := range operations {
			if operation != nil && operation.RequestBody != nil {
				for contentType, mediaType := range operation.RequestBody.Content {
					if strings.Contains(contentType, "json") {
						schemaName := extractSchemaNameFromMediaType(mediaType)
						if schemaName != "" && spec.Components != nil {
							if schemaDef, exists := spec.Components.Schemas[schemaName]; exists {
								bodies[schemaName] = convertSchemaDefinitionToTemplate(schemaName, schemaDef)
							}
						}
					}
				}
			}
		}
	}
}

// extractSchemaNameFromMediaType - Extract schema name from MediaType
func extractSchemaNameFromMediaType(mediaType types.MediaType) string {
	if mediaType.Schema.Ref != "" {
		parts := strings.Split(mediaType.Schema.Ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return ""
}

// convertSchemaDefinitionToTemplate - Convert schema definition to user-friendly template
func convertSchemaDefinitionToTemplate(schemaName string, schemaDef types.SchemaDefinition) map[string]interface{} {
	template := map[string]interface{}{
		"schema_name": schemaName, // Schema name
		"description": getDescriptionWithFallback(schemaDef.Description, schemaName),
		"type":        schemaDef.Type,
		"required":    schemaDef.Required,
		"properties":  make(map[string]interface{}),
	}

	if schemaDef.Properties != nil {
		properties := make(map[string]interface{})

		for propName, propDef := range schemaDef.Properties {
			properties[propName] = convertPropertyToTemplate(propName, propDef, schemaDef.Required)
		}

		template["properties"] = properties
	}

	return template
}

// getDescriptionWithFallback - Get description with fallback to schema name
func getDescriptionWithFallback(description, schemaName string) string {
	if description != "" {
		return description
	}
	return fmt.Sprintf("Schema: %s", schemaName)
}

// convertPropertyToTemplate - Convert property to user-friendly template
func convertPropertyToTemplate(propName string, propDef interface{}, requiredFields []string) map[string]interface{} {
	isRequired := contains(requiredFields, propName)

	// Convert propDef to map
	propMap, ok := propDef.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"description": fmt.Sprintf("Property: %s", propName),
			"type":        "string",
			"required":    isRequired,
			"example":     "example-value",
			"value":       "",
		}
	}

	template := map[string]interface{}{
		"required": isRequired,
		"value":    "", // User input field
	}

	// Extract type
	if propType, exists := propMap["type"]; exists {
		template["type"] = propType
		template["example"] = generateExampleByType(propType.(string))
	} else {
		template["type"] = "string"
		template["example"] = "example-value"
	}

	// Extract description
	if description, exists := propMap["description"]; exists {
		template["description"] = description
	} else {
		template["description"] = fmt.Sprintf("Property: %s", propName)
	}

	// Handle $ref
	if ref, exists := propMap["$ref"]; exists {
		template["description"] = fmt.Sprintf("Reference to %s", extractSchemaNameFromRef(ref.(string)))
		template["type"] = "object"
		template["example"] = map[string]interface{}{"ref": ref}
	}

	// Handle enum
	if enum, exists := propMap["enum"]; exists {
		template["enum"] = enum
		if enumSlice, ok := enum.([]interface{}); ok && len(enumSlice) > 0 {
			template["example"] = enumSlice[0]
		}
	}

	// Handle format
	if format, exists := propMap["format"]; exists {
		template["format"] = format
		template["example"] = generateExampleByFormat(format.(string))
	}

	return template
}

// generateExampleByType - Generate example value by type
func generateExampleByType(propType string) interface{} {
	switch propType {
	case "string":
		return "example-string"
	case "integer":
		return 0
	case "number":
		return 0.0
	case "boolean":
		return false
	case "array":
		return []interface{}{"example-item"}
	case "object":
		return map[string]interface{}{"example": "value"}
	default:
		return "example-value"
	}
}

// generateExampleByFormat - Generate example value by format
func generateExampleByFormat(format string) interface{} {
	switch format {
	case "uuid":
		return "550e8400-e29b-41d4-a716-446655440000"
	case "date-time":
		return "2025-01-01T00:00:00Z"
	case "date":
		return "2025-01-01"
	case "email":
		return "example@example.com"
	case "uri":
		return "http://example.com"
	case "ipv4":
		return "192.168.1.1"
	case "ipv6":
		return "2001:db8::1"
	default:
		return "example-value"
	}
}

// extractSchemaNameFromRef - Extract schema name from $ref
func extractSchemaNameFromRef(ref string) string {
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ref
}

// contains - Check if slice contains item
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// separateCommonParameters - Classify based on actual usage frequency
func separateCommonParameters(extractedParams map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	commonParams := make(map[string]interface{})
	apiSpecificParams := make(map[string]interface{})

	// Calculate parameter usage frequency
	paramFrequency := make(map[string]int)
	for paramName := range extractedParams {
		frequency := calculateParameterFrequency(paramName)
		paramFrequency[paramName] = frequency
	}

	// Classify parameters with high frequency as common, low frequency as API-specific
	for paramName, paramInfo := range extractedParams {
		if paramFrequency[paramName] >= 3 { // Common if used in 3+ APIs
			commonParams[paramName] = paramInfo
		} else {
			apiSpecificParams[paramName] = paramInfo
		}
	}

	return commonParams, apiSpecificParams
}

// calculateParameterFrequency - Calculate parameter usage frequency
func calculateParameterFrequency(paramName string) int {
	paramLower := strings.ToLower(paramName)

	// Commonly used parameters
	commonPatterns := []string{"ueid", "supi", "authctx", "session", "subscription"}

	for _, pattern := range commonPatterns {
		if strings.Contains(paramLower, pattern) {
			return 5 // High frequency
		}
	}

	return 1 // Low frequency
}

// separateCommonRequestBodies - Classify request bodies
func separateCommonRequestBodies(extractedBodies map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	commonBodies := make(map[string]interface{})
	apiSpecificBodies := make(map[string]interface{})

	// Calculate request body usage frequency
	bodyFrequency := make(map[string]int)
	for bodyName := range extractedBodies {
		frequency := calculateBodyFrequency(bodyName)
		bodyFrequency[bodyName] = frequency
	}

	// Classify request bodies with high frequency as common, low frequency as API-specific
	for bodyName, bodyInfo := range extractedBodies {
		if bodyFrequency[bodyName] >= 2 { // Common if used in 2+ APIs
			commonBodies[bodyName] = bodyInfo
		} else {
			apiSpecificBodies[bodyName] = bodyInfo
		}
	}

	return commonBodies, apiSpecificBodies
}

// calculateBodyFrequency - Calculate request body usage frequency
func calculateBodyFrequency(bodyName string) int {
	bodyLower := strings.ToLower(bodyName)

	// Commonly used request bodies
	commonPatterns := []string{"auth", "subscription", "session", "context"}

	for _, pattern := range commonPatterns {
		if strings.Contains(bodyLower, pattern) {
			return 3 // High frequency
		}
	}

	return 1 // Low frequency
}

func getSortedNFNames(nfServices map[string][]types.ServiceMetadata) []string {
	nfNames := make([]string, 0, len(nfServices))
	for nf := range nfServices {
		nfNames = append(nfNames, nf)
	}
	sort.Strings(nfNames)
	return nfNames
}

// File writing functions
func writeAPIListFile(apiList types.APIList) error {
	openapiDir := "./openapi"
	if err := os.MkdirAll(openapiDir, 0755); err != nil {
		return fmt.Errorf("failed to create openapi directory: %w", err)
	}

	filename := filepath.Join(openapiDir, "api_list.yaml")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create api_list file: %w", err)
	}
	defer file.Close()

	// Add header comments
	header := `# =============================================================================
# API LIST - 5G Core Network Functions API Catalog
# =============================================================================
# This file is automatically generated from OpenAPI specifications.
# It contains available APIs organized by NF (Network Function).
#
# Structure:
#   NF_NAME:
#     SERVICE_NAME:
#       path: /service-base-path/version
#       version: v1
#       apis:
#         API_NAME:
#           path: /api-specific-path
#           method: HTTP_METHOD
#           parameters: [list of parameters]
#           request_body: request_body_schema_name
# =============================================================================

`

	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)

	if err := encoder.Encode(apiList); err != nil {
		return fmt.Errorf("failed to write api_list: %w", err)
	}

	fmt.Printf("✅ API list file created: %s\n", filename)
	return nil
}

func writeConfigurationFile(config types.ConfigurationFile, nfFilter string) error {
	filename := "configuration.yaml"
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create configuration file: %w", err)
	}
	defer file.Close()

	// Add header comments
	header := `# =============================================================================
# 5G BENCHMARK CONFIGURATION FILE
# =============================================================================
# This is the configuration file for 5G Core Network Functions API benchmark tool.
#
# Usage:
# 1. Modify NRF URL and basic settings in global_settings
# 2. Adjust individual NF settings in nf_settings  
# 3. Enter common parameter values in common_parameters
# 4. Fill in request body field values in common_request_bodies
#
# Important Notes:
# - Only enter actual values in the 'value' fields
# - Fields with required=true must have values
# - 'example' fields are for reference only, do not modify them
# =============================================================================

`

	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)

	if err := encoder.Encode(&config); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	fmt.Printf("✅ Configuration file created: %s\n", filename)
	if nfFilter != "" {
		fmt.Printf("📋 Generated configuration for NF: %s\n", nfFilter)
	} else {
		fmt.Printf("📋 Generated configuration for all NFs\n")
	}

	return nil
}
