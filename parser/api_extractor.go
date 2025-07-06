package parser

import (
	"regexp"
	"strings"

	"github.com/devuk0204/ctrlbench/types"
)

// title converts first character to uppercase
func title(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// APIExtractor handles API extraction from OpenAPI specs
type APIExtractor struct {
	schemaProcessor *SchemaProcessor
}

// NewAPIExtractor creates a new API extractor
func NewAPIExtractor() *APIExtractor {
	return &APIExtractor{
		schemaProcessor: NewSchemaProcessor(),
	}
}

// ProcessPathItem processes a single path item and extracts APIs
func (ae *APIExtractor) ProcessPathItem(path string, pathItem types.PathItem, service *types.ServiceMetadata, schemas map[string]interface{}) {
	operations := map[string]*types.Operation{
		"GET":     pathItem.Get,
		"POST":    pathItem.Post,
		"PUT":     pathItem.Put,
		"DELETE":  pathItem.Delete,
		"PATCH":   pathItem.Patch,
		"HEAD":    pathItem.Head,
		"OPTIONS": pathItem.Options,
	}

	for method, operation := range operations {
		if operation != nil {
			apiMetadata := ae.CreateAPIMetadata(path, method, operation, schemas)
			service.APIs[apiMetadata.Name] = apiMetadata
		}
	}
}

// CreateAPIMetadata creates API metadata from operation details
func (ae *APIExtractor) CreateAPIMetadata(path, method string, operation *types.Operation, schemas map[string]interface{}) types.APIMetadata {
	api := types.APIMetadata{
		Name:        ae.GetAPIName(operation, method, path),
		Description: ae.GetDescription(operation),
		Methods:     []string{method},
		Path:        path,
		Parameters:  ae.ExtractAllParameters(path, operation),
	}

	// Extract request body information
	requestBodyType, requestBodySchema := ae.schemaProcessor.ExtractRequestBodyInfo(operation, schemas)
	api.RequestBody = requestBodyType
	api.RequestBodySchema = requestBodySchema

	return api
}

// ExtractAllParameters extracts all parameters from path and operation
func (ae *APIExtractor) ExtractAllParameters(path string, operation *types.Operation) []string {
	paramSet := make(map[string]bool)
	var parameters []string

	// Extract path parameters
	pathParams := ae.ExtractPathParameters(path)
	for _, param := range pathParams {
		if !paramSet[param] {
			parameters = append(parameters, param)
			paramSet[param] = true
		}
	}

	// Extract operation parameters
	if operation.Parameters != nil {
		for _, param := range operation.Parameters {
			if ae.IsRelevantParameter(param) && !paramSet[param.Name] {
				parameters = append(parameters, param.Name)
				paramSet[param.Name] = true
			}
		}
	}

	return parameters
}

// ExtractPathParameters extracts parameters from URL path
func (ae *APIExtractor) ExtractPathParameters(path string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)

	var params []string
	for _, match := range matches {
		if len(match) > 1 {
			params = append(params, match[1])
		}
	}

	return params
}

// IsRelevantParameter checks if a parameter is relevant for API execution
func (ae *APIExtractor) IsRelevantParameter(param types.Parameter) bool {
	return param.In == "path" || param.In == "query" || param.In == "header"
}

// GetAPIName generates an API name from operation details
func (ae *APIExtractor) GetAPIName(operation *types.Operation, method, path string) string {
	if operation.OperationID != "" {
		return operation.OperationID
	}

	return ae.GenerateAPIName(method, path)
}

// GetDescription extracts description from operation
func (ae *APIExtractor) GetDescription(operation *types.Operation) string {
	if operation.Description != "" {
		return operation.Description
	}
	if operation.Summary != "" {
		return operation.Summary
	}
	return ""
}

// GenerateAPIName generates API name from method and path
func (ae *APIExtractor) GenerateAPIName(method, path string) string {
	// Clean up the path to create a readable name
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	var nameParts []string

	for _, part := range pathParts {
		// Skip parameter placeholders and version info
		if !strings.Contains(part, "{") && !regexp.MustCompile(`^v\d+`).MatchString(part) {
			// Convert kebab-case and snake_case to PascalCase
			words := regexp.MustCompile(`[-_]+`).Split(part, -1)
			for _, word := range words {
				if word != "" {
					nameParts = append(nameParts, title(word))
				}
			}
		}
	}

	if len(nameParts) == 0 {
		nameParts = []string{"API"}
	}

	// Combine method with path-based name
	methodName := title(strings.ToLower(method))
	return methodName + strings.Join(nameParts, "")
}
