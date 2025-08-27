package testutils

import (
	"io"
	"net/http"
	"strings"
)

type TestHTTPClient struct {
	responses map[string]*http.Response
	errors    map[string]error
}

func NewTestHTTPClient() *TestHTTPClient {
	return &TestHTTPClient{
		responses: make(map[string]*http.Response),
		errors:    make(map[string]error),
	}
}

func (t *TestHTTPClient) Post(url, contentType string, body io.Reader) (*http.Response, error) {
	if err, exists := t.errors[url]; exists {
		return nil, err
	}

	if resp, exists := t.responses[url]; exists {
		return resp, nil
	}

	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`SUCCESS`)),
		Header:     make(http.Header),
	}, nil
}

func (t *TestHTTPClient) SetResponse(url string, response *http.Response) {
	t.responses[url] = response
}

func (t *TestHTTPClient) SetError(url string, err error) {
	t.errors[url] = err
}
