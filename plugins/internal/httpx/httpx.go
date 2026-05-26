// Package httpx provides small HTTP helpers shared by Warden's integration
// plugins: a default client with timeouts, JSON request/response helpers,
// and a typed error for non-2xx responses that preserves the upstream
// status code and body for the audit log.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultTimeout is the per-request timeout applied by NewClient when no
// override is supplied. Plugins may override on a per-call basis using a
// context.WithTimeout.
const DefaultTimeout = 30 * time.Second

// NewClient returns an *http.Client configured with a sensible timeout. The
// transport is the package-default so it benefits from connection pooling
// across plugins.
func NewClient() *http.Client {
	return &http.Client{Timeout: DefaultTimeout}
}

// APIError represents a non-2xx response from an upstream API.
type APIError struct {
	StatusCode int
	Status     string
	URL        string
	Method     string
	Body       string
}

// Error implements error.
func (e *APIError) Error() string {
	return fmt.Sprintf("httpx: %s %s: %d %s: %s", e.Method, e.URL, e.StatusCode, e.Status, truncate(e.Body, 512))
}

// IsAPIError reports whether err is (or wraps) an APIError with the given
// status code. A statusCode of 0 matches any APIError.
func IsAPIError(err error, statusCode int) bool {
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return statusCode == 0 || ae.StatusCode == statusCode
}

// DoJSON sends req and decodes a JSON response into out (if non-nil). A
// non-2xx response is returned as an *APIError so callers can recover the
// upstream status without re-reading the body.
func DoJSON(ctx context.Context, client *http.Client, req *http.Request, out any) error {
	if client == nil {
		client = NewClient()
	}
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("httpx: do %s %s: %w", req.Method, req.URL.String(), err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("httpx: read body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			URL:        req.URL.String(),
			Method:     req.Method,
			Body:       string(body),
		}
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("httpx: decode response: %w", err)
	}
	return nil
}

// NewJSONRequest builds a JSON-bodied request. Pass body == nil to send a
// bodyless request. Method, URL, and headers must be set by the caller (or
// added on the returned *http.Request); this function only sets the
// Content-Type and Accept headers when a body is present.
func NewJSONRequest(method, url string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("httpx: marshal body: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		return nil, fmt.Errorf("httpx: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// JoinURL joins a base URL and one or more path segments with a single `/`
// separator, preserving any query string already on base. It is intentionally
// dumb: it does not URL-encode segments. Callers must encode user input
// themselves before passing it in.
func JoinURL(base string, segments ...string) string {
	out := strings.TrimRight(base, "/")
	for _, s := range segments {
		out += "/" + strings.TrimLeft(s, "/")
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
