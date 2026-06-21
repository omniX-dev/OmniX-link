// Package image defines image generation types and executor interface for Omni-link.
//
// Standard format: OpenAI /v1/images/generations compatible.
// All providers adapt to/from this standard via their executor.
package image

// TaskStatus represents the status of an async image generation task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// IsTerminal returns true if the task has reached a terminal state.
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusCompleted || s == TaskStatusFailed
}

// TextToImageRequest is the standard input for text-to-image generation.
// Compatible with OpenAI /v1/images/generations.
type TextToImageRequest struct {
	Prompt         string            `json:"prompt"`
	N              int               `json:"n,omitempty"`           // images count, default 1
	Size           string            `json:"size,omitempty"`        // "1024x1024"
	Quality        string            `json:"quality,omitempty"`     // low / medium / high
	ResponseFormat string            `json:"response_format,omitempty"` // "url" or "b64_json"
	Model          string            `json:"model,omitempty"`
	Extra          map[string]any    `json:"extra,omitempty"`       // provider-specific passthrough
}

// ImageToImageRequest is the standard input for image-to-image generation
// (edit, inpainting, variation).
type ImageToImageRequest struct {
	Prompt         string            `json:"prompt"`
	Image          string            `json:"image"`                 // source image (URL or base64)
	Mask           string            `json:"mask,omitempty"`        // inpaint region
	Strength       float64           `json:"strength,omitempty"`    // 0-1, transformation degree
	N              int               `json:"n,omitempty"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Model          string            `json:"model,omitempty"`
	Extra          map[string]any    `json:"extra,omitempty"`
}

// ImageResult represents a single generated image.
type ImageResult struct {
	Index         int    `json:"index"`
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	Seed          int64  `json:"seed,omitempty"`
	ContentType   string `json:"content_type,omitempty"` // "image/png", "image/webp"
}

// ImageTask wraps the result of an image generation request.
// Sync providers return with Status=completed; async providers return with Status=pending.
type ImageTask struct {
	ID        string         `json:"id,omitempty"`
	Status    TaskStatus     `json:"status"`
	Images    []ImageResult  `json:"images,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt int64          `json:"created_at,omitempty"`
}
