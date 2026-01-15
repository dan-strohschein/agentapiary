package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// APIClient provides methods to interact with the Apiary API server.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	userID     string
	userName   string
}

// NewAPIClient creates a new API client.
func NewAPIClient() *APIClient {
	server := viper.GetString("server")
	if server == "" {
		server = "http://localhost:8080"
	}

	// Get user credentials from flags or environment
	userID := viper.GetString("user-id")
	if userID == "" {
		userID = os.Getenv("APIARY_USER_ID")
	}

	userName := viper.GetString("user-name")
	if userName == "" {
		userName = os.Getenv("APIARY_USER_NAME")
	}

	return &APIClient{
		baseURL:    strings.TrimSuffix(server, "/"),
		httpClient: &http.Client{},
		userID:     userID,
		userName:   userName,
	}
}

// doRequest performs an HTTP request with authentication headers.
func (c *APIClient) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication headers if provided
	if c.userID != "" {
		req.Header.Set("X-User-ID", c.userID)
	}
	if c.userName != "" {
		req.Header.Set("X-User-Name", c.userName)
	}

	// Set content type for requests with body
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// Get performs a GET request.
func (c *APIClient) Get(path string) (*http.Response, error) {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	// Don't treat 404 as an error here - let the caller decide
	return resp, nil
}

// GetWithError performs a GET request and returns an error for non-2xx status codes.
func (c *APIClient) GetWithError(path string) (*http.Response, error) {
	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// Post performs a POST request with JSON body.
func (c *APIClient) Post(path string, body interface{}) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("failed to encode body: %w", err)
		}
	}
	return c.doRequest(http.MethodPost, path, &buf)
}

// Put performs a PUT request with JSON body.
func (c *APIClient) Put(path string, body interface{}) (*http.Response, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, fmt.Errorf("failed to encode body: %w", err)
		}
	}
	return c.doRequest(http.MethodPut, path, &buf)
}

// Delete performs a DELETE request.
func (c *APIClient) Delete(path string) (*http.Response, error) {
	return c.doRequest(http.MethodDelete, path, nil)
}

// HandleResponse handles an HTTP response, checking for errors and decoding JSON.
func (c *APIClient) HandleResponse(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// IsNotFound checks if an error is a 404 Not Found error.
func (c *APIClient) IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "status 404")
}

// GetNamespace returns the current namespace from viper config.
func GetNamespace() string {
	ns := viper.GetString("namespace")
	if ns == "" {
		return "default"
	}
	return ns
}
