package image

import "fmt"

// ErrNotSupported is returned when an executor doesn't support a specific operation.
var ErrNotSupported = fmt.Errorf("operation not supported")

// ImageExecutor is the interface for image generation providers.
//
// Standard format: OpenAI /v1/images/generations compatible.
// Each executor converts standard types to/from the provider's native format.
//
// Sync providers (GPT Image, Qwen, etc.) return completed tasks directly.
// Async providers (Midjourney) return pending tasks; client polls via GetTask.
type ImageExecutor interface {
	// Init initializes the executor with channel configuration.
	Init(channel any) // *model.Channel

	// GetName returns the human-readable executor name.
	GetName() string

	// TextToImage generates images from a text prompt.
	// Sync providers return a completed task; async providers return a pending task.
	TextToImage(req *TextToImageRequest) (*ImageTask, error)

	// ImageToImage performs image-to-image generation (edit, inpainting, variation).
	// Returns ErrNotSupported if the provider lacks this capability.
	ImageToImage(req *ImageToImageRequest) (*ImageTask, error)

	// GetTask queries the status of an async image generation task.
	// Each call proxies directly to the upstream provider.
	GetTask(taskID string) (*ImageTask, error)
}
