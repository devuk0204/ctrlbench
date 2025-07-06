package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/devuk0204/ctrlbench/types"
)

// HTTPClient wraps http.Client with additional functionality
type HTTPClient struct {
	client *http.Client
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

// PrepareHTTPRequest creates an HTTP request with proper headers and body
func (c *HTTPClient) PrepareHTTPRequest(method, url string, body []byte, headers map[string]string) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set custom headers
	for key, value := range headers {
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
func (c *HTTPClient) ExecuteHTTPRequest(method, url string, body []byte, headers map[string]string) (*http.Response, []byte, error) {
	req, err := c.PrepareHTTPRequest(method, url, body, headers)
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

// ExecuteWithResult executes an HTTP request and returns detailed result
func (c *HTTPClient) ExecuteWithResult(execInfo *types.APIExecutionInfo, workerID int) *RequestResult {
	start := time.Now()
	result := &RequestResult{
		Timestamp: start,
		WorkerID:  workerID,
	}

	// Build final URL
	finalURL := buildFinalURL(execInfo)

	// Prepare request body
	var bodyBytes []byte
	if execInfo.RequestBody != nil {
		var err error
		bodyBytes, err = json.Marshal(execInfo.RequestBody)
		if err != nil {
			result.Error = fmt.Errorf("failed to marshal request body: %v", err)
			result.Duration = time.Since(start)
			return result
		}
	}

	// Execute HTTP request
	resp, _, err := c.ExecuteHTTPRequest(execInfo.Method, finalURL, bodyBytes, execInfo.Headers)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err
		return result
	}

	result.Status = resp.StatusCode

	return result
}

// buildFinalURL constructs the final URL with parameters
func buildFinalURL(execInfo *types.APIExecutionInfo) string {
	finalURL := execInfo.DiscoveredURL + execInfo.Path

	// Replace path parameters
	for key, value := range execInfo.Parameters {
		placeholder := "{" + key + "}"
		if strings.Contains(finalURL, placeholder) {
			finalURL = strings.ReplaceAll(finalURL, placeholder, value)
		}
	}

	// Add query parameters
	if len(execInfo.Parameters) > 0 {
		queryParams := make([]string, 0)
		for key, value := range execInfo.Parameters {
			placeholder := "{" + key + "}"
			if !strings.Contains(execInfo.Path, placeholder) {
				queryParams = append(queryParams, fmt.Sprintf("%s=%s", key, value))
			}
		}

		if len(queryParams) > 0 {
			finalURL += "?" + strings.Join(queryParams, "&")
		}
	}

	return finalURL
}
