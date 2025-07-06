package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/devuk0204/ctrlbench/types"
)

func ExtractServicePath(service types.ServiceMetadata) string {
	if service.OpenAPISpec != nil && len(service.OpenAPISpec.Servers) > 0 {
		serverURL := service.OpenAPISpec.Servers[0].Url

		if strings.Contains(serverURL, "{") {
			if idx := strings.Index(serverURL, "}"); idx != -1 && len(serverURL) > idx+1 {
				path := serverURL[idx+1:]
				if strings.HasPrefix(path, "/") {
					return path
				}
				return "/" + path
			}
		}

		if strings.Contains(serverURL, "://") {
			parts := strings.Split(serverURL, "://")
			if len(parts) > 1 {
				hostAndPath := parts[1]
				if idx := strings.Index(hostAndPath, "/"); idx != -1 {
					return hostAndPath[idx:]
				}
			}
		}
	}

	return ExtractServicePathFromAPI(service)
}

func ExtractServicePathFromAPI(service types.ServiceMetadata) string {
	if len(service.APIs) == 0 {
		return "/"
	}

	var firstPath string
	for _, api := range service.APIs {
		if api.Path != "" {
			firstPath = api.Path
			break
		}
	}

	if firstPath == "" {
		return "/"
	}

	segments := strings.Split(strings.Trim(firstPath, "/"), "/")
	if len(segments) > 0 {
		if len(segments) >= 2 && (segments[1] == "v1" || segments[1] == "v2") {
			return "/" + strings.Join(segments[:2], "/")
		}
		return "/" + segments[0]
	}

	return "/"
}

func ExtractVersionFromPath(servicePath string) string {
	segments := strings.Split(strings.Trim(servicePath, "/"), "/")
	for _, segment := range segments {
		if strings.HasPrefix(segment, "v") && len(segment) <= 3 {
			return segment
		}
	}
	return "v1"
}

// API 이름 관련 유틸리티 함수들
func ExtractMethodFromAPI(apiName string) string {
	methods := []string{"POST", "GET", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		if strings.Contains(apiName, "["+method+"]") {
			return method
		}
	}
	return "GET"
}

func CleanAPIName(apiName string) string {
	methods := []string{"[POST]", "[GET]", "[PUT]", "[DELETE]", "[PATCH]", "[HEAD]", "[OPTIONS]"}
	for _, method := range methods {
		if strings.HasSuffix(apiName, " "+method) {
			return strings.TrimSuffix(apiName, " "+method)
		}
	}
	return apiName
}

func CleanServiceName(serviceName string) string {
	if strings.HasSuffix(serviceName, "Service") {
		return strings.TrimSuffix(serviceName, "Service")
	}
	return serviceName
}

// NF 관련 유틸리티 함수들
func GroupServicesByNF(services map[string]types.ServiceMetadata) map[string][]types.ServiceMetadata {
	nfServices := make(map[string][]types.ServiceMetadata)
	for _, service := range services {
		nf := service.NF
		if nf == "" {
			nf = "UNKNOWN"
		}
		nfServices[nf] = append(nfServices[nf], service)
	}
	return nfServices
}

// IsPathParameter - Check if parameter is a path parameter
func IsPathParameter(paramName, path string) bool {
	return strings.Contains(path, "{"+paramName+"}")
}

// Configuration value extraction functions

// GetConfigString extracts string value from configuration node
// Handles both new format (map with "value" key) and old format (direct string)
func GetConfigString(node interface{}) (string, bool) {
	if node == nil {
		return "", false
	}

	// Handle new configuration format: {value: "..."}
	if nodeMap, ok := node.(map[string]interface{}); ok {
		if val, exists := nodeMap["value"]; exists {
			if str, ok := val.(string); ok {
				return str, true
			}
		}
	}

	// Handle direct string value (old format)
	if str, ok := node.(string); ok {
		return str, true
	}

	return "", false
}

// GetConfigValue extracts any value from configuration node and converts to string
func GetConfigValue(node interface{}) string {
	if node == nil {
		return ""
	}

	// If it's a map with 'value' field (new configuration format)
	if nodeMap, ok := node.(map[string]interface{}); ok {
		if val, exists := nodeMap["value"]; exists {
			return fmt.Sprintf("%v", val)
		}
	}

	// If it's a direct value (old format or simple value)
	return fmt.Sprintf("%v", node)
}

// YAML file loading functions

// LoadYAMLFile loads and parses a YAML file into the provided interface
func LoadYAMLFile(filename string, target interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filename, err)
	}

	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	return nil
}

// LoadConfiguration loads configuration.yaml file
func LoadConfiguration() (map[string]interface{}, error) {
	var config map[string]interface{}
	err := LoadYAMLFile("configuration.yaml", &config)
	return config, err
}

// Map and slice utility functions

// GetSortedKeys returns sorted keys from any map[string]T
func GetSortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String utility functions

// TrimSlashRight removes trailing slash from string
func TrimSlashRight(s string) string {
	return strings.TrimSuffix(s, "/")
}

// TrimSlashLeft removes leading slash from string
func TrimSlashLeft(s string) string {
	return strings.TrimPrefix(s, "/")
}

// NormalizeURL ensures URL has proper format (no trailing slash)
func NormalizeURL(url string) string {
	return TrimSlashRight(url)
}
