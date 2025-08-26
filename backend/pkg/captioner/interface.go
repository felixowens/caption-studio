package captioners

// Captioner defines the interface for image captioning services
type Captioner interface {
	CaptionSingle(imageBase64, systemPrompt string) (string, error)
	CaptionEdit(imageABase64, imageBBase64, systemPrompt string) (string, error)
}

// Config holds configuration for creating a captioner
type Config struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
}

// NewCaptioner creates a captioner instance based on the config
func NewCaptioner(config *Config) (Captioner, error) {
	switch config.Provider {
	case "gemini":
		return NewGemini(config.APIKey), nil
	default:
		return nil, ErrUnsupportedProvider(config.Provider)
	}
}