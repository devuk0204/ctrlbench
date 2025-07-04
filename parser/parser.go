package parser

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/devuk0204/ctrlbench/types"

	"gopkg.in/yaml.v3"
)

// ParseOpenAPIDir parses OpenAPI YAML files and returns service metadata
func ParseOpenAPIDir(dirPath string) (map[string]types.ServiceMetadata, error) {
	services := make(map[string]types.ServiceMetadata)

	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read openapi dir: %w", err)
	}

	for _, fi := range files {
		if !isYAMLFile(fi.Name()) || fi.IsDir() {
			continue
		}

		spec, err := loadOpenAPISpec(filepath.Join(dirPath, fi.Name()))
		if err != nil {
			fmt.Printf("⚠️  Failed to parse %s: %v\n", fi.Name(), err)
			continue
		}

		processOpenAPISpec(spec, services)
	}

	return services, nil
}

// isYAMLFile checks if file has YAML extension
func isYAMLFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".yaml" || ext == ".yml"
}

// loadOpenAPISpec loads and parses OpenAPI spec from file
func loadOpenAPISpec(filePath string) (*types.OpenAPISpec, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var spec types.OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// processOpenAPISpec processes a single OpenAPI spec
func processOpenAPISpec(spec *types.OpenAPISpec, services map[string]types.ServiceMetadata) {
	nfName := extractNFName(spec)
	serviceName := cleanServiceName(extractServiceName(spec))

	service := getOrCreateService(services, serviceName, nfName, spec)

	// Extract schemas for request body resolution
	schemas := extractSchemas(spec)

	// Process all paths and operations
	for path, pathItem := range spec.Paths {
		processPathItem(path, pathItem, service, schemas)
	}

	services[serviceName] = *service
}

// getOrCreateService gets existing service or creates new one
func getOrCreateService(services map[string]types.ServiceMetadata, serviceName, nfName string, spec *types.OpenAPISpec) *types.ServiceMetadata {
	if service, exists := services[serviceName]; exists {
		service.OpenAPISpec = spec
		return &service
	}

	return &types.ServiceMetadata{
		Name:        serviceName,
		Description: extractServiceDescription(spec),
		APIs:        make(map[string]types.APIMetadata),
		NF:          nfName,
		OpenAPISpec: spec,
	}
}

// processPathItem processes a single path item with all operations
func processPathItem(path string, pathItem types.PathItem, service *types.ServiceMetadata, schemas map[string]interface{}) {
	operations := map[string]*types.Operation{
		"GET": pathItem.Get, "POST": pathItem.Post, "PUT": pathItem.Put,
		"DELETE": pathItem.Delete, "PATCH": pathItem.Patch,
		"HEAD": pathItem.Head, "OPTIONS": pathItem.Options,
	}

	for method, operation := range operations {
		if operation != nil {
			apiMetadata := createAPIMetadata(path, method, operation, schemas)
			service.APIs[apiMetadata.Name] = apiMetadata
		}
	}
}

// createAPIMetadata creates API metadata from operation
func createAPIMetadata(path, method string, operation *types.Operation, schemas map[string]interface{}) types.APIMetadata {
	apiName := getAPIName(operation, method, path)
	requestBodyType, requestBodySchema := extractRequestBodyInfo(operation, schemas)

	return types.APIMetadata{
		Name:              fmt.Sprintf("%s [%s]", apiName, method),
		Description:       getDescription(operation),
		Methods:           []string{method},
		Path:              path,
		Parameters:        extractAllParameters(path, operation),
		RequestBody:       requestBodyType,
		RequestBodySchema: requestBodySchema,
	}
}

// extractSchemas extracts all schemas from OpenAPI components
func extractSchemas(spec *types.OpenAPISpec) map[string]interface{} {
	schemas := make(map[string]interface{})

	if spec.Components != nil && spec.Components.Schemas != nil {
		for name, schema := range spec.Components.Schemas {
			schemas[name] = convertSchemaToMap(schema)
		}
	}

	return schemas
}

// convertSchemaToMap converts SchemaDefinition to map
func convertSchemaToMap(schema types.SchemaDefinition) map[string]interface{} {
	result := make(map[string]interface{})

	if schema.Description != "" {
		result["description"] = schema.Description
	}
	if schema.Type != "" {
		result["type"] = schema.Type
	}
	if len(schema.Properties) > 0 {
		result["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}
	if schema.Ref != "" {
		result["$ref"] = schema.Ref
	}

	return result
}

// extractRequestBodyInfo extracts request body type and schema
func extractRequestBodyInfo(operation *types.Operation, schemas map[string]interface{}) (string, map[string]interface{}) {
	if operation == nil || operation.RequestBody == nil {
		return "", nil
	}

	for contentType, mediaType := range operation.RequestBody.Content {
		if strings.Contains(contentType, "json") {
			return determineRequestBodyType(mediaType, operation, schemas)
		}
	}

	return "", nil
}

// determineRequestBodyType determines request body type and schema
func determineRequestBodyType(mediaType types.MediaType, operation *types.Operation, schemas map[string]interface{}) (string, map[string]interface{}) {
	if mediaType.Schema.Ref != "" {
		schemaName := extractSchemaNameFromRef(mediaType.Schema.Ref)
		if schemaName != "" {
			if schema, exists := schemas[schemaName]; exists {
				if schemaMap, ok := schema.(map[string]interface{}); ok {
					return schemaName, schemaMap
				}
			}
			return schemaName, nil
		}
	}

	if mediaType.Schema.Type == "array" {
		return "patch_request", map[string]interface{}{
			"type":        "array",
			"description": "JSON Patch array for updates",
		}
	}

	return inferRequestBodyType(operation), nil
}

// extractAllParameters extracts all parameters from path and operation
func extractAllParameters(path string, operation *types.Operation) []string {
	paramSet := make(map[string]bool)
	var params []string

	// Extract from path
	for _, param := range extractPathParameters(path) {
		if !paramSet[param] {
			params = append(params, param)
			paramSet[param] = true
		}
	}

	// Extract from operation parameters
	if operation != nil {
		for _, param := range operation.Parameters {
			if isRelevantParameter(param) && !paramSet[param.Name] {
				params = append(params, param.Name)
				paramSet[param.Name] = true
			}
		}
	}

	return params
}

// extractPathParameters extracts parameters from URL path
func extractPathParameters(path string) []string {
	var params []string
	segments := strings.Split(strings.Trim(path, "/"), "/")

	for _, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			paramName := strings.Trim(segment, "{}")
			params = append(params, paramName)
		}
	}

	return params
}

// isRelevantParameter checks if parameter is relevant for API execution
func isRelevantParameter(param types.Parameter) bool {
	return param.In == "path" || param.In == "query" || param.In == "header"
}

// Utility functions for name extraction and processing
func getAPIName(operation *types.Operation, method, path string) string {
	if operation.OperationID != "" {
		return operation.OperationID
	}
	return generateAPIName(method, path)
}

func getDescription(operation *types.Operation) string {
	if operation.Description != "" {
		return operation.Description
	}
	if operation.Summary != "" {
		return operation.Summary
	}
	return "API operation"
}

func generateAPIName(method, path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	var validSegments []string

	for _, segment := range segments {
		if segment != "" && !(strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")) {
			validSegments = append(validSegments, segment)
		}
	}

	if len(validSegments) == 0 {
		return strings.Title(strings.ToLower(method))
	}

	combinedName := strings.Join(validSegments, " ")
	combinedName = strings.Title(strings.ReplaceAll(combinedName, "-", " "))

	switch method {
	case "GET":
		if strings.Contains(path, "{") {
			return "Get" + combinedName
		}
		return "List" + combinedName
	case "POST":
		return "Create" + combinedName
	case "PUT", "PATCH":
		return "Update" + combinedName
	case "DELETE":
		return "Delete" + combinedName
	default:
		return strings.Title(strings.ToLower(method)) + combinedName
	}
}

// NF name extraction with comprehensive mapping
func extractNFName(spec *types.OpenAPISpec) string {
	candidates := []string{
		extractNFFromTitle(spec.Info.Title),
		extractNFFromDescription(spec.Info.Description),
		extractNFFromServers(spec.Servers),
		extractNFFromTags(spec.Tags),
	}

	for _, candidate := range candidates {
		if normalized := normalizeNFName(candidate); normalized != "UNKNOWN" {
			return normalized
		}
	}

	return "UNKNOWN"
}

func extractNFFromTitle(title string) string {
	title = strings.ToUpper(strings.TrimSpace(title))
	nfPatterns := []string{"AUSF", "AMF", "SMF", "PCF", "UDR", "UDM", "NRF", "NSSF", "BSF", "CHF", "NEF", "AF", "UPF"}

	for _, pattern := range nfPatterns {
		if strings.Contains(title, pattern) {
			return pattern
		}
	}

	words := strings.Fields(title)
	if len(words) > 0 && len(words[0]) <= 5 && strings.ToUpper(words[0]) == words[0] {
		return words[0]
	}

	return ""
}

func extractNFFromDescription(desc string) string {
	if desc == "" {
		return ""
	}

	words := strings.Fields(strings.TrimSpace(desc))
	if len(words) == 0 {
		return ""
	}

	firstWord := strings.ToUpper(words[0])
	if firstWord == "UNIFIED" && len(words) > 1 {
		secondWord := strings.ToUpper(words[1])
		if secondWord == "DATA" {
			return "UDR"
		}
	}

	return firstWord
}

func extractNFFromServers(servers []types.Server) string {
	if len(servers) == 0 {
		return ""
	}

	url := servers[0].Url
	if strings.HasPrefix(url, "{") {
		if idx := strings.Index(url, "}"); idx != -1 && len(url) > idx+1 {
			url = url[idx+1:]
		}
	}

	parts := strings.Split(url, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "n") && len(part) > 1 {
			nfPart := strings.ToUpper(part[1:])
			if idx := strings.Index(nfPart, "-"); idx > 0 {
				return nfPart[:idx]
			}
		}
	}

	return ""
}

func extractNFFromTags(tags []types.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(tags[0].Name))
}

func normalizeNFName(nfName string) string {
	if nfName == "" {
		return "UNKNOWN"
	}

	nfName = strings.ToUpper(strings.TrimSpace(nfName))

	// Remove common suffixes
	suffixes := []string{" API", " SERVICE", "API", "SERVICE", " FUNCTION"}
	for _, suffix := range suffixes {
		nfName = strings.TrimSuffix(nfName, suffix)
	}
	nfName = strings.TrimSpace(nfName)

	// Comprehensive NF mapping
	mapping := map[string]string{
		"AUTHENTICATION SERVER FUNCTION": "AUSF",
		"ACCESS AND MOBILITY MANAGEMENT": "AMF",
		"SESSION MANAGEMENT FUNCTION":    "SMF",
		"POLICY CONTROL FUNCTION":        "PCF",
		"UNIFIED DATA REPOSITORY":        "UDR",
		"UNIFIED DATA MANAGEMENT":        "UDM",
		"NETWORK REPOSITORY FUNCTION":    "NRF",
		"NETWORK SLICE SELECTION":        "NSSF",
		"BINDING SUPPORT FUNCTION":       "BSF",
		"CHARGING FUNCTION":              "CHF",
		"NETWORK EXPOSURE FUNCTION":      "NEF",
		"APPLICATION FUNCTION":           "AF",
		"USER PLANE FUNCTION":            "UPF",
	}

	if canonical, exists := mapping[nfName]; exists {
		return canonical
	}

	// Valid 5G NF list
	validNFs := []string{
		"AMF", "SMF", "UPF", "PCF", "UDM", "UDR", "AUSF", "NRF", "NSSF",
		"NEF", "AF", "CHF", "BSF", "NWDAF", "UCMF", "UDSF", "NSSAAF",
		"SEPP", "N3IWF", "TNGF", "W-AGF", "TWIF", "GMLC", "LMF",
		"SMSF", "5G-EIR", "SCP", "SEAF",
	}

	for _, validNF := range validNFs {
		if nfName == validNF || strings.Contains(nfName, validNF) {
			return validNF
		}
	}

	return "UNKNOWN"
}

// Service name extraction and cleaning
func extractServiceName(spec *types.OpenAPISpec) string {
	if serviceName := extractServiceNameFromSecurity(spec); serviceName != "" {
		return serviceName
	}
	return extractServiceNameFromTitle(spec.Info.Title)
}

func extractServiceNameFromSecurity(spec *types.OpenAPISpec) string {
	if len(spec.Security) == 0 {
		return ""
	}

	for _, securityItem := range spec.Security {
		for key, scopes := range securityItem {
			if key == "oAuth2ClientCredentials" && len(scopes) > 0 {
				scope := strings.ToUpper(strings.TrimSpace(scopes[0]))
				if scope != "" {
					return scope + "Service"
				}
			}
		}
	}

	return ""
}

func extractServiceNameFromTitle(title string) string {
	if title == "" {
		return "UnknownService"
	}

	serviceName := strings.ReplaceAll(strings.TrimSpace(title), "'", "")
	if !strings.HasSuffix(serviceName, "Service") {
		serviceName += "Service"
	}

	return serviceName
}

func cleanServiceName(serviceName string) string {
	if strings.HasSuffix(serviceName, "Service") {
		return strings.TrimSuffix(serviceName, "Service")
	}
	return serviceName
}

func extractServiceDescription(spec *types.OpenAPISpec) string {
	if spec.Info.Description == "" {
		return "API Service"
	}

	description := strings.TrimSpace(spec.Info.Description)
	lines := strings.Split(description, "\n")
	var cleanLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "©") || strings.Contains(line, "All rights reserved") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	if len(cleanLines) == 0 {
		return "API Service"
	}

	cleanDescription := strings.Join(cleanLines, " ")
	re := regexp.MustCompile(`(?:[.!?])\s+`)
	sentences := re.Split(cleanDescription, -1)

	if len(sentences) > 0 && strings.TrimSpace(sentences[0]) != "" {
		return strings.TrimSpace(sentences[0])
	}

	return cleanDescription
}

// Schema and request body processing
func extractSchemaNameFromRef(ref string) string {
	if strings.Contains(ref, "#/components/schemas/") {
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return ""
}

func inferRequestBodyType(operation *types.Operation) string {
	combined := strings.ToLower(operation.OperationID + " " + operation.Summary)

	switch {
	case strings.Contains(combined, "register") && strings.Contains(combined, "nf"):
		return "nf_profile"
	case strings.Contains(combined, "update") && strings.Contains(combined, "nf"):
		return "patch_request"
	case strings.Contains(combined, "auth"):
		return "authentication_info"
	case strings.Contains(combined, "subscription"):
		return "subscription_data"
	case strings.Contains(combined, "session") || strings.Contains(combined, "pdu"):
		return "pdu_session_info"
	default:
		return "default_request"
	}
}

// GetDefaultServices returns default example services
func GetDefaultServices() map[string]types.ServiceMetadata {
	return map[string]types.ServiceMetadata{
		"UserService": {
			Name:        "UserService",
			Description: "User management API",
			NF:          "DEMO",
			APIs: map[string]types.APIMetadata{
				"GetUser [GET]": {
					Name:        "GetUser [GET]",
					Description: "Get user by ID",
					Methods:     []string{"GET"},
					Path:        "/users/{id}",
					Parameters:  []string{"id"},
					RequestBody: "",
				},
			},
		},
	}
}
