package captioner

import (
	"context"
)

// Captioner defines the interface for image captioning services
type Captioner interface {
	CaptionSingle(ctx context.Context, img Image, systemPrompt string) (string, error)
	CaptionEdit(ctx context.Context, imgA, imgB Image, systemPrompt string) (string, error)
}
