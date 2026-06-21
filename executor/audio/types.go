// Package audio defines audio/speech types and executor interface for Omni-link.
//
// Standard formats:
//   - TTS: OpenAI /v1/audio/speech compatible
//   - STT: OpenAI /v1/audio/transcriptions compatible
package audio

// AudioTaskStatus represents the status of an async audio task (e.g. music generation).
type AudioTaskStatus string

const (
	AudioTaskPending    AudioTaskStatus = "pending"
	AudioTaskProcessing AudioTaskStatus = "processing"
	AudioTaskCompleted  AudioTaskStatus = "completed"
	AudioTaskFailed     AudioTaskStatus = "failed"
)

// IsTerminal returns true if the task has reached a terminal state.
func (s AudioTaskStatus) IsTerminal() bool {
	return s == AudioTaskCompleted || s == AudioTaskFailed
}

// TTSRequest is the standard input for text-to-speech.
// Compatible with OpenAI /v1/audio/speech.
type TTSRequest struct {
	Model          string            `json:"model,omitempty"`        // e.g. "gpt-4o-mini-tts", "eleven_v3"
	Input          string            `json:"input"`                  // text to speak
	Voice          string            `json:"voice,omitempty"`        // voice ID or name
	Instructions   string            `json:"instructions,omitempty"` // tone, emotion, accent (OpenAI)
	ResponseFormat string            `json:"response_format,omitempty"` // "mp3", "wav", "opus", "aac", "flac", "pcm"
	Speed          float64           `json:"speed,omitempty"`        // 0.25-4.0, default 1.0
	Extra          map[string]any    `json:"extra,omitempty"`        // provider-specific passthrough
}

// STTRequest is the standard input for speech-to-text.
// Compatible with OpenAI /v1/audio/transcriptions.
type STTRequest struct {
	Model           string            `json:"model,omitempty"`            // "whisper-1", "gpt-4o-transcribe"
	File            []byte            `json:"-"`                          // audio file content
	FileName        string            `json:"-"`                          // "audio.mp3"
	Language        string            `json:"language,omitempty"`         // ISO-639-1 (e.g. "zh")
	Prompt          string            `json:"prompt,omitempty"`           // optional context
	ResponseFormat  string            `json:"response_format,omitempty"`  // "json", "verbose_json", "srt", "vtt"
	Temperature     float64           `json:"temperature,omitempty"`      // 0-1
	TimestampLevels []string          `json:"timestamp_granularities,omitempty"` // ["word"], ["segment"]
	Extra           map[string]any    `json:"extra,omitempty"`
}

// MusicRequest is the input for music generation (async).
type MusicRequest struct {
	Model       string         `json:"model,omitempty"`       // "suno-v5", "chirp-v5"
	Prompt      string         `json:"prompt"`                // description or lyrics
	Title       string         `json:"title,omitempty"`       // song title (custom mode)
	Tags        string         `json:"tags,omitempty"`        // genre/style tags
	Instrumental bool          `json:"instrumental,omitempty"`
	Duration    int            `json:"duration,omitempty"`    // desired duration (seconds)
	CallbackURL string         `json:"callback_url,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

// AudioResult holds the result of a sync TTS call.
type AudioResult struct {
	Audio       []byte  `json:"-"`                    // raw audio bytes
	ContentType string  `json:"content_type,omitempty"` // "audio/mpeg", "audio/wav"
	Duration    float64 `json:"duration,omitempty"`    // seconds
	Format      string  `json:"format,omitempty"`      // "mp3", "wav", "opus"
}

// AudioChunk is a single piece of audio from a streaming TTS call.
type AudioChunk struct {
	Data []byte
}

// AudioStream is the unified streaming result from TextToSpeech.
//
// Sync providers push one chunk then close the channel.
// Streaming providers push chunks as they arrive from upstream.
//
// Usage:
//
//	stream, _ := tts.TextToSpeech(req)
//	for chunk := range stream.Chunk {
//	    process(chunk.Data)
//	}
//
//	// Or collect into a single AudioResult:
//	result, _ := stream.Collect()
type AudioStream struct {
	Chunk       <-chan AudioChunk
	ContentType string // "audio/mpeg"
	Format      string // "mp3", "wav"
}

// Collect drains the AudioStream into a single AudioResult.
// Blocks until the stream is exhausted.
func (s *AudioStream) Collect() (*AudioResult, error) {
	var buf []byte
	for chunk := range s.Chunk {
		buf = append(buf, chunk.Data...)
	}
	return &AudioResult{
		Audio:       buf,
		ContentType: s.ContentType,
		Format:      s.Format,
	}, nil
}

// NewStreamFromResult wraps an AudioResult as a single-chunk AudioStream.
func NewStreamFromResult(r *AudioResult) *AudioStream {
	ch := make(chan AudioChunk, 1)
	ch <- AudioChunk{Data: r.Audio}
	close(ch)
	return &AudioStream{
		Chunk:       ch,
		ContentType: r.ContentType,
		Format:      r.Format,
	}
}

// STTResult holds the result of a speech-to-text call.
type STTResult struct {
	Text     string       `json:"text"`
	Language string       `json:"language,omitempty"`
	Duration float64      `json:"duration,omitempty"`
	Segments []STTSegment `json:"segments,omitempty"`
	Words    []STTWord    `json:"words,omitempty"`
}

// STTSegment represents a segment-level transcription result.
type STTSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// STTWord represents a word-level timestamp.
type STTWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// AudioTask wraps an async audio task (e.g. Suno music generation).
type AudioTask struct {
	ID        string          `json:"id"`
	Status    AudioTaskStatus `json:"status"`
	Title     string          `json:"title,omitempty"`
	AudioURL  string          `json:"audio_url,omitempty"`  // expires ~24h
	Lyric     string          `json:"lyric,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	Error     string          `json:"error,omitempty"`
	CreatedAt int64           `json:"created_at"`
}

// Voice describes a TTS voice option.
type Voice struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	PreviewURL string            `json:"preview_url,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`    // gender, accent, etc.
	Category   string            `json:"category,omitempty"`  // "premade", "cloned", "professional"
}
