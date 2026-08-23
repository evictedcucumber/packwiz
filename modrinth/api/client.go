// Package api is a minimal client for the parts of the Modrinth API
// (https://docs.modrinth.com/api) that packwiz needs.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	defaultBaseURL   = "https://api.modrinth.com/v2/"
	defaultUserAgent = "packwiz"
)

// Client manages communication with the Modrinth API.
type Client struct {
	client *http.Client

	// BaseURL for API requests. Must always be set with a trailing slash.
	BaseURL *url.URL

	// UserAgent used when communicating with the Modrinth API.
	UserAgent string

	Projects *ProjectsService
	Versions *VersionsService
}

// NewClient returns a new Modrinth API client. If httpClient is nil,
// http.DefaultClient is used.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURL, _ := url.Parse(defaultBaseURL)

	c := &Client{
		client:    httpClient,
		BaseURL:   baseURL,
		UserAgent: defaultUserAgent,
	}
	c.Projects = &ProjectsService{client: c}
	c.Versions = &VersionsService{client: c}
	return c
}

// newRequest creates an API request. urlStr is resolved relative to the
// client's BaseURL, and should be specified without a preceding slash.
func (c *Client) newRequest(method string, urlStr string, body interface{}) (*http.Request, error) {
	u, err := c.BaseURL.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	var buf io.ReadWriter
	if body != nil {
		buf = &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	return req, nil
}

// do sends an API request and JSON-decodes the response body into v.
func (c *Client) do(req *http.Request, v interface{}) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		errResp := &ErrorResponse{StatusCode: resp.StatusCode}
		data, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			// Best-effort decode; fall back to a bare status-code error below
			// if the body isn't the expected JSON error shape.
			_ = json.Unmarshal(data, errResp)
		}
		return errResp
	}

	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}
	return nil
}

// ErrorResponse is returned when the Modrinth API responds with a non-2xx
// status code.
type ErrorResponse struct {
	StatusCode  int
	ErrorType   string `json:"error"`
	Description string `json:"description"`
}

func (e *ErrorResponse) Error() string {
	if e.ErrorType == "" && e.Description == "" {
		return fmt.Sprintf("modrinth API returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("modrinth API returned status %d: %s - %s", e.StatusCode, e.ErrorType, e.Description)
}
