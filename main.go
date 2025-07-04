package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/cli"
	"github.com/devuk0204/ctrlbench/parser"
	"github.com/devuk0204/ctrlbench/types"
)

var (
	helpFlag        = flag.Bool("h", false, "Show help information")
	apiFlag         = flag.String("a", "", "API method name")
	targetNFFlag    = flag.String("t", "", "Target NF name")
	iterationsFlag  = flag.Int("i", 1, "Number of iterations")
	buildConfigFlag = flag.Bool("b", false, "Build configuration file")
)

// runAPIExecution executes API calls using api_list.yaml and configuration.yaml
func runAPIExecution(targetNF, apiName string, iterations int) {
	fmt.Printf("🚀 Starting API execution for %s.%s\n", targetNF, apiName)
	fmt.Printf("📊 Iterations: %d\n\n", iterations)

	// Create executor
	executor := cli.NewAPIExecutor(30 * time.Second)

	// Prepare execution info using api_list.yaml
	execInfo, err := executor.ExecuteAPI(targetNF, apiName)
	if err != nil {
		log.Printf("❌ Failed to prepare API execution: %v", err)
		os.Exit(1)
	}

	fmt.Printf("📋 Execution Details:\n")
	fmt.Printf("   NF: %s\n", execInfo.NF)
	fmt.Printf("   API: %s\n", execInfo.APIName)
	fmt.Printf("   Method: %s\n", execInfo.Method)
	fmt.Printf("   Path: %s\n", execInfo.Path)
	fmt.Printf("   Discovered URL: %s\n", execInfo.DiscoveredURL)
	fmt.Printf("   Parameters: %v\n", execInfo.Parameters)
	if execInfo.RequestBody != nil {
		bodyBytes, _ := json.Marshal(execInfo.RequestBody)
		fmt.Printf("   Request Body: %s\n", string(bodyBytes))
	}
	fmt.Println()

	var result types.BenchmarkResult
	result.TotalRequests = iterations
	result.MinTime = time.Hour

	startTime := time.Now()

	for i := 0; i < iterations; i++ {
		duration, err := executor.ExecuteHTTPCall(execInfo)

		if err != nil {
			result.FailureCount++
			fmt.Printf("❌ Request %d failed: %v\n", i+1, err)
		} else {
			result.SuccessCount++
			if i%10 == 0 {
				fmt.Printf("✅ Request %d completed in %v\n", i+1, duration)
			}
		}

		if duration < result.MinTime {
			result.MinTime = duration
		}
		if duration > result.MaxTime {
			result.MaxTime = duration
		}
		result.TotalTime += duration
	}

	totalElapsed := time.Since(startTime)
	result.AvgTime = result.TotalTime / time.Duration(result.TotalRequests)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📈 BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	successRate := float64(result.SuccessCount) / float64(result.TotalRequests) * 100
	rps := float64(result.TotalRequests) / totalElapsed.Seconds()

	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d\n", result.SuccessCount)
	fmt.Printf("Failed: %d\n", result.FailureCount)
	fmt.Printf("Success Rate: %.2f%%\n", successRate)
	fmt.Printf("Throughput: %.2f RPS\n", rps)
	fmt.Println()
	fmt.Printf("Response Times:\n")
	fmt.Printf("Average: %v\n", result.AvgTime)
	fmt.Printf("Minimum: %v\n", result.MinTime)
	fmt.Printf("Maximum: %v\n", result.MaxTime)
	fmt.Printf("Total Duration: %v\n", totalElapsed)
}

func main() {
	flag.Parse()

	var services map[string]types.ServiceMetadata

	openapiDir := "./openapi"
	if _, err := os.Stat(openapiDir); err == nil {
		services, err = parser.ParseOpenAPIDir(openapiDir)
		if err != nil {
			log.Printf("⚠️  Failed to parse OpenAPI dir: %v", err)
			os.Exit(1)
		}
	} else {
		log.Printf("⚠️  OpenAPI dir '%s' not found, please create it and add your OpenAPI YAML files", openapiDir)
	}

	if *helpFlag {
		if flag.NArg() == 0 {
			cli.PrintUsage()
		} else if strings.EqualFold(flag.Arg(0), "all") {
			cli.ShowHelp(services, "")
		} else {
			cli.ShowHelp(services, flag.Arg(0))
		}
		return
	}

	if *buildConfigFlag {
		nfFilter := ""
		if flag.NArg() > 0 {
			nfFilter = flag.Arg(0)
		}
		err := cli.BuildConfiguration(services, nfFilter)
		if err != nil {
			log.Printf("❌ Failed to build configuration: %v", err)
			os.Exit(1)
		}
		return
	}

	// Handle API execution with api_list.yaml
	if *targetNFFlag != "" && *apiFlag != "" {
		var targetNF = strings.ToUpper(*targetNFFlag)
		runAPIExecution(targetNF, *apiFlag, *iterationsFlag)
		return
	}

	fmt.Println("❌ Target NF (-nf) and API name (-a) are required")
	fmt.Println("💡 Use -h to see available NFs and APIs")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("    github.com/devuk0204/ctrlbench -nf AUSF -a \"CreateUe-authentications [POST]\" -i 10")
	os.Exit(1)
}
