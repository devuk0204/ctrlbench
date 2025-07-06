package types

// ServiceMetadata represents metadata for a service
type ServiceMetadata struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	APIs        map[string]APIMetadata `json:"apis"`
	NF          string                 `json:"nf"`
	OpenAPISpec *OpenAPISpec           `json:"-"`
}

// APIMetadata represents metadata for an API with execution details
type APIMetadata struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Methods           []string               `json:"methods"`
	Path              string                 `json:"path"`
	Parameters        []string               `json:"parameters"`
	RequestBody       string                 `json:"request_body"`
	RequestBodySchema map[string]interface{} `json:"request_body_schema,omitempty"`
}
