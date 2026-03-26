package workloadexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WorkerCodedError is returned when the worker responds with a non-OK
// status and the body contains a JSON object with a "code" field.
type WorkerCodedError struct {
	Endpoint   string
	StatusCode int
	Code       string
	Details    string
}

func (e *WorkerCodedError) Error() string {
	return fmt.Sprintf("%s returned status %d: code=%s details=%s", e.Endpoint, e.StatusCode, e.Code, e.Details)
}

// parseErrorResponse attempts to parse a JSON error body into a WorkerCodedError.
// If the body isn't valid JSON with a "code" field, it falls back to a plain error.
func parseErrorResponse(endpoint string, statusCode int, body []byte) error {
	var errResp struct {
		Code    string `json:"code"`
		Details string `json:"details"`
	}
	if json.Unmarshal(body, &errResp) == nil && errResp.Code != "" {
		return &WorkerCodedError{
			Endpoint:   endpoint,
			StatusCode: statusCode,
			Code:       errResp.Code,
			Details:    errResp.Details,
		}
	}
	return fmt.Errorf("%s returned status %d: %s", endpoint, statusCode, string(body))
}

// WorkerClient provides REST API calls against a worker (testsvc) endpoint.
type WorkerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewWorkerClient creates a new WorkerClient for the given base URL.
func NewWorkerClient(baseURL string) *WorkerClient {
	return &WorkerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BaseURL returns the base URL of the worker this client connects to.
func (c *WorkerClient) BaseURL() string {
	return c.baseURL
}

// Setup calls POST /_setup on the worker with the given options payload.
func (c *WorkerClient) Setup(opts interface{}) error {
	body, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("failed to marshal setup request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/_setup", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to call /_setup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return parseErrorResponse("/_setup", resp.StatusCode, respBody)
	}

	return nil
}

// Cleanup calls POST /_cleanup on the worker.
func (c *WorkerClient) Cleanup() error {
	resp, err := c.httpClient.Post(c.baseURL+"/_cleanup", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to call /_cleanup: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return parseErrorResponse("/_cleanup", resp.StatusCode, respBody)
	}

	return nil
}

// WorkerMetrics holds the response from GET /_metrics.
type WorkerMetrics struct {
	CPUPercent float64 `json:"cpu_percent"`
	RSSBytes   int64   `json:"rss_bytes"`
}

// Metrics calls GET /_metrics on the worker and returns CPU and memory usage.
func (c *WorkerClient) Metrics() (*WorkerMetrics, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/_metrics")
	if err != nil {
		return nil, fmt.Errorf("failed to call /_metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, parseErrorResponse("/_metrics", resp.StatusCode, respBody)
	}

	var m WorkerMetrics
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("failed to decode /_metrics response: %w", err)
	}

	return &m, nil
}
