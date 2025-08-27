package captioner

import "fmt"

// ErrUnsupportedProvider is returned when an unsupported provider is requested
func ErrUnsupportedProvider(provider Provider) error {
	return fmt.Errorf("unsupported captioner provider: %s", provider)
}
