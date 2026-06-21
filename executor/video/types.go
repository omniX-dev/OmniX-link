// Package video defines video generation types and executor interface for Omni-link.
//
// All video providers are async — no sync generation exists.
// Standard types defined here, providers adapt to/from their native format internally.
package video

// VideoTaskStatus represents the status of a video generation task.
type VideoTaskStatus string

const (
	VideoTaskQueued     VideoTaskStatus = "queued"
	VideoTaskProcessing VideoTaskStatus = "processing"
	VideoTaskCompleted  VideoTaskStatus = "completed"
	VideoTaskFailed     VideoTaskStatus = "failed"
)

// IsTerminal returns true if the task has reached a terminal state.
func (s VideoTaskStatus) IsTerminal() bool {
	return s == VideoTaskCompleted || s == VideoTaskFailed
}

// TextToVideoRequest generates video from a text prompt.
type TextToVideoRequest struct {
	Prompt   string         `json:"prompt"`
	Model    string         `json:"model,omitempty"`
	Size     string         `json:"size,omitempty"`     // "1920x1080"
	Duration int            `json:"duration,omitempty"` // seconds
	Quality  string         `json:"quality,omitempty"`  // "standard", "pro"
	Extra    map[string]any `json:"extra,omitempty"`
}

// ImageToVideoRequest animates/generates video from one or more images.
type ImageToVideoRequest struct {
	Prompt   string         `json:"prompt"`
	Model    string         `json:"model,omitempty"`
	Image    string         `json:"image"`              // URL or base64
	ImageEnd string         `json:"image_end,omitempty"` // last frame (Wan, Kling)
	Size     string         `json:"size,omitempty"`
	Duration int            `json:"duration,omitempty"`
	Quality  string         `json:"quality,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// VideoToVideoRequest modifies an existing video with a prompt.
type VideoToVideoRequest struct {
	Prompt   string         `json:"prompt"`
	Model    string         `json:"model,omitempty"`
	Video    string         `json:"video"`                // source video URL
	Strength float64        `json:"strength,omitempty"`   // 0-1
	Size     string         `json:"size,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// ExtendVideoRequest continues an existing video.
type ExtendVideoRequest struct {
	VideoID  string         `json:"video_id"`
	Duration int            `json:"duration,omitempty"`   // additional seconds
	Prompt   string         `json:"prompt,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// EditVideoRequest applies instruction-based edits to a video.
type EditVideoRequest struct {
	Video        string         `json:"video"`         // source video URL
	Instructions string         `json:"instructions"`  // "change background to..."
	Model        string         `json:"model,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// CharacterRequest registers a character from a reference clip (Sora-specific).
type CharacterRequest struct {
	Name  string         `json:"name"`
	Video string         `json:"video"`          // reference clip URL
	Model string         `json:"model,omitempty"`
	Extra map[string]any `json:"extra,omitempty"`
}

// Character represents a registered character for consistent generation.
type Character struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

// VideoTask wraps the result of a video generation operation.
// All video providers are async — task starts as queued/processing,
// client polls via GetTask until terminal.
type VideoTask struct {
	ID           string          `json:"id"`
	Status       VideoTaskStatus `json:"status"`
	VideoURL     string          `json:"video_url,omitempty"`     // expires ~24h
	ThumbnailURL string          `json:"thumbnail_url,omitempty"`
	Duration     float64         `json:"duration,omitempty"`
	Size         string          `json:"size,omitempty"`         // "1920x1080"
	Error        string          `json:"error,omitempty"`
	CreatedAt    int64           `json:"created_at"`
}
