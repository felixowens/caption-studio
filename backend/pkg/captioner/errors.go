package captioners

import "fmt"

// ErrUnsupportedProvider is returned when an unsupported provider is requested
func ErrUnsupportedProvider(provider string) error {
	return fmt.Errorf("unsupported captioner provider: %s", provider)
}

// ErrAPIKeyRequired is returned when no API key is provided
var ErrAPIKeyRequired = fmt.Errorf("API key is required")