package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/parser"
	"github.com/devuk0204/ctrlbench/types"
)

// CLI flags
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

// Run starts the CLI workflow
func Run() {
	flag.Parse()
	services := loadOpenAPIServices()

	switch {
	case *helpFlag:
		handleHelpCommand(services)
	case *buildConfigFlag:
		handleBuildCommand(services)
	case *targetNFFlag != "" && *apiFlag != "":
		handleAPIExecution()
	default:
		PrintUsage()
	}
}

func loadOpenAPIServices() map[string]types.ServiceMetadata {
	openapiDir := "./openapi"
	if _, err := os.Stat(openapiDir); err != nil {
		log.Printf("OpenAPI dir '%s' not found, please create it and add your OpenAPI YAML files", openapiDir)
		return nil
	}

	services, err := parser.ParseOpenAPIDir(openapiDir)
	if err != nil {
		log.Fatalf("Failed to parse OpenAPI dir: %v", err)
	}
	return services
}

func handleHelpCommand(services map[string]types.ServiceMetadata) {
	if flag.NArg() == 0 {
		PrintUsage()
	} else if strings.EqualFold(flag.Arg(0), "all") {
		ShowHelp(services, "")
	} else {
		ShowHelp(services, flag.Arg(0))
	}
}

func handleBuildCommand(services map[string]types.ServiceMetadata) {
	var nfFilters []string
	if flag.NArg() > 0 {
		nfFilters = strings.Fields(strings.ToUpper(flag.Arg(0)))
	}
	if err := BuildConfiguration(services, nfFilters); err != nil {
		log.Fatalf("Failed to build configuration: %v", err)
	}
}

func handleAPIExecution() {
	targetNF := strings.ToUpper(*targetNFFlag)
	executor := NewAPIExecutor(30 * time.Second)

	execInfo, err := executor.ExecuteAPI(targetNF, *apiFlag)
	if err != nil {
		log.Fatalf("Failed to prepare API execution: %v", err)
	}

	if *concurrency > 1 {
		runConcurrentBenchmark(executor, execInfo)
	} else {
		runSequentialBenchmark(executor, execInfo)
	}
}

func runConcurrentBenchmark(executor *APIExecutor, execInfo *types.APIExecutionInfo) {
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

func runSequentialBenchmark(executor *APIExecutor, execInfo *types.APIExecutionInfo) {
	result, err := executor.RunBenchmark(
		execInfo,
		*iterationsFlag,
		time.Duration(*duration)*time.Second,
		*rateLimit,
	)
	if err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}
	printResults(result)
}

func printResults(result *types.BenchmarkResult) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  SEQUENTIAL BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	successRate := float64(result.SuccessCount) / float64(result.TotalRequests) * 100

	fmt.Printf("Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("Successful: %d\n", result.SuccessCount)
	fmt.Printf("Failed: %d\n", result.FailureCount)
	fmt.Printf("Success Rate: %.2f%%\n", successRate)
	fmt.Printf("Total Duration: %v\n", result.TotalTime)
	fmt.Println()

	fmt.Printf("Response Times:\n")
	fmt.Printf("Average: %v\n", result.AvgTime)
	fmt.Printf("Minimum: %v\n", result.MinTime)
	fmt.Printf("Maximum: %v\n", result.MaxTime)
	if result.TrimmedMeans != nil {
		fmt.Printf("90%% Trimmed Average: %v\n", result.TrimmedMeans["90%"])
		fmt.Printf("95%% Trimmed Average: %v\n", result.TrimmedMeans["95%"])
		fmt.Printf("99%% Trimmed Average: %v\n", result.TrimmedMeans["99%"])
	}

	if len(result.ErrorDistribution) > 0 {
		fmt.Printf("\nError Distribution:\n")
		for errType, count := range result.ErrorDistribution {
			fmt.Printf("%s: %d\n", errType, count)
		}
	}
}

func printConcurrentResults(result *types.BenchmarkResult) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  CONCURRENT BENCHMARK RESULTS")
	fmt.Println(strings.Repeat("=", 60))

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
	if result.TrimmedMeans != nil {
		fmt.Printf("90%% Trimmed Average: %v\n", result.TrimmedMeans["90%"])
		fmt.Printf("95%% Trimmed Average: %v\n", result.TrimmedMeans["95%"])
		fmt.Printf("99%% Trimmed Average: %v\n", result.TrimmedMeans["99%"])
	}

	if result.Percentiles != nil {
		fmt.Printf("\nPercentiles:\n")
		fmt.Printf("50%%: %v\n", result.Percentiles["50th"])
		fmt.Printf("75%%: %v\n", result.Percentiles["75th"])
		fmt.Printf("90%%: %v\n", result.Percentiles["90th"])
		fmt.Printf("95%%: %v\n", result.Percentiles["95th"])
		fmt.Printf("99%%: %v\n", result.Percentiles["99th"])
	}

	if len(result.ErrorDistribution) > 0 {
		fmt.Printf("\nError Distribution:\n")
		for errType, count := range result.ErrorDistribution {
			fmt.Printf("%s: %d\n", errType, count)
		}
	}
}
