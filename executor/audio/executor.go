package audio

import "fmt"

// ErrNotSupported is returned when an executor doesn't support a specific operation.
var ErrNotSupported = fmt.Errorf("operation not supported")

// AudioExecutor is the interface for audio/speech providers.
//
// Standard formats:
//   - TTS: OpenAI /v1/audio/speech compatible
//   - STT: OpenAI /v1/audio/transcriptions compatible
//
// TextToSpeech returns *AudioStream — unified for sync and streaming providers.
// Sync provider: stream has one chunk then closes.
// Streaming provider: push chunks as they arrive, close when done.
// Call Collect() on the stream to get a single AudioResult if needed.
type AudioExecutor interface {
	// Init initializes the executor with channel configuration.
	Init(channel any) // *model.Channel

	// GetName returns the human-readable executor name.
	GetName() string

	// TextToSpeech converts text to audio.
	// Returns AudioStream — for-range over Chunk, or call Collect().
	// Standard format = OpenAI /v1/audio/speech compatible.
	TextToSpeech(req *TTSRequest) (*AudioStream, error)

	// SpeechToText transcribes audio to text.
	// Returns ErrNotSupported if the provider lacks this capability.
	// Standard format = OpenAI /v1/audio/transcriptions compatible.
	SpeechToText(req *STTRequest) (*STTResult, error)

	// MusicGenerate creates music from a text prompt.
	// Async pattern: returns pending task, client polls GetTask.
	// Returns ErrNotSupported if the provider lacks this capability.
	MusicGenerate(req *MusicRequest) (*AudioTask, error)

	// GetTask queries the status of an async audio task.
	// Each call proxies directly to the upstream provider.
	GetTask(taskID string) (*AudioTask, error)

	// ListVoices returns available voices for TTS.
	// Returns ErrNotSupported if the provider doesn't expose voice listing.
	ListVoices() ([]Voice, error)
}
