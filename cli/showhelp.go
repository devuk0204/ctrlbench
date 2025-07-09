package cli

import (
	"fmt"
	"strings"

	"github.com/devuk0204/ctrlbench/types"
)

// PrintUsage prints usage information
func PrintUsage() {
	fmt.Println("💡 Usage:")
	fmt.Println("    ctrlbench -t NF_NAME -a API_NAME  [OPTIONS]")
	fmt.Println("    ctrlbench -h              # Show usage only")
	fmt.Println("    ctrlbench -h all          # Show all NFs and APIs")
	fmt.Println("    ctrlbench -h NF_NAME      # Show specific NF APIs")
	fmt.Println("    ctrlbench -b              # Build configuration file for all NFs")
	fmt.Println("    ctrlbench -b NF_NAME      # Build configuration file for specific NF")
	fmt.Println("    ctrlbench -b \"NF1 NF2\"    # Build configuration file for multiple NFs")
	fmt.Println()

	fmt.Println("📊 Benchmark Options:")
	fmt.Println("    -i INT        Number of iterations (default: 1)")
	fmt.Println("    -d INT        Duration in seconds (overrides iterations)")
	fmt.Println("    -r INT        Rate limit (requests per second)")
	fmt.Println("    -c INT        Concurrent connections (enables load testing)")
	fmt.Println()

	fmt.Println("📋 Sequential Benchmark Examples:")
	fmt.Println("    ctrlbench -t AUSF -a PostUeAuthentications -i 10")
	fmt.Println("    ctrlbench -t AUSF -a PostUeAuthentications -d 30")
	fmt.Println("    ctrlbench -t AUSF -a PostUeAuthentications -d 30 -r 5")
	fmt.Println("    ctrlbench -t UDM -a GetSubscriptionData -i 5 -r 2")
	fmt.Println()

	fmt.Println("🚀 Concurrent Load Testing Examples:")
	fmt.Println("    ctrlbench -t AUSF -a PostUeAuthentications -c 10 -d 30")
	fmt.Println("    ctrlbench -t AUSF -a PostUeAuthentications -c 5 -d 60 -r 10")
	fmt.Println("    ctrlbench -t UDM -a GetSubscriptionData -c 20 -d 120")
	fmt.Println()

	fmt.Println("🔧 Configuration Build Examples:")
	fmt.Println("    ctrlbench -b              # Build for all NFs")
	fmt.Println("    ctrlbench -b AUSF         # Build for AUSF only")
	fmt.Println("    ctrlbench -b \"AUSF NRF\"   # Build for AUSF and NRF")
	fmt.Println("    ctrlbench -b \"ausf udm amf\"  # Build for multiple NFs (case insensitive)")
	fmt.Println()

	fmt.Println("📝 Notes:")
	fmt.Println("    • Build configuration file first using -b option before executing APIs")
	fmt.Println("    • Configure NRF URL in configuration.yaml")
	fmt.Println("    • Use -c option for concurrent load testing")
	fmt.Println("    • Use -d for time-based execution, -i for iteration-based")
	fmt.Println("    • Rate limiting (-r) works with both sequential and concurrent modes")
	fmt.Println("    • NF names are case insensitive for -b option")
}

// ShowHelp displays help information for services and APIs
func ShowHelp(services map[string]types.ServiceMetadata, nfFilter string) {
	if nfFilter == "" {
		showAllNFs(services)
	} else {
		showSpecificNF(services, nfFilter)
	}
}

func showAllNFs(services map[string]types.ServiceMetadata) {
	fmt.Println("📋 Available Network Functions (NFs)")
	fmt.Println(strings.Repeat("=", 50))

	nfServices := GroupServicesByNF(services)
	nfNames := GetSortedKeys(nfServices)

	for _, nf := range nfNames {
		serviceList := nfServices[nf]
		totalAPIs := 0
		for _, service := range serviceList {
			totalAPIs += len(service.APIs)
		}

		fmt.Printf("📁 %s (%d services, %d APIs)\n", nf, len(serviceList), totalAPIs)
	}

	fmt.Println("\n💡 Use -h NF_NAME to see detailed APIs for a specific NF")
}

func showSpecificNF(services map[string]types.ServiceMetadata, nfFilter string) {
	fmt.Printf("📋 APIs for NF: %s\n", strings.ToUpper(nfFilter))
	fmt.Println(strings.Repeat("=", 50))

	nfServices := GroupServicesByNF(services)
	var matchedServices []types.ServiceMetadata

	for nf, serviceList := range nfServices {
		if strings.EqualFold(nf, nfFilter) {
			matchedServices = serviceList
			break
		}
	}

	if len(matchedServices) == 0 {
		fmt.Printf("❌ No services found for NF: %s\n", nfFilter)
		return
	}

	for _, service := range matchedServices {
		servicePath := ExtractServicePath(service)

		fmt.Printf("📂 %s [%s]\n", CleanServiceName(service.Name), servicePath)

		apiNames := GetSortedKeys(service.APIs)
		for _, apiName := range apiNames {
			api := service.APIs[apiName]
			method := GetMethodFromOpenAPISpec(api, service, apiName)

			fmt.Printf("    📄 %s\n", apiName)
			fmt.Printf("        Method: %s\n", method)
			fmt.Printf("        Path: %s\n", api.Path)

			// Show only required parameters
			requiredParams := getRequiredParameters(api, service)
			if len(requiredParams) > 0 {
				fmt.Printf("        Required Parameters: %v\n", requiredParams)
			}

			// Show only required request body fields
			if api.RequestBody != "" {
				requiredFields := getRequiredRequestBodyFields(api)
				if len(requiredFields) > 0 {
					fmt.Printf("        Request Body: %s (Required: %v)\n", api.RequestBody, requiredFields)
				} else {
					fmt.Printf("        Request Body: %s\n", api.RequestBody)
				}
			}
			fmt.Println()
		}
	}
}

// getRequiredParameters - Extract only required parameters
func getRequiredParameters(api types.APIMetadata, service types.ServiceMetadata) []string {
	var requiredParams []string

	if service.OpenAPISpec != nil {
		// Find required parameters from OpenAPI spec
		for _, pathItem := range service.OpenAPISpec.Paths {
			if pathItem.Get != nil && pathItem.Get.OperationID == strings.TrimSuffix(api.Name, " [GET]") {
				requiredParams = extractRequiredParams(pathItem.Get)
			} else if pathItem.Post != nil && pathItem.Post.OperationID == strings.TrimSuffix(api.Name, " [POST]") {
				requiredParams = extractRequiredParams(pathItem.Post)
			} else if pathItem.Put != nil && pathItem.Put.OperationID == strings.TrimSuffix(api.Name, " [PUT]") {
				requiredParams = extractRequiredParams(pathItem.Put)
			} else if pathItem.Delete != nil && pathItem.Delete.OperationID == strings.TrimSuffix(api.Name, " [DELETE]") {
				requiredParams = extractRequiredParams(pathItem.Delete)
			} else if pathItem.Patch != nil && pathItem.Patch.OperationID == strings.TrimSuffix(api.Name, " [PATCH]") {
				requiredParams = extractRequiredParams(pathItem.Patch)
			}
		}
	}

	// If not found in OpenAPI, consider path parameters as required by default
	if len(requiredParams) == 0 {
		for _, param := range api.Parameters {
			if IsPathParameter(param, api.Path) {
				requiredParams = append(requiredParams, param)
			}
		}
	}

	return requiredParams
}

// extractRequiredParamsFromOperation - Extract required parameters from operation
func extractRequiredParams(operation *types.Operation) []string {
	var requiredParams []string

	for _, param := range operation.Parameters {
		if param.Required {
			requiredParams = append(requiredParams, param.Name)
		}
	}

	return requiredParams
}

// getRequiredRequestBodyFields - Extract only required request body fields
func getRequiredRequestBodyFields(api types.APIMetadata) []string {
	if api.RequestBodySchema == nil {
		return []string{}
	}

	// Extract required fields from RequestBodySchema
	if required, exists := api.RequestBodySchema["required"]; exists {
		if requiredSlice, ok := required.([]string); ok {
			return requiredSlice
		}
		// Convert if it's interface{} slice
		if requiredInterface, ok := required.([]interface{}); ok {
			var requiredFields []string
			for _, field := range requiredInterface {
				if fieldStr, ok := field.(string); ok {
					requiredFields = append(requiredFields, fieldStr)
				}
			}
			return requiredFields
		}
	}

	return []string{}
}
