package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestClient returns a Client pointed at a test server running handler,
// closing the server on test cleanup.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c := NewClient(nil)
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}
	c.BaseURL = baseURL
	return c
}

func TestNewClientDefaults(t *testing.T) {
	c := NewClient(nil)

	if c.client != http.DefaultClient {
		t.Errorf("client = %v, want http.DefaultClient", c.client)
	}
	if c.BaseURL == nil || c.BaseURL.String() != defaultBaseURL {
		t.Errorf("BaseURL = %v, want %v", c.BaseURL, defaultBaseURL)
	}
	if c.UserAgent != defaultUserAgent {
		t.Errorf("UserAgent = %q, want %q", c.UserAgent, defaultUserAgent)
	}
	if c.Projects == nil {
		t.Error("Projects service is nil")
	}
	if c.Versions == nil {
		t.Error("Versions service is nil")
	}
	if c.Projects.client != c {
		t.Error("Projects service does not reference the owning client")
	}
	if c.Versions.client != c {
		t.Error("Versions service does not reference the owning client")
	}
}

func TestNewClientCustomHTTPClient(t *testing.T) {
	custom := &http.Client{}
	c := NewClient(custom)
	if c.client != custom {
		t.Error("NewClient did not use the provided http.Client")
	}
}

func TestNewRequestGetNoBody(t *testing.T) {
	c := NewClient(nil)
	c.UserAgent = "packwiz-test"

	req, err := c.newRequest(http.MethodGet, "project/foo", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}

	if req.Method != http.MethodGet {
		t.Errorf("Method = %q, want %q", req.Method, http.MethodGet)
	}
	wantURL := defaultBaseURL + "project/foo"
	if req.URL.String() != wantURL {
		t.Errorf("URL = %q, want %q", req.URL.String(), wantURL)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept header = %q, want application/json", got)
	}
	if got := req.Header.Get("User-Agent"); got != "packwiz-test" {
		t.Errorf("User-Agent header = %q, want packwiz-test", got)
	}
	if got := req.Header.Get("Content-Type"); got != "" {
		t.Errorf("Content-Type header = %q, want empty for a body-less request", got)
	}
}

func TestNewRequestEmptyUserAgent(t *testing.T) {
	c := NewClient(nil)
	c.UserAgent = ""

	req, err := c.newRequest(http.MethodGet, "project/foo", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}
	if _, ok := req.Header["User-Agent"]; ok {
		t.Errorf("User-Agent header should not be set when UserAgent is empty, got %q", req.Header.Get("User-Agent"))
	}
}

func TestNewRequestWithBody(t *testing.T) {
	c := NewClient(nil)

	type payload struct {
		Note string `json:"note"`
	}
	req, err := c.newRequest(http.MethodPost, "thing", payload{Note: "a & b < c"})
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}

	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type header = %q, want application/json", got)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}
	// SetEscapeHTML(false) must be honoured, so '&' and '<' stay literal
	// rather than becoming & / <.
	got := strings.TrimSpace(string(body))
	want := `{"note":"a & b < c"}`
	if got != want {
		t.Errorf("request body = %q, want %q", got, want)
	}
}

func TestNewRequestInvalidRelativeURL(t *testing.T) {
	c := NewClient(nil)
	_, err := c.newRequest(http.MethodGet, "%zz", nil)
	if err == nil {
		t.Error("expected error for an unparseable relative URL, got nil")
	}
}

func TestNewRequestUnencodableBody(t *testing.T) {
	c := NewClient(nil)
	// A channel cannot be marshalled to JSON.
	_, err := c.newRequest(http.MethodPost, "thing", make(chan int))
	if err == nil {
		t.Error("expected error for a body that cannot be JSON-encoded, got nil")
	}
}

func TestDoDecodesSuccessResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":"hello"}`))
	})

	req, err := c.newRequest(http.MethodGet, "thing", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}

	var out struct {
		Value string `json:"value"`
	}
	if err := c.do(req, &out); err != nil {
		t.Fatalf("do() returned error: %v", err)
	}
	if out.Value != "hello" {
		t.Errorf("Value = %q, want %q", out.Value, "hello")
	}
}

func TestDoWithNilOutputIgnoresBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req, err := c.newRequest(http.MethodGet, "thing", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}
	if err := c.do(req, nil); err != nil {
		t.Fatalf("do() returned error: %v", err)
	}
}

func TestDoNon2xxWithJSONErrorBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","description":"no such project"}`))
	})

	req, err := c.newRequest(http.MethodGet, "project/missing", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}

	err = c.do(req, nil)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("error type = %T, want *ErrorResponse", err)
	}
	if errResp.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", errResp.StatusCode, http.StatusNotFound)
	}
	if errResp.ErrorType != "not_found" {
		t.Errorf("ErrorType = %q, want %q", errResp.ErrorType, "not_found")
	}
	if errResp.Description != "no such project" {
		t.Errorf("Description = %q, want %q", errResp.Description, "no such project")
	}
}

func TestDoNon2xxWithNonJSONBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<html>oops</html>"))
	})

	req, err := c.newRequest(http.MethodGet, "thing", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}

	err = c.do(req, nil)
	errResp, ok := err.(*ErrorResponse)
	if !ok {
		t.Fatalf("error type = %T, want *ErrorResponse", err)
	}
	if errResp.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", errResp.StatusCode, http.StatusInternalServerError)
	}
	if errResp.ErrorType != "" || errResp.Description != "" {
		t.Errorf("expected empty ErrorType/Description for a non-JSON body, got %q / %q", errResp.ErrorType, errResp.Description)
	}
}

func TestDoNetworkError(t *testing.T) {
	c := NewClient(nil)
	baseURL, _ := url.Parse("http://127.0.0.1:0/")
	c.BaseURL = baseURL

	req, err := c.newRequest(http.MethodGet, "thing", nil)
	if err != nil {
		t.Fatalf("newRequest() returned error: %v", err)
	}
	if err := c.do(req, nil); err == nil {
		t.Error("expected a network error, got nil")
	}
}

func TestErrorResponseErrorMessage(t *testing.T) {
	cases := []struct {
		name string
		err  *ErrorResponse
		want string
	}{
		{
			name: "with error type and description",
			err:  &ErrorResponse{StatusCode: 404, ErrorType: "not_found", Description: "no such project"},
			want: "modrinth API returned status 404: not_found - no such project",
		},
		{
			name: "bare status code only",
			err:  &ErrorResponse{StatusCode: 500},
			want: "modrinth API returned status 500",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Errorf("Error() = %q, want %q", got, c.want)
			}
		})
	}
}

// sanity check that ErrorResponse round-trips through encoding/json the way
// the do() fallback decode expects.
func TestErrorResponseJSONShape(t *testing.T) {
	data := []byte(`{"error":"rate_limited","description":"slow down"}`)
	var errResp ErrorResponse
	if err := json.Unmarshal(data, &errResp); err != nil {
		t.Fatalf("Unmarshal() returned error: %v", err)
	}
	if errResp.ErrorType != "rate_limited" || errResp.Description != "slow down" {
		t.Errorf("got %+v, want ErrorType=rate_limited Description=%q", errResp, "slow down")
	}
}
