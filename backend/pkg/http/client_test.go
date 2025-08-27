package http

import (
	"context"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// zero backoff so tests don't sleep
func zeroBackoff(int) time.Duration { return 0 }

func Test_SuccessWithoutRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(3),
	)

	req, _ := stdhttp.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 hit, got %d", got)
	}
}

func Test_RetryOn500ThenSucceed(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n <= 2 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(5),
	)
	req, _ := stdhttp.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func Test_NoRetryOn4xx_ReturnsResponse(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(3),
	)
	req, _ := stdhttp.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("http.Client semantics: want nil error on 4xx; got %v", err)
	}
	if resp.StatusCode != 404 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}
}

func Test_MaxRetriesExceeded(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(2),
	)
	req, _ := stdhttp.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err == nil || resp != nil {
		t.Fatalf("expected error and nil resp; got resp=%v err=%v", resp, err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 { // initial + 2 retries
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

type errRT struct{}

func (errRT) RoundTrip(*stdhttp.Request) (*stdhttp.Response, error) { return nil, errors.New("boom") }

func Test_RetryOnTransportError(t *testing.T) {
	c := NewClient(
		WithHTTPClient(&stdhttp.Client{Transport: errRT{}}),
		WithBackoff(zeroBackoff),
		WithMaxRetries(2),
	)
	req, _ := stdhttp.NewRequest("GET", "http://example.invalid", nil)
	resp, err := c.Do(req)
	if err == nil || resp != nil {
		t.Fatalf("expected error and nil resp; got resp=%v err=%v", resp, err)
	}
}

func Test_ContextCancelStopsRetries(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		// Make backoff non-zero so the select can hit ctx.Done()
		WithBackoff(FixedBackoff(200*time.Millisecond)),
		WithMaxRetries(5),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	req, _ := stdhttp.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || resp != nil {
		t.Fatalf("expected context error; got resp=%v err=%v", resp, err)
	}
}

func Test_BodyIsReplayedWhenGetBodyIsSet(t *testing.T) {
	var hits int32
	var bodies []string
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		atomic.AddInt32(&hits, 1)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if hits < 2 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(3),
	)

	// NewRequest with a strings.Reader sets GetBody for us.
	req, _ := stdhttp.NewRequest("POST", srv.URL, strings.NewReader("hello"))
	req.ContentLength = int64(len("hello"))

	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("got resp=%v err=%v", resp, err)
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", got)
	}
	if len(bodies) < 2 || bodies[0] != "hello" || bodies[1] != "hello" {
		t.Fatalf("body was not replayed correctly: %v", bodies)
	}
}

func Test_TimedOut(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := NewClient(
		WithHTTPClient(srv.Client()),
		WithBackoff(zeroBackoff),
		WithMaxRetries(3),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	req, _ := stdhttp.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || resp != nil {
		t.Fatalf("expected context error; got resp=%v err=%v", resp, err)
	}
}
