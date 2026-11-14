package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testGitHubResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
	}
}

type slowGitHubBody struct {
	io.ReadCloser
	finished func()
}

func (b slowGitHubBody) Close() error {
	time.Sleep(time.Second)
	b.finished()
	return b.ReadCloser.Close()
}

func TestGitHubClientBoundsAndPacesReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := newGitHubClient("test-token")
		var active atomic.Int64
		var startsMu sync.Mutex
		var starts []time.Time
		client.http.Transport = testTransport(func(_ *http.Request) (*http.Response, error) {
			if count := active.Add(1); count > 4 {
				t.Errorf("Started %d concurrent reads", count)
			}
			startsMu.Lock()
			starts = append(starts, time.Now())
			startsMu.Unlock()
			resp := testGitHubResponse(http.StatusOK)
			resp.Body = slowGitHubBody{resp.Body, func() { active.Add(-1) }}
			return resp, nil
		})

		start := time.Now()
		var requests sync.WaitGroup
		for range 12 {
			requests.Go(func() {
				if err := client.request(t.Context(), http.MethodGet, "/settings", nil, nil); err != nil {
					t.Error(err)
				}
			})
		}
		time.Sleep(400 * time.Millisecond)
		if got := active.Load(); got != 4 {
			t.Errorf("Concurrent reads = %d, want 4", got)
		}
		requests.Wait()
		if elapsed := time.Since(start); elapsed >= 4*time.Second {
			t.Errorf("Reads took %s, want less than 4s", elapsed)
		}
		for i := 1; i < len(starts); i++ {
			if gap := starts[i].Sub(starts[i-1]); gap < 125*time.Millisecond-time.Microsecond {
				t.Errorf("Read %d started %s after the previous read", i+1, gap)
			}
		}
	})
}

func TestGitHubClientKeepsWritesExclusiveAndSpaced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := newGitHubClient("test-token")
		unblock := make(chan struct{})
		start := time.Now()
		var mu sync.Mutex
		var paths []string
		var writeTimes []time.Time
		client.http.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			paths = append(paths, req.URL.Path)
			if req.Method == http.MethodPatch {
				writeTimes = append(writeTimes, time.Now())
			}
			mu.Unlock()
			if req.URL.Path == "/first-read" {
				<-unblock
			}
			return testGitHubResponse(http.StatusOK), nil
		})
		var requests sync.WaitGroup
		for _, item := range []struct{ method, path string }{
			{http.MethodGet, "/first-read"},
			{http.MethodPatch, "/first-write"},
			{http.MethodPatch, "/second-write"},
			{http.MethodGet, "/second-read"},
		} {
			requests.Go(func() {
				if err := client.request(t.Context(), item.method, item.path, nil, nil); err != nil {
					t.Error(err)
				}
			})
			synctest.Wait()
		}
		if !slices.Equal(paths, []string{"/first-read"}) {
			t.Errorf("Requests started while a read was blocking the queued writes: %v", paths)
		}
		close(unblock)
		requests.Wait()
		if !slices.Equal(paths, []string{"/first-read", "/first-write", "/second-write", "/second-read"}) {
			t.Errorf("Request order = %v", paths)
		}
		if len(writeTimes) != 2 || !writeTimes[0].Equal(start) || writeTimes[1].Sub(writeTimes[0]) < time.Second {
			t.Errorf("Write times = %v, want at least 1s between writes", writeTimes)
		}
	})
}

func TestGitHubClientStopsQueuedReadsAndKeepsLongestCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := newGitHubClient("test-token")
		var calls atomic.Int64
		client.http.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			resp := testGitHubResponse(http.StatusTooManyRequests)
			switch req.URL.Path {
			case "/long":
				time.Sleep(200 * time.Millisecond)
				resp.Header.Set("Retry-After", "10")
			case "/short":
				time.Sleep(150 * time.Millisecond)
				resp.Header.Set("Retry-After", "1")
			default:
				t.Errorf("Unexpected request during cooldown: %s", req.URL.Path)
			}
			return resp, nil
		})
		var requests sync.WaitGroup
		for _, path := range []string{"/long", "/short", "/queued", "/also-queued", "/beyond-limit"} {
			requests.Go(func() {
				err := client.request(t.Context(), http.MethodGet, path, nil, nil)
				if err == nil || !strings.Contains(err.Error(), "2000-01-01T00:00:10Z") {
					t.Errorf("%s got %v, want the longest cooldown", path, err)
				}
			})
			synctest.Wait()
		}
		requests.Wait()
		if got := calls.Load(); got != 2 {
			t.Errorf("Transport received %d requests, want 2", got)
		}
	})
}

func TestGitHubClientCancelsQueuedRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := newGitHubClient("test-token")
		unblock := make(chan struct{})
		client.http.Transport = testTransport(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/blocking":
				<-unblock
			case "/canceled":
				t.Error("Canceled request reached the transport")
			}
			return testGitHubResponse(http.StatusOK), nil
		})
		done := make(chan error, 1)
		go func() { done <- client.request(t.Context(), http.MethodPatch, "/blocking", nil, nil) }()
		synctest.Wait()
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		if err := client.request(ctx, http.MethodGet, "/canceled", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Queued read returned %v, want deadline exceeded", err)
		}
		close(unblock)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := client.request(t.Context(), http.MethodGet, "/next", nil, nil); err != nil {
			t.Fatal(err)
		}
	})
}
