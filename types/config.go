package types

// Configuration structure with user input sections
type ConfigurationFile struct {
	UserInputs UserInputSection `yaml:"user_inputs"`
}

type UserInputSection struct {
	GlobalSettings           map[string]interface{} `yaml:"global_settings"`
	NFSettings              map[string]map[string]interface{} `yaml:"nf_settings"`
	CommonParameters        map[string]interface{} `yaml:"common_parameters"`
	CommonRequestBodies     map[string]interface{} `yaml:"common_request_bodies"`
	APISpecificParameters   map[string]interface{} `yaml:"api_specific_parameters"`
	APISpecificRequestBodies map[string]interface{} `yaml:"api_specific_request_bodies"`
}

type CommonConfig struct {
	Parameters    map[string]Parameter              `yaml:"parameters"`
	RequestBodies map[string]map[string]interface{} `yaml:"request_bodies"`
	Headers       map[string]string                 `yaml:"headers"`
}

type NFConfiguration struct {
	Name        string                        `yaml:"name"`
	Description string                        `yaml:"description,omitempty"`
	Services    map[string]ServiceConfig      `yaml:"services"`
}

type ServiceConfig struct {
	Name        string                        `yaml:"name"`
	Description string                        `yaml:"description,omitempty"`
	APIs        map[string]APIConfig          `yaml:"apis"`
}

type APIConfig struct {
	Method         string                    `yaml:"method"`
	Path           string                    `yaml:"path"`
	Description    string                    `yaml:"description"`
	UseParameters  []string                  `yaml:"use_parameters,omitempty"`
	UseRequestBody string                    `yaml:"use_request_body,omitempty"`
	CustomParams   map[string]Parameter      `yaml:"custom_parameters,omitempty"`
}
