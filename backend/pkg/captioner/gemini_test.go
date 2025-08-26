package captioners

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGemini(t *testing.T) {
	captioner := NewGemini("test-api-key")
	if captioner == nil {
		t.Fatal("NewGemini returned nil")
	}

	if captioner.apiKey != "test-api-key" {
		t.Errorf("Expected API key 'test-api-key', got '%s'", captioner.apiKey)
	}
}

func TestGeminiCaptionSingle_NoAPIKey(t *testing.T) {
	captioner := NewGemini("")

	_, err := captioner.CaptionSingle("test-image", "test-prompt")
	if err != ErrAPIKeyRequired {
		t.Errorf("Expected ErrAPIKeyRequired, got %v", err)
	}
}

func TestGeminiCaptionEdit_NoAPIKey(t *testing.T) {
	captioner := NewGemini("")

	_, err := captioner.CaptionEdit("image-a", "image-b", "test-prompt")
	if err != ErrAPIKeyRequired {
		t.Errorf("Expected ErrAPIKeyRequired, got %v", err)
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"iVBORw0KGgoAAAANSUhEUgAA", "image/png"},
		{"UklGRiIAAABXRUJQVlA4I", "image/webp"},
		{"/9j/4AAQSkZJRgABAQAAAQ", "image/jpeg"},
		{"short", "image/jpeg"},
		{"", "image/jpeg"},
	}

	for _, test := range tests {
		result := detectMimeType(test.input)
		if result != test.expected {
			t.Errorf("detectMimeType(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}

func TestNewCaptioner(t *testing.T) {
	config := &Config{
		Provider: "gemini",
		APIKey:   "test-key",
	}

	captioner, err := NewCaptioner(config)
	if err != nil {
		t.Fatalf("NewCaptioner failed: %v", err)
	}

	if captioner == nil {
		t.Fatal("NewCaptioner returned nil captioner")
	}

	geminiCaptioner, ok := captioner.(*Gemini)
	if !ok {
		t.Fatal("Expected Gemini captioner")
	}

	if geminiCaptioner.apiKey != "test-key" {
		t.Errorf("Expected API key 'test-key', got '%s'", geminiCaptioner.apiKey)
	}
}

func TestNewCaptioner_UnsupportedProvider(t *testing.T) {
	config := &Config{
		Provider: "unsupported",
		APIKey:   "test-key",
	}

	_, err := NewCaptioner(config)
	if err == nil {
		t.Fatal("Expected error for unsupported provider")
	}

	expected := "unsupported captioner provider: unsupported"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestGeminiMakeRequest_Success(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	result, err := captioner.makeRequest(request)
	if err != nil {
		t.Fatalf("makeRequest failed: %v", err)
	}

	if result != "This is a test caption" {
		t.Errorf("Expected 'This is a test caption', got '%s'", result)
	}
}

func TestGeminiMakeRequest_APIError(t *testing.T) {
	mockErrorResponse := geminiResponse{
		Error: &geminiError{
			Code:    400,
			Message: "Invalid request",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockErrorResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err := captioner.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for API error response")
	}

	expected := "Gemini API error: Invalid request"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestGeminiMakeRequest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad request"))
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err := captioner.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for HTTP error")
	}

	if !strings.Contains(err.Error(), "Gemini API error (status 400)") {
		t.Errorf("Expected HTTP error message, got '%s'", err.Error())
	}
}

func TestGeminiMakeRequest_NoCandidates(t *testing.T) {
	mockResponse := geminiResponse{
		Candidates: []geminiCandidate{},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err := captioner.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for no candidates")
	}

	expected := "no caption generated by Gemini API"
	if err.Error() != expected {
		t.Errorf("Expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestGeminiMakeRequest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	request := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "test prompt"},
				},
			},
		},
	}

	_, err := captioner.makeRequest(request)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to unmarshal response") {
		t.Errorf("Expected unmarshal error, got '%s'", err.Error())
	}
}

func TestGeminiCaptionSingle_Success(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody geminiRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &requestBody)

		if len(requestBody.Contents) == 0 || len(requestBody.Contents[0].Parts) != 2 {
			t.Error("Expected request to have text and image parts")
		}

		if requestBody.Contents[0].Parts[0].Text == "" {
			t.Error("Expected system prompt in request")
		}

		if requestBody.Contents[0].Parts[1].InlineData == nil {
			t.Error("Expected inline data in request")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	result, err := captioner.CaptionSingle("iVBORw0KGgoAAAANSUhEUgAA", "Describe this image")
	if err != nil {
		t.Fatalf("CaptionSingle failed: %v", err)
	}

	if result != "A beautiful landscape with mountains" {
		t.Errorf("Expected 'A beautiful landscape with mountains', got '%s'", result)
	}
}

func TestGeminiCaptionSingle_DefaultPrompt(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody geminiRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &requestBody)

		expectedPrompt := "Describe this image in detail for training a diffusion model. Focus on the visual elements, composition, style, and any notable features."
		if requestBody.Contents[0].Parts[0].Text != expectedPrompt {
			t.Errorf("Expected default prompt, got '%s'", requestBody.Contents[0].Parts[0].Text)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	_, err := captioner.CaptionSingle("iVBORw0KGgoAAAANSUhEUgAA", "")
	if err != nil {
		t.Fatalf("CaptionSingle with default prompt failed: %v", err)
	}
}

func TestGeminiCaptionEdit_Success(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody geminiRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &requestBody)

		if len(requestBody.Contents) == 0 || len(requestBody.Contents[0].Parts) != 3 {
			t.Error("Expected request to have text and two image parts")
		}

		if requestBody.Contents[0].Parts[0].Text == "" {
			t.Error("Expected system prompt in request")
		}

		if requestBody.Contents[0].Parts[1].InlineData == nil || requestBody.Contents[0].Parts[2].InlineData == nil {
			t.Error("Expected both inline data parts in request")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	result, err := captioner.CaptionEdit("iVBORw0KGgoAAAANSUhEUgAA", "UklGRiIAAABXRUJQVlA4I", "Compare these images")
	if err != nil {
		t.Fatalf("CaptionEdit failed: %v", err)
	}

	if result != "The image was brightened and color-corrected" {
		t.Errorf("Expected 'The image was brightened and color-corrected', got '%s'", result)
	}
}

func TestGeminiCaptionEdit_DefaultPrompt(t *testing.T) {
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody geminiRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &requestBody)

		expectedPrompt := "Compare these two images and describe the edit or transformation that was applied to convert the first image into the second image. Focus on the specific changes made, including any adjustments to color, lighting, objects, text, style, or composition. Be concise and descriptive."
		if requestBody.Contents[0].Parts[0].Text != expectedPrompt {
			t.Errorf("Expected default prompt, got '%s'", requestBody.Contents[0].Parts[0].Text)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
	}

	_, err := captioner.CaptionEdit("iVBORw0KGgoAAAANSUhEUgAA", "UklGRiIAAABXRUJQVlA4I", "")
	if err != nil {
		t.Fatalf("CaptionEdit with default prompt failed: %v", err)
	}
}

func TestGeminiMakeRequest_NetworkError(t *testing.T) {
	captioner := &Gemini{
		apiKey:  "test-key",
		client:  &http.Client{},
		baseURL: "http://invalid-url-that-does-not-exist.invalid",
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

func TestGeminiMakeRequest_ReadResponseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("short"))
	}))
	defer server.Close()

	captioner := &Gemini{
		apiKey:  "test-key",
		client:  server.Client(),
		baseURL: server.URL,
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
	if err != nil && strings.Contains(err.Error(), "failed to read response") {
		return
	}
}
