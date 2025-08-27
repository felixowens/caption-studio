package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	captionerPkg "main/pkg/captioner"
	"main/pkg/testutils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var TestEndpoint = url.URL{
	Scheme: "http",
	Host:   "invalid-url-that-does-not-exist.invalid",
}

func createTestImage() captionerPkg.Image {
	return captionerPkg.Image{
		Data:     []byte("iVBORw0KGgoAAAANSUhEUgAA"),
		MimeType: captionerPkg.MimeTypePNG,
	}
}

func TestNewGemini(t *testing.T) {
	captioner, err := NewGemini("test-api-key", ModelGeminiPro, nil)
	if err != nil {
		t.Fatalf("NewGemini failed: %v", err)
	}
	if captioner == nil {
		t.Fatal("NewGemini returned nil")
	}

	if captioner.apiKey != "test-api-key" {
		t.Errorf("Expected API key 'test-api-key', got '%s'", captioner.apiKey)
	}
}

func TestNewCaptioner(t *testing.T) {
	config := &captionerPkg.Config{
		ApiKey: "test-key",
	}

	captioner, err := NewGemini(config.ApiKey, ModelGeminiPro, nil)
	if err != nil {
		t.Fatalf("NewCaptioner failed: %v", err)
	}

	if captioner == nil {
		t.Fatal("NewCaptioner returned nil captioner")
	}

	if captioner.apiKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got '%s'", captioner.apiKey)
	}
}

func TestGemini_CaptionSingle(t *testing.T) {
	tests := []struct {
		name           string
		setupClient    func(*testutils.TestHTTPClient)
		image          captionerPkg.Image
		systemPrompt   string
		expectedResult string
		expectError    bool
	}{
		{
			name: "successful caption generation",
			setupClient: func(client *testutils.TestHTTPClient) {
				resp := &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"A beautiful landscape"}]}}]}`)),
					Header:     make(http.Header),
				}
				client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)
			},
			image:          createTestImage(), // Your test image helper
			systemPrompt:   "Describe this image",
			expectedResult: "A beautiful landscape",
			expectError:    false,
		},
		{
			name: "API error response",
			setupClient: func(client *testutils.TestHTTPClient) {
				resp := &http.Response{
					StatusCode: 400,
					Body:       io.NopCloser(strings.NewReader(`{"error":{"code":400,"message":"Invalid request"}}`)),
					Header:     make(http.Header),
				}
				client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)
			},
			image:        createTestImage(),
			systemPrompt: "Describe this image",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutils.NewTestHTTPClient()
			tt.setupClient(client)

			gemini, err := NewGemini("test-key", ModelGeminiPro, client)
			require.NoError(t, err)

			result, err := gemini.CaptionSingle(context.Background(), tt.image, tt.systemPrompt)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestGeminiMakeRequest_Success(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "This is a test caption"},
					},
				},
			},
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	result, err := gemini.makeRequest(request)
	if err != nil {
		t.Fatalf("makeRequest failed: %v", err)
	}

	if result != "This is a test caption" {
		t.Errorf("Expected 'This is a test caption', got '%s'", result)
	}
}

func TestGeminiMakeRequest_APIError(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockErrorResponse := geminiResponse{
		Error: &geminiError{
			Code:    400,
			Message: "Invalid request",
		},
	}

	responseBody, _ := json.Marshal(mockErrorResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err = gemini.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for API error response")
	}

	expected := "Gemini API error: Invalid request"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestGeminiMakeRequest_HTTPError(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader("Bad request")),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err = gemini.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for HTTP error")
	}

	if !strings.Contains(err.Error(), "Gemini API error (status 400)") {
		t.Errorf("Expected HTTP error message, got '%s'", err.Error())
	}
}

func TestGeminiMakeRequest_NoCandidates(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err = gemini.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for no candidates")
	}

	expected := "no caption generated by Gemini API"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestGeminiMakeRequest_InvalidJSON(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("invalid json")),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err = gemini.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Errorf("Expected unmarshal error, got '%s'", err.Error())
	}
}

func TestGeminiCaptionSingle_Success(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "A beautiful landscape with mountains"},
					},
				},
			},
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	result, err := gemini.CaptionSingle(context.Background(), captionerPkg.Image{Data: []byte("iVBORw0KGgoAAAANSUhEUgAA"), MimeType: captionerPkg.MimeTypePNG}, "Describe this image")
	if err != nil {
		t.Fatalf("CaptionSingle failed: %v", err)
	}

	if result != "A beautiful landscape with mountains" {
		t.Errorf("Expected 'A beautiful landscape with mountains', got '%s'", result)
	}
}

func TestGeminiCaptionSingle_DefaultPrompt(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Default caption"},
					},
				},
			},
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	_, err = gemini.CaptionSingle(context.Background(), captionerPkg.Image{Data: []byte("iVBORw0KGgoAAAANSUhEUgAA"), MimeType: captionerPkg.MimeTypePNG}, "")
	if err != nil {
		t.Fatalf("CaptionSingle with default prompt failed: %v", err)
	}
}

func TestGeminiCaptionEdit_Success(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "The image was brightened and color-corrected"},
					},
				},
			},
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	result, err := gemini.CaptionEdit(context.Background(), captionerPkg.Image{Data: []byte("iVBORw0KGgoAAAANSUhEUgAA"), MimeType: captionerPkg.MimeTypePNG}, captionerPkg.Image{Data: []byte("UklGRiIAAABXRUJQVlA4I"), MimeType: captionerPkg.MimeTypePNG}, "Compare these images")
	if err != nil {
		t.Fatalf("CaptionEdit failed: %v", err)
	}

	if result != "The image was brightened and color-corrected" {
		t.Errorf("Expected 'The image was brightened and color-corrected', got '%s'", result)
	}
}

func TestGeminiCaptionEdit_DefaultPrompt(t *testing.T) {
	client := testutils.NewTestHTTPClient()

	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content: geminiContent{
					Parts: []geminiPart{
						{Text: "Default edit description"},
					},
				},
			},
		},
	}

	responseBody, _ := json.Marshal(mockResponse)
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(string(responseBody))),
		Header:     make(http.Header),
	}
	client.SetResponse("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:generateContent?key=test-key", resp)

	gemini, err := NewGemini("test-key", ModelGeminiPro, client)
	require.NoError(t, err)

	_, err = gemini.CaptionEdit(context.Background(), captionerPkg.Image{Data: []byte("iVBORw0KGgoAAAANSUhEUgAA"), MimeType: captionerPkg.MimeTypePNG}, captionerPkg.Image{Data: []byte("UklGRiIAAABXRUJQVlA4I"), MimeType: captionerPkg.MimeTypePNG}, "")
	if err != nil {
		t.Fatalf("CaptionEdit with default prompt failed: %v", err)
	}
}

func TestGeminiMakeRequest_NetworkError(t *testing.T) {
	captioner := &Gemini{
		apiKey: "test-key",
		client: &http.Client{},
		endpoint: url.URL{
			Scheme: "http",
			Host:   "invalid-url-that-does-not-exist.invalid",
		},
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test"},
				},
			},
		},
	}

	_, err := captioner.makeRequest(request)
	if err == nil || !strings.Contains(err.Error(), "failed to call Gemini API") {
		t.Errorf("Expected network error, got: %v", err)
	}
}
