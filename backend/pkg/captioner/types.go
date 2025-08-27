package captioner

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Provider represents the provider of the captioner
type Provider string

// Model represents the model of the captioner
type Model string

// MimeType represents the MIME type of the image
type MimeType string

const (
	MimeTypeJPEG MimeType = "image/jpeg"
	MimeTypePNG  MimeType = "image/png"
	MimeTypeGIF  MimeType = "image/gif"
	MimeTypeWebP MimeType = "image/webp"
)

func (p Provider) String() string {
	return string(p)
}

func (m Model) String() string {
	return string(m)
}

func (m MimeType) String() string {
	return string(m)
}

// Config holds configuration for creating a captioner
type Config struct {
	Provider Provider `json:"provider"`
	ApiKey   string   `json:"apiKey"`
	Model    Model    `json:"model,omitempty"`
}

func NewConfig(provider Provider, apiKey string, model Model) (*Config, error) {
	if provider == "" {
		return nil, fmt.Errorf("provider is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("apiKey is required")
	}
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	return &Config{
		Provider: provider,
		ApiKey:   apiKey,
		Model:    model,
	}, nil
}

type Image struct {
	Data     []byte   `json:"data"`
	MimeType MimeType `json:"mimeType"`
}

func NewImageFromBytes(data []byte, mimeType MimeType) (*Image, error) {
	// TODO: resize image if needed?
	if len(data) == 0 {
		return nil, fmt.Errorf("data is required")
	}
	if mimeType == "" {
		return nil, fmt.Errorf("mimeType is required")
	}

	return &Image{Data: data, MimeType: mimeType}, nil
}

func NewImageFromBase64(b64 string, mimeType MimeType) (*Image, error) {
	if i := strings.Index(b64, ","); i >= 0 && strings.Contains(b64[:i], ";base64") {
		b64 = b64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %v", err)
	}
	return NewImageFromBytes(raw, mimeType)
}

func (i *Image) Base64() string {
	return base64.StdEncoding.EncodeToString(i.Data)
}
