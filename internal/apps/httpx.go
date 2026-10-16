// Package apps holds per-app API clients and wiring steps (T-7). Every
// endpoint used here is recorded in docs/COMPATIBILITY.md (CC-1).
package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPError is a non-2xx response.
type HTTPError struct {
	Method, Path string
	Status       int
	Body         string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.Path, e.Status, e.Body)
}

// client is a small JSON HTTP client bound to one app.
type client struct {
	base   string
	auth   func(*http.Request)
	http   *http.Client
	maxErr int
}

func newClient(base string, auth func(*http.Request)) *client {
	return &client{base: base, auth: auth, http: &http.Client{Timeout: 30 * time.Second}, maxErr: 300}
}

// do sends body as JSON (if non-nil) and decodes a JSON response into out (if non-nil).
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s %s: %w", method, path, err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.auth != nil {
		c.auth(req)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := string(bytes.TrimSpace(raw))
		if len(msg) > c.maxErr {
			msg = msg[:c.maxErr] + "…"
		}
		return &HTTPError{Method: method, Path: path, Status: resp.StatusCode, Body: msg}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// isStatus reports whether err is an HTTPError with the given status.
func isStatus(err error, status int) bool {
	he, ok := err.(*HTTPError)
	return ok && he.Status == status
}
