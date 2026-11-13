package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type gitHubClient struct {
	baseURL  string
	token    string
	http     *http.Client
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

func newGitHubClient(token string) *gitHubClient {
	return &gitHubClient{
		baseURL:  "https://api.github.com",
		token:    token,
		interval: 200 * time.Millisecond,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type statusError struct {
	method string
	path   string
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("%s %s returned HTTP %d", e.method, e.path, e.status)
}

func (c *gitHubClient) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	scheduled := now
	if c.next.After(now) {
		scheduled = c.next
	}
	c.next = scheduled.Add(c.interval)
	c.mu.Unlock()

	if !scheduled.After(now) {
		return ctx.Err()
	}
	timer := time.NewTimer(time.Until(scheduled))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *gitHubClient) request(ctx context.Context, method, path string, payload any, response any) error {
	if err := c.wait(ctx); err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &statusError{method: method, path: path, status: resp.StatusCode}
	}
	if response == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(response)
}
