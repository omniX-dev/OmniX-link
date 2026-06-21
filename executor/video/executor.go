package video

import "fmt"

// ErrNotSupported is returned when an executor doesn't support a specific operation.
var ErrNotSupported = fmt.Errorf("operation not supported")

// VideoExecutor is the interface for video generation providers.
//
// All video providers are async — generators return a pending task,
// and the client polls via GetTask until the task reaches a terminal state.
//
// Standard format: no universal standard exists yet for video APIs,
// so each provider adapts to/from our standard types internally.
type VideoExecutor interface {
	// Init initializes the executor with channel configuration.
	Init(channel any) // *model.Channel

	// GetName returns the human-readable executor name.
	GetName() string

	// TextToVideo generates video from a text prompt.
	TextToVideo(req *TextToVideoRequest) (*VideoTask, error)

	// ImageToVideo animates/generates video from one or more images.
	// Returns ErrNotSupported if the provider lacks this capability.
	ImageToVideo(req *ImageToVideoRequest) (*VideoTask, error)

	// VideoToVideo modifies an existing video with a prompt.
	// Returns ErrNotSupported if the provider lacks this capability.
	VideoToVideo(req *VideoToVideoRequest) (*VideoTask, error)

	// ExtendVideo continues an existing video.
	// Returns ErrNotSupported if the provider lacks this capability.
	ExtendVideo(req *ExtendVideoRequest) (*VideoTask, error)

	// EditVideo applies instruction-based edits to a video.
	// Returns ErrNotSupported if the provider lacks this capability.
	EditVideo(req *EditVideoRequest) (*VideoTask, error)

	// CreateCharacter registers a character from a reference clip (Sora-specific).
	// Returns ErrNotSupported if the provider lacks this capability.
	CreateCharacter(req *CharacterRequest) (*Character, error)

	// GetTask queries the status of an async video task.
	// Each call proxies directly to the upstream provider.
	GetTask(taskID string) (*VideoTask, error)
}
