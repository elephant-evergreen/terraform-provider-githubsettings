package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
)

const maxConcurrentReads = 4
const readsPerSecond = 8

type gitHubClient struct {
	baseURL   string
	token     string
	http      *http.Client
	interval  time.Duration
	requests  *semaphore.Weighted
	reads     *rate.Limiter
	nextWrite time.Time
	retryMu   sync.Mutex
	retryAt   time.Time
}

func newGitHubClient(token string) *gitHubClient {
	transport := http.DefaultTransport
	if base, ok := transport.(*http.Transport); ok {
		pool := base.Clone()
		pool.MaxIdleConnsPerHost = maxConcurrentReads
		transport = pool
	}
	return &gitHubClient{
		baseURL:  "https://api.github.com",
		token:    token,
		interval: time.Second,
		requests: semaphore.NewWeighted(maxConcurrentReads),
		reads:    rate.NewLimiter(readsPerSecond, 1),
		http: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
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

func waitUntil(ctx context.Context, scheduled time.Time) error {
	delay := time.Until(scheduled)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func rateLimitReset(resp *http.Response, now time.Time) time.Time {
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusForbidden {
		return time.Time{}
	}
	if value := resp.Header.Get("Retry-After"); value != "" {
		if delay, err := strconv.ParseInt(value, 10, 32); err == nil && delay >= 0 {
			return now.Add(time.Duration(delay) * time.Second)
		}
		if retryAt, err := http.ParseTime(value); err == nil {
			return retryAt
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return time.Unix(reset, 0)
		}
		return now.Add(time.Minute)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return now.Add(time.Minute)
	}

	var reply struct {
		Message string `json:"message"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err == nil && json.Unmarshal(data, &reply) == nil &&
		strings.Contains(strings.ToLower(reply.Message), "secondary rate limit") {
		return now.Add(time.Minute)
	}
	return time.Time{}
}

func rateLimitError(retryAt time.Time) error {
	return fmt.Errorf("GitHub API rate limit reached; retry after %s", retryAt.UTC().Format(time.RFC3339))
}

func (c *gitHubClient) cooldown() error {
	c.retryMu.Lock()
	defer c.retryMu.Unlock()
	if c.retryAt.After(time.Now()) {
		return rateLimitError(c.retryAt)
	}
	return nil
}

func (c *gitHubClient) recordCooldown(retryAt time.Time) error {
	c.retryMu.Lock()
	defer c.retryMu.Unlock()
	if retryAt.After(c.retryAt) {
		c.retryAt = retryAt
	}
	return rateLimitError(c.retryAt)
}

func (c *gitHubClient) request(ctx context.Context, method, path string, payload any, response any) error {
	if err := c.cooldown(); err != nil {
		return err
	}
	mutation := method != http.MethodGet
	weight := int64(1)
	if mutation {
		weight = maxConcurrentReads
	}
	if err := c.requests.Acquire(ctx, weight); err != nil {
		return err
	}
	defer c.requests.Release(weight)
	if err := c.cooldown(); err != nil {
		return err
	}

	var err error
	if mutation {
		err = waitUntil(ctx, c.nextWrite)
	} else {
		err = c.reads.Wait(ctx)
	}
	if err != nil {
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

	if err := c.cooldown(); err != nil {
		return err
	}
	if mutation {
		c.nextWrite = time.Now().Add(c.interval)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if retryAt := rateLimitReset(resp, time.Now()); !retryAt.IsZero() {
			return c.recordCooldown(retryAt)
		}
		return &statusError{method: method, path: path, status: resp.StatusCode}
	}
	if response == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
		return err
	}
	const maxResponse = 4 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return fmt.Errorf("GitHub returned an incomplete or oversized response")
	}
	if err := json.Unmarshal(data, response); err != nil {
		return fmt.Errorf("could not decode GitHub response: %w", err)
	}
	return nil
}
