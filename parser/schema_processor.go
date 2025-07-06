package parser

import (
	"strings"

	"github.com/devuk0204/ctrlbench/types"
)

// SchemaProcessor handles OpenAPI schema processing
type SchemaProcessor struct{}

// NewSchemaProcessor creates a new schema processor
func NewSchemaProcessor() *SchemaProcessor {
	return &SchemaProcessor{}
}

// ExtractSchemas extracts all schemas from an OpenAPI spec
func (sp *SchemaProcessor) ExtractSchemas(spec *types.OpenAPISpec) map[string]interface{} {
	schemas := make(map[string]interface{})

	if spec.Components != nil && spec.Components.Schemas != nil {
		for name, schema := range spec.Components.Schemas {
			schemas[name] = sp.ConvertSchemaToMap(schema)
		}
	}

	return schemas
}

// ConvertSchemaToMap converts a schema definition to a map
func (sp *SchemaProcessor) ConvertSchemaToMap(schema types.SchemaDefinition) map[string]interface{} {
	result := make(map[string]interface{})

	if schema.Type != "" {
		result["type"] = schema.Type
	}

	if schema.Description != "" {
		result["description"] = schema.Description
	}

	if schema.Properties != nil {
		result["properties"] = schema.Properties
	}

	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	if schema.Items != nil {
		result["items"] = sp.ConvertSchemaToMap(*schema.Items)
	}

	if schema.Ref != "" {
		result["$ref"] = schema.Ref
	}

	return result
}

// ExtractRequestBodyInfo extracts request body information from an operation
func (sp *SchemaProcessor) ExtractRequestBodyInfo(operation *types.Operation, schemas map[string]interface{}) (string, map[string]interface{}) {
	if operation.RequestBody == nil || operation.RequestBody.Content == nil {
		return "", nil
	}

	for contentType, mediaType := range operation.RequestBody.Content {
		if contentType == "application/json" {
			return sp.DetermineRequestBodyType(mediaType, operation, schemas)
		}
	}

	return "", nil
}

// DetermineRequestBodyType determines the type and schema of request body
func (sp *SchemaProcessor) DetermineRequestBodyType(mediaType types.MediaType, operation *types.Operation, schemas map[string]interface{}) (string, map[string]interface{}) {
	if mediaType.Schema.Ref != "" {
		// Extract schema name from $ref
		refParts := strings.Split(mediaType.Schema.Ref, "/")
		if len(refParts) > 0 {
			schemaName := refParts[len(refParts)-1]
			if schema, exists := schemas[schemaName]; exists {
				if schemaMap, ok := schema.(map[string]interface{}); ok {
					return schemaName, schemaMap
				}
			}
			return schemaName, nil
		}
	}

	if mediaType.Schema.Type != "" {
		schemaMap := map[string]interface{}{
			"type": mediaType.Schema.Type,
		}

		if mediaType.Schema.Properties != nil {
			schemaMap["properties"] = mediaType.Schema.Properties
		}

		return mediaType.Schema.Type, schemaMap
	}

	return "", nil
}
