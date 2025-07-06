package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/devuk0204/ctrlbench/types"
)

// OpenAPIParser main parser with components
type OpenAPIParser struct {
	schemaProcessor *SchemaProcessor
	apiExtractor    *APIExtractor
}

// NewOpenAPIParser creates a new parser instance
func NewOpenAPIParser() *OpenAPIParser {
	return &OpenAPIParser{
		schemaProcessor: NewSchemaProcessor(),
		apiExtractor:    NewAPIExtractor(),
	}
}

// ParseOpenAPIDir parses OpenAPI YAML files and returns service metadata
func ParseOpenAPIDir(dirPath string) (map[string]types.ServiceMetadata, error) {
	parser := NewOpenAPIParser()
	return parser.ParseDirectory(dirPath)
}

// ParseDirectory parses all YAML files in the given directory
func (p *OpenAPIParser) ParseDirectory(dirPath string) (map[string]types.ServiceMetadata, error) {
	services := make(map[string]types.ServiceMetadata)

	files, err := os.ReadDir(dirPath)
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

		p.processOpenAPISpec(spec, services)
	}

	return services, nil
}

// processOpenAPISpec processes a single OpenAPI spec
func (p *OpenAPIParser) processOpenAPISpec(spec *types.OpenAPISpec, services map[string]types.ServiceMetadata) {
	nfName := extractNFName(spec)
	serviceName := strings.TrimSuffix(spec.Info.Title, " API")

	// Get or create service
	var service types.ServiceMetadata
	if existing, exists := services[serviceName]; exists {
		service = existing
	} else {
		service = types.ServiceMetadata{
			Name:        serviceName,
			Description: spec.Info.Description,
			APIs:        make(map[string]types.APIMetadata),
			NF:          nfName,
			OpenAPISpec: spec,
		}
	}

	schemas := p.schemaProcessor.ExtractSchemas(spec)

	// Process all paths using the APIExtractor
	for path, pathItem := range spec.Paths {
		p.apiExtractor.ProcessPathItem(path, pathItem, &service, schemas)
	}

	// Save back to services map
	services[serviceName] = service
}

// isYAMLFile checks if file has YAML extension
func isYAMLFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".yaml" || ext == ".yml"
}

// loadOpenAPISpec loads and parses OpenAPI spec from file
func loadOpenAPISpec(filePath string) (*types.OpenAPISpec, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var spec types.OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// extractNFName extracts NF name from OpenAPI spec
func extractNFName(spec *types.OpenAPISpec) string {
	// Try to extract from title
	if nf := extractNFFromTitle(spec.Info.Title); nf != "" {
		return nf
	}

	// Try to extract from description
	if nf := extractNFFromDescription(spec.Info.Description); nf != "" {
		return nf
	}

	// Try to extract from servers
	if nf := extractNFFromServers(spec.Servers); nf != "" {
		return nf
	}

	return "UNKNOWN"
}

// extractNFFromTitle extracts NF name from title
func extractNFFromTitle(title string) string {
	// Pattern: "TS29503_Nudm_SDM" -> "UDM"
	re := regexp.MustCompile(`N([a-z]+)_`)
	matches := re.FindStringSubmatch(title)
	if len(matches) > 1 {
		return strings.ToUpper(matches[1])
	}

	// Pattern: "UDM Service" -> "UDM"
	parts := strings.Fields(title)
	for _, part := range parts {
		if isNFName(part) {
			return strings.ToUpper(part)
		}
	}

	return ""
}

// extractNFFromDescription extracts NF name from description
func extractNFFromDescription(desc string) string {
	// Look for known NF patterns
	nfPatterns := []string{"AMF", "SMF", "UDM", "UDR", "AUSF", "NRF", "PCF", "UPF", "NSSF", "NEF"}

	for _, nf := range nfPatterns {
		if strings.Contains(strings.ToUpper(desc), nf) {
			return nf
		}
	}

	return ""
}

// extractNFFromServers extracts NF name from server URLs
func extractNFFromServers(servers []types.Server) string {
	for _, server := range servers {
		// Pattern: "https://example.com/nudm-sdm/v1" -> "UDM"
		re := regexp.MustCompile(`/n([a-z]+)-`)
		matches := re.FindStringSubmatch(server.Url)
		if len(matches) > 1 {
			return strings.ToUpper(matches[1])
		}
	}

	return ""
}

// isNFName checks if a string looks like an NF name
func isNFName(s string) bool {
	nfNames := []string{"AMF", "SMF", "UDM", "UDR", "AUSF", "NRF", "PCF", "UPF", "NSSF", "NEF"}
	upper := strings.ToUpper(s)

	for _, nf := range nfNames {
		if upper == nf {
			return true
		}
	}

	return false
}
