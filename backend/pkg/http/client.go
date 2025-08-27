package http

import (
	"fmt"
	"math"
	"net/http"
	"time"
)

// Client wraps http.Client with retry capabilities
type Client struct {
	*http.Client
	maxRetries int
	backoff    BackoffFunc
}

// BackoffFunc defines how to calculate delay between retries
type BackoffFunc func(attempt int) time.Duration

// NewClient creates a new Client with retry capabilities
func NewClient(opts ...Option) *Client {
	c := &Client{
		Client:     &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		backoff:    ExponentialBackoff(100*time.Millisecond, 2.0),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Option allows configuring the Client
type Option func(*Client)

// WithHTTPClient sets the underlying http.Client
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.Client = client
	}
}

// WithMaxRetries sets the maximum number of retries
func WithMaxRetries(retries int) Option {
	return func(c *Client) {
		c.maxRetries = retries
	}
}

// WithBackoff sets the backoff function
func WithBackoff(backoff BackoffFunc) Option {
	return func(c *Client) {
		c.backoff = backoff
	}
}

// WithTimeout sets the client timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.Client.Timeout = timeout
	}
}

// Do executes the request with retry logic
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		reqClone := req.Clone(req.Context())

		resp, err := c.Client.Do(reqClone)
		if err == nil && !shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}

		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		}

		if attempt < c.maxRetries {
			delay := c.backoff(attempt)
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// shouldRetry determines if a response should be retried based on status code
func shouldRetry(statusCode int) bool {
	return statusCode >= 500 || statusCode == 429 || statusCode == 408
}

// ExponentialBackoff creates an exponential backoff function
func ExponentialBackoff(baseDelay time.Duration, multiplier float64) BackoffFunc {
	return func(attempt int) time.Duration {
		delay := float64(baseDelay) * math.Pow(multiplier, float64(attempt))
		return time.Duration(delay)
	}
}

// LinearBackoff creates a linear backoff function
func LinearBackoff(delay time.Duration) BackoffFunc {
	return func(attempt int) time.Duration {
		return delay * time.Duration(attempt+1)
	}
}

// FixedBackoff creates a fixed delay backoff function
func FixedBackoff(delay time.Duration) BackoffFunc {
	return func(_ int) time.Duration {
		return delay
	}
}
