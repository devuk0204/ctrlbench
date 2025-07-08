package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/devuk0204/ctrlbench/types"
)

// HTTPClient wraps http.Client with additional functionality
type HTTPClient struct {
	client *http.Client
}

// HTTPResult represents the result of an HTTP request execution
type HTTPResult struct {
	Duration     time.Duration
	StatusCode   int
	ResponseBody string
	Error        error
}

// NewHTTPClient creates a new HTTP client with proper configuration
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// PrepareHTTPRequest prepares HTTP request with body and headers
func (c *HTTPClient) PrepareHTTPRequest(execInfo *types.APIExecutionInfo) (*http.Request, error) {
	// Debug: URL 확인
	fmt.Printf("🔍 DEBUG: Using URL: %s\n", execInfo.FinalURL)

	var requestBody []byte
	var err error

	// Marshal request body if exists
	if execInfo.RequestBody != nil {
		requestBody, err = json.Marshal(execInfo.RequestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Create HTTP request with FinalURL
	req, err := http.NewRequest(execInfo.Method, execInfo.FinalURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for key, value := range execInfo.Headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// MakeHTTPRequest executes HTTP request and returns response
func (c *HTTPClient) MakeHTTPRequest(req *http.Request) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

// HandleHTTPResponse processes HTTP response and extracts body
func (c *HTTPClient) HandleHTTPResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// ExecuteHTTPRequest is a convenience method that combines prepare, execute, and handle
func (c *HTTPClient) ExecuteHTTPRequest(execInfo *types.APIExecutionInfo) (*http.Response, []byte, error) {
	req, err := c.PrepareHTTPRequest(execInfo)
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.MakeHTTPRequest(req)
	if err != nil {
		return nil, nil, err
	}

	respBody, err := c.HandleHTTPResponse(resp)
	if err != nil {
		return resp, nil, err
	}

	return resp, respBody, nil
}

// ExecuteWithResult executes HTTP request and returns detailed result
func (c *HTTPClient) ExecuteWithResult(execInfo *types.APIExecutionInfo, requestNumber int) *HTTPResult {
	req, err := c.PrepareHTTPRequest(execInfo)
	if err != nil {
		return &HTTPResult{Error: err}
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("🔍 DEBUG: HTTP %s %s -> ERROR: %v (%v)\n", execInfo.Method, execInfo.FinalURL, err, duration)
		return &HTTPResult{
			Duration: duration,
			Error:    err,
		}
	}
	defer resp.Body.Close()

	// Always show HTTP response log for each request
	fmt.Printf("🔍 DEBUG: HTTP %s %s -> %d (%v)\n", execInfo.Method, execInfo.FinalURL, resp.StatusCode, duration)

	// Read response body
	bodyBytes, readErr := io.ReadAll(resp.Body)
	responseBody := ""
	if readErr == nil {
		responseBody = string(bodyBytes)
		fmt.Printf("📋 DEBUG: Response Body: %s\n", responseBody)
	}

	return &HTTPResult{
		Duration:     duration,
		StatusCode:   resp.StatusCode,
		ResponseBody: responseBody,
		Error:        readErr,
	}
}
