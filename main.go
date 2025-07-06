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
	buildConfigFlag = flag.Bool("b", false, "Build configuration file")
	apiFlag         = flag.String("a", "", "API method name")
	targetNFFlag    = flag.String("t", "", "Target NF name")
	iterationsFlag  = flag.Int("i", 1, "Number of iterations")
	concurrency     = flag.Int("c", 1, "Number of concurrent connections")
	duration        = flag.Int("d", 0, "Duration in seconds (0 = use iterations)")
	rateLimit       = flag.Int("r", 0, "Rate limit (requests per second, 0 = unlimited)")
)

func main() {
	flag.Parse()

	// Load OpenAPI services
	services := loadOpenAPIServices()

	// Handle different command modes
	switch {
	case *helpFlag:
		handleHelpCommand(services)
	case *buildConfigFlag:
		handleBuildCommand(services)
	case *targetNFFlag != "" && *apiFlag != "":
		handleAPIExecution()
	default:
		cli.PrintUsage()
	}
}

// loadOpenAPIServices loads and parses OpenAPI directory
func loadOpenAPIServices() map[string]types.ServiceMetadata {
	openapiDir := "./openapi"
	if _, err := os.Stat(openapiDir); err != nil {
		log.Printf("   OpenAPI dir '%s' not found, please create it and add your OpenAPI YAML files", openapiDir)
		return nil
	}

	services, err := parser.ParseOpenAPIDir(openapiDir)
	if err != nil {
		log.Printf("   Failed to parse OpenAPI dir: %v", err)
		os.Exit(1)
	}
	return services
}

// handleHelpCommand processes help flag
func handleHelpCommand(services map[string]types.ServiceMetadata) {
	if flag.NArg() == 0 {
		cli.PrintUsage()
	} else if strings.EqualFold(flag.Arg(0), "all") {
		cli.ShowHelp(services, "")
	} else {
		cli.ShowHelp(services, flag.Arg(0))
	}
}

// handleBuildCommand processes build configuration flag
func handleBuildCommand(services map[string]types.ServiceMetadata) {
	nfFilter := ""
	if flag.NArg() > 0 {
		nfFilter = flag.Arg(0)
	}

	err := cli.BuildConfiguration(services, nfFilter)
	if err != nil {
		log.Printf("  Failed to build configuration: %v", err)
		os.Exit(1)
	}
}

// handleAPIExecution processes API execution
func handleAPIExecution() {
	targetNF := strings.ToUpper(*targetNFFlag)
	executor := cli.NewAPIExecutor(30 * time.Second)

	// Prepare execution info
	execInfo, err := executor.ExecuteAPI(targetNF, *apiFlag)
	if err != nil {
		log.Fatalf("Failed to prepare API execution: %v", err)
	}

	// Print execution details
	printExecutionDetails(execInfo)

	// Execute benchmark based on flags
	if *concurrency > 1 || *duration > 0 {
		runConcurrentBenchmark(executor, execInfo)
	} else {
		runSequentialBenchmark(executor, execInfo)
	}
}

// printExecutionDetails prints API execution information
func printExecutionDetails(execInfo *types.APIExecutionInfo) {
	fmt.Printf("  Execution Details:\n")
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
}

// runConcurrentBenchmark executes concurrent benchmark
func runConcurrentBenchmark(executor *cli.APIExecutor, execInfo *types.APIExecutionInfo) {
	result, err := executor.RunConcurrentBenchmark(
		execInfo,
		*concurrency,
		time.Duration(*duration)*time.Second,
		*rateLimit,
	)
	if err != nil {
		log.Fatalf("Concurrent benchmark failed: %v", err)
	}
	printConcurrentResults(result)
}

// runSequentialBenchmark executes sequential benchmark
func runSequentialBenchmark(executor *cli.APIExecutor, execInfo *types.APIExecutionInfo) {
	result, err := executor.RunBenchmark(execInfo, *iterationsFlag)
	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}
	printResults(result)
}

// printResults prints sequential benchmark results
func printResults(result *types.BenchmarkResult) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	successRate := float64(result.SuccessCount) / float64(result.TotalRequests) * 100

	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d\n", result.SuccessCount)
	fmt.Printf("Failed: %d\n", result.FailureCount)
	fmt.Printf("Success Rate: %.2f%%\n", successRate)
	fmt.Printf("Throughput: %.2f RPS\n", result.RequestsPerSecond)
	fmt.Println()
	fmt.Printf("Response Times:\n")
	fmt.Printf("Average: %v\n", result.AvgTime)
	fmt.Printf("Minimum: %v\n", result.MinTime)
	fmt.Printf("Maximum: %v\n", result.MaxTime)
	fmt.Printf("Total Duration: %v\n", result.TotalTime)
}

// printConcurrentResults prints concurrent benchmark results
func printConcurrentResults(result *types.BenchmarkResult) {
	fmt.Printf("\n============================================================\n")
	fmt.Printf("  CONCURRENT BENCHMARK RESULTS\n")
	fmt.Printf("============================================================\n")
	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d\n", result.SuccessCount)
	fmt.Printf("Failed: %d\n", result.FailureCount)
	fmt.Printf("Success Rate: %.2f%%\n", float64(result.SuccessCount)/float64(result.TotalRequests)*100)
	fmt.Printf("Throughput: %.2f RPS\n", result.RequestsPerSecond)
	fmt.Printf("Test Duration: %v\n", result.TotalTime)

	fmt.Printf("\nResponse Times:\n")
	fmt.Printf("Average: %v\n", result.AvgTime)
	fmt.Printf("Minimum: %v\n", result.MinTime)
	fmt.Printf("Maximum: %v\n", result.MaxTime)

	if result.Percentiles != nil {
		fmt.Printf("\nPercentiles:\n")
		fmt.Printf("50th: %v\n", result.Percentiles["50th"])
		fmt.Printf("75th: %v\n", result.Percentiles["75th"])
		fmt.Printf("90th: %v\n", result.Percentiles["90th"])
		fmt.Printf("95th: %v\n", result.Percentiles["95th"])
		fmt.Printf("99th: %v\n", result.Percentiles["99th"])
	}

	if len(result.ErrorDistribution) > 0 {
		fmt.Printf("\nError Distribution:\n")
		for errorType, count := range result.ErrorDistribution {
			fmt.Printf("%s: %d\n", errorType, count)
		}
	}
}
