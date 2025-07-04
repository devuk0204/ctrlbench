package types

// APIList represents the tree structure: NF -> Service -> API
type APIList map[string]map[string]ServiceAPIList

// ServiceAPIList represents service information with APIs
type ServiceAPIList struct {
	Path    string                  `yaml:"path"`
	Version string                  `yaml:"version"`
	APIs    map[string]APIListEntry `yaml:"apis"`
}

// APIListEntry represents an API entry in the tree structure
type APIListEntry struct {
	Path              string      `yaml:"path"`
	Method            string      `yaml:"method"`
	Parameters        []ParamMeta `yaml:"parameters"`
	RequestBody       string      `yaml:"request_body,omitempty"`
	RequestBodySchema BodyMeta    `yaml:"request_body_schema,omitempty"`
}

// ParamMeta represents parameter with required information
type ParamMeta struct {
	Name     string `yaml:"name"`
	Required bool   `yaml:"required"`
	Type     string `yaml:"type,omitempty"`
	In       string `yaml:"in,omitempty"`
}

// BodyMeta represents request body with required fields
type BodyMeta struct {
	SchemaName     string                 `yaml:"schema_name,omitempty"`
	RequiredFields []string               `yaml:"required_fields,omitempty"`
	Schema         map[string]interface{} `yaml:"schema,omitempty"`
}
