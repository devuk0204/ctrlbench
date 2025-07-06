package types

// API List output types
type APIList map[string]map[string]ServiceAPIList

type ServiceAPIList struct {
	Path    string                  `yaml:"path"`
	Version string                  `yaml:"version"`
	APIs    map[string]APIListEntry `yaml:"apis"`
}

type APIListEntry struct {
	Path              string      `yaml:"path"`
	Method            string      `yaml:"method"`
	Parameters        []ParamMeta `yaml:"parameters"`
	RequestBody       string      `yaml:"request_body,omitempty"`
	RequestBodySchema BodyMeta    `yaml:"request_body_schema,omitempty"`
}

type ParamMeta struct {
	Name     string `yaml:"name"`
	Required bool   `yaml:"required"`
	Type     string `yaml:"type,omitempty"`
	In       string `yaml:"in,omitempty"`
}

type BodyMeta struct {
	SchemaName     string                 `yaml:"schema_name,omitempty"`
	RequiredFields []string               `yaml:"required_fields,omitempty"`
	Schema         map[string]interface{} `yaml:"schema,omitempty"`
}
