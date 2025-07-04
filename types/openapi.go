package types

// OpenAPI 3.0 spec structures
type OpenAPISpec struct {
	OpenAPI    string                `yaml:"openapi"`
	Info       Info                  `yaml:"info"`
	Servers    []Server              `yaml:"servers"`
	Security   []map[string][]string `yaml:"security,omitempty"`
	Paths      map[string]PathItem   `yaml:"paths"`
	Tags       []Tag                 `yaml:"tags,omitempty"`
	Components *Components           `yaml:"components,omitempty"`
}

type Info struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Version     string `yaml:"version"`
}

type Server struct {
	Url         string                 `yaml:"url"`
	Description string                 `yaml:"description,omitempty"`
	Variables   map[string]interface{} `yaml:"variables,omitempty"`
}

type Tag struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type Components struct {
	Schemas map[string]SchemaDefinition `yaml:"schemas,omitempty"`
}

type SchemaDefinition struct {
	Description string                 `yaml:"description,omitempty"`
	Type        string                 `yaml:"type,omitempty"`
	Properties  map[string]interface{} `yaml:"properties,omitempty"`
	Required    []string               `yaml:"required,omitempty"`
	Items       *SchemaDefinition      `yaml:"items,omitempty"`
	Ref         string                 `yaml:"$ref,omitempty"`
}

type PathItem struct {
	Get     *Operation `yaml:"get,omitempty"`
	Post    *Operation `yaml:"post,omitempty"`
	Put     *Operation `yaml:"put,omitempty"`
	Delete  *Operation `yaml:"delete,omitempty"`
	Patch   *Operation `yaml:"patch,omitempty"`
	Head    *Operation `yaml:"head,omitempty"`
	Options *Operation `yaml:"options,omitempty"`
}

type Operation struct {
	OperationID string                 `yaml:"operationId,omitempty"`
	Summary     string                 `yaml:"summary,omitempty"`
	Description string                 `yaml:"description,omitempty"`
	Tags        []string               `yaml:"tags,omitempty"`
	Parameters  []Parameter            `yaml:"parameters,omitempty"`
	RequestBody *RequestBody           `yaml:"requestBody,omitempty"`
	Responses   map[string]interface{} `yaml:"responses,omitempty"`
	Security    []map[string][]string  `yaml:"security,omitempty"`
}

type Parameter struct {
	Name        string `yaml:"name"`
	In          string `yaml:"in"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description,omitempty"`
	Schema      Schema `yaml:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `yaml:"description,omitempty"`
	Required    bool                 `yaml:"required,omitempty"`
	Content     map[string]MediaType `yaml:"content,omitempty"`
}

type MediaType struct {
	Schema Schema `yaml:"schema,omitempty"`
}

type Schema struct {
	Type       string                 `yaml:"type,omitempty"`
	Format     string                 `yaml:"format,omitempty"`
	Ref        string                 `yaml:"$ref,omitempty"`
	Properties map[string]interface{} `yaml:"properties,omitempty"`
	Items      *Schema                `yaml:"items,omitempty"`
}
