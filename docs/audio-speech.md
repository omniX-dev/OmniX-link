# Audio / Speech — Design

## Overview

Audio modality for Omni-link, covering TTS (text-to-speech), STT (speech-to-text), and music generation. Same pattern as image: unified interface, provider adaptation, stateless library.

## Categories

| Type | Description | Sync/Async |
|---|---|---|
| **TTS** | Text → Audio stream/file | Sync (most providers) |
| **STT** | Audio → Text transcript | Sync (optional streaming) |
| **Music** | Prompt → Music track | ⏳ Async (task-based) |
| **Voice Clone** | Samples → Custom voice profile | Special (setup step for TTS) |

## Architecture

```
executor/
├── text/                  # Text (existing, migrated)
├── image/                 # Image
├── audio/                 # Audio (new)
│   ├── types.go           # TTSRequest, STTRequest, AudioResult, AudioTask
│   ├── executor.go        # AudioExecutor interface
│   ├── registry.go        RegisterAudio()
│   ├── openai/            OpenAI TTS + Whisper STT
│   ├── elevenlabs/        ElevenLabs TTS + Voice Clone
│   ├── suno/              Suno music generation (async)
│   ├── azure/             Azure Speech (TTS + STT)
│   ├── playht/            PlayHT TTS
│   ├── cartesia/          Cartesia TTS (ultra-low latency)
│   ├── fishaudio/         Fish Audio TTS + Voice Clone (open source)
│   ├── cosyvoice/         CosyVoice TTS + Voice Design (Alibaba)
│   └── funasr/            FunASR STT (Alibaba, open source)
└── video/                 # Video (TBD)
```

## Principle: Unified Interface, Provider Adaptation

One set of standard types. Each executor adapts to its provider's native format.

Standard format = **OpenAI Audio compatible**:
- TTS: `POST /v1/audio/speech` format
- STT: `POST /v1/audio/transcriptions` format

## AudioExecutor Interface

```go
type AudioExecutor interface {
    Init(channel any)
    GetName() string

    // — TTS: Text-to-Speech —

    // TextToSpeech converts text to audio.
    // Returns audio bytes directly (sync), or pending AudioTask (async providers).
    // Standard format = OpenAI /v1/audio/speech compatible.
    TextToSpeech(req *TTSRequest) (*AudioResult, error)

    // — STT: Speech-to-Text (optional; may return ErrNotSupported) —

    // SpeechToText transcribes audio to text.
    // Accepts audio bytes + metadata, returns transcript.
    // Standard format = OpenAI /v1/audio/transcriptions compatible.
    SpeechToText(req *STTRequest) (*STTResult, error)

    // — Music Generation (optional; may return ErrNotSupported) —

    // MusicGenerate creates music from a text prompt.
    // Async pattern: returns pending task, client polls GetTask.
    MusicGenerate(req *MusicRequest) (*AudioTask, error)

    // — Async Task Polling —

    // GetTask queries async task status (Suno etc.).
    // Each call proxies directly to upstream.
    GetTask(taskID string) (*AudioTask, error)

    // — Voice (optional; may return ErrNotSupported) —

    // ListVoices returns available voices for TTS.
    ListVoices() ([]Voice, error)
}
```

## Data Types

### TTSRequest (Standard: OpenAI /v1/audio/speech)

```go
type TTSRequest struct {
    Model           string            `json:"model,omitempty"`       // e.g. "gpt-4o-mini-tts", "eleven_v3"
    Input           string            `json:"input"`                 // text to speak
    Voice           string            `json:"voice,omitempty"`       // voice ID or name
    Instructions    string            `json:"instructions,omitempty"`// tone, emotion, accent (OpenAI)
    ResponseFormat  string            `json:"response_format,omitempty"` // "mp3", "wav", "opus", "aac", "flac", "pcm"
    Speed           float64           `json:"speed,omitempty"`       // 0.25-4.0, default 1.0

    // Provider-specific
    Extra           map[string]any    `json:"extra,omitempty"`
}
```

### STTRequest (Standard: OpenAI /v1/audio/transcriptions)

```go
type STTRequest struct {
    Model           string            `json:"model,omitempty"`       // "whisper-1", "gpt-4o-transcribe"
    File            []byte            `json:"-"`                     // audio file content
    FileName        string            `json:"-"`                     // "audio.mp3"
    Language        string            `json:"language,omitempty"`    // ISO-639-1 (e.g. "zh")
    Prompt          string            `json:"prompt,omitempty"`      // optional context
    ResponseFormat  string            `json:"response_format,omitempty"` // "json", "verbose_json", "srt", "vtt"
    Temperature     float64           `json:"temperature,omitempty"` // 0-1
    TimestampLevels []string          `json:"timestamp_granularities,omitempty"` // ["word"], ["segment"]

    Extra           map[string]any    `json:"extra,omitempty"`
}
```

### MusicRequest

```go
type MusicRequest struct {
    Model           string            `json:"model,omitempty"`       // "suno-v5", "chirp-v5"
    Prompt          string            `json:"prompt"`                // description or lyrics
    Title           string            `json:"title,omitempty"`       // song title (custom mode)
    Tags            string            `json:"tags,omitempty"`        // genre/style tags
    Instrumental    bool              `json:"instrumental,omitempty"`
    Duration        int               `json:"duration,omitempty"`    // desired duration (seconds)
    CallbackURL     string            `json:"callback_url,omitempty"`
    Extra           map[string]any    `json:"extra,omitempty"`
}
```

### AudioResult

```go
type AudioResult struct {
    Audio           []byte            `json:"-"`                     // raw audio bytes
    ContentType     string            `json:"content_type,omitempty"`// "audio/mpeg", "audio/wav"
    Duration        float64           `json:"duration,omitempty"`    // seconds
    Format          string            `json:"format,omitempty"`      // "mp3", "wav", "opus"
}
```

### STTResult

```go
type STTResult struct {
    Text            string            `json:"text"`
    Language        string            `json:"language,omitempty"`
    Duration        float64           `json:"duration,omitempty"`
    Segments        []STTSegment      `json:"segments,omitempty"`    // for verbose_json
    Words           []STTWord         `json:"words,omitempty"`       // word-level timestamps
}

type STTSegment struct {
    Start float64 `json:"start"`
    End   float64 `json:"end"`
    Text  string  `json:"text"`
}

type STTWord struct {
    Word  string  `json:"word"`
    Start float64 `json:"start"`
    End   float64 `json:"end"`
}
```

### AudioTask (async — Suno)

```go
type AudioTaskStatus string

const (
    AudioTaskPending    AudioTaskStatus = "pending"
    AudioTaskProcessing AudioTaskStatus = "processing"
    AudioTaskCompleted  AudioTaskStatus = "completed"
    AudioTaskFailed     AudioTaskStatus = "failed"
)

type AudioTask struct {
    ID              string           `json:"id"`
    Status          AudioTaskStatus  `json:"status"`
    Title           string           `json:"title,omitempty"`
    AudioURL        string           `json:"audio_url,omitempty"`   // expires ~24h
    Lyric           string           `json:"lyric,omitempty"`
    Duration        float64          `json:"duration,omitempty"`
    Error           string           `json:"error,omitempty"`
    CreatedAt       int64            `json:"created_at"`
}
```

### Voice

```go
type Voice struct {
    ID              string            `json:"id"`
    Name            string            `json:"name"`
    PreviewURL      string            `json:"preview_url,omitempty"`
    Labels          map[string]string `json:"labels,omitempty"`     // gender, accent, etc.
    Category        string            `json:"category,omitempty"`   // "premade", "cloned", "professional"
}
```

## Provider Support

### TTS

| Executor | TextToSpeech | Voices | Sync/Async | Notes |
|---|---|---|---|---|
| OpenAI TTS | ✅ | 13 built-in | Sync | Native OpenAI `/v1/audio/speech` |
| ElevenLabs | ✅ | 100+ premade + cloned | Sync | Ultra-low-latency flash models |
| Azure Speech | ✅ | 400+ | Sync + streaming | SSML support |
| PlayHT | ✅ | 2000+ (incl. cloned) | Sync + async | Play3.0-mini, PlayDialog |
| Cartesia | ✅ | 80+ (Sonic-3) | Sync + SSE + WS | ~90ms first-byte latency |
| Fish Audio | ✅ | Custom (clone supported) | Sync | Open source S2 model, zero-shot clone |
| CosyVoice | ✅ | Custom (design + clone) | Sync + streaming | Alibaba, text-to-voice design |

### STT

| Executor | SpeechToText | Models | Sync/Async | Notes |
|---|---|---|---|---|
| OpenAI Whisper | ✅ | whisper-1, gpt-4o-transcribe | Sync | Native OpenAI `/v1/audio/transcriptions` |
| Azure Speech | ✅ | Customizable | Sync + streaming | |
| FunASR | ✅ | fun-asr, fun-asr-mtl | Async + streaming | **OpenAI compatible** (self-hosted `v1/audio/transcriptions`) |

### Music

| Executor | MusicGenerate | GetTask | Async | Notes |
|---|---|---|---|---|
| Suno | ✅ | ✅ | ⏳ Submit → poll | Third-party relays, no official public API |

### Voice Clone

| Executor | Feature |
|---|---|
| ElevenLabs | Clone from 10-30s audio samples |
| Fish Audio | Zero-shot voice clone via reference audio |
| CosyVoice | Voice design from text description + clone from audio |

## Async Polling

Same pattern as image: **client-driven, stateless**.

```go
// TTS — sync, returns audio bytes
result, _ := openai.TextToSpeech(&TTSRequest{
    Input:  "Hello world",
    Voice:  "coral",
    Format: "mp3",
})
// result.Audio = raw mp3 bytes

// Music — async, client polls
task, _ := suno.MusicGenerate(&MusicRequest{
    Prompt: "Upbeat pop song about summer",
})
// task.Status == pending

for !task.Status.IsTerminal() {
    time.Sleep(5 * time.Second)
    task, _ = suno.GetTask(task.ID)
}
```

## Provider Implementations

### OpenAI
- Native OpenAI `/v1/audio/speech` (TTS) and `/v1/audio/transcriptions` (STT)
- Auth: `Authorization: Bearer`
- TTS models: `gpt-4o-mini-tts`, `tts-1`, `tts-1-hd`
- STT models: `whisper-1`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`

### ElevenLabs
- Custom REST `POST /v1/text-to-speech/{voice_id}`
- Auth: `xi-api-key`
- Converts standard TTSRequest → ElevenLabs native
- Voice cloning via separate endpoint
- Ultra-low-latency flash models available

### Suno
- No official public API — third-party relays (EvoLink, AceDataCloud, etc.)
- Async: MusicGenerate → task ID → GetTask → completed
- Models: `suno-v5`, `chirp-v5`, `suno-v4`, `chirp-v4`

### Azure Speech
- Custom REST endpoint per region
- SSML support for fine-grained prosody control
- 400+ multilingual voices

### PlayHT
- Custom REST `POST /v2/tts` (async job) + `POST /v2/tts/stream` (sync)
- Auth: `Authorization: Bearer` + `X-USER-ID`
- Voice engines: PlayHT2.0, Play3.0-mini, PlayDialog
- Converts standard TTSRequest → PlayHT native format
- Voice cloning: instant clone + professional studio

### Cartesia
- Custom REST `POST /tts/bytes` (sync) + `POST /tts/sse` (SSE streaming) + WebSocket
- Auth: `Authorization: Bearer`
- Model: `sonic-3` — ~90ms first-byte latency
- 42 languages, 80+ voices
- Good fit for real-time voice agent use cases

### Fish Audio
- Custom REST `POST /v1/tts`
- Auth: `Authorization: Bearer`
- Open source S2 model (Qwen3-4B backbone)
- Zero-shot voice cloning via reference audio
- 80+ languages
- Also self-hostable (fish-speech open source)

### CosyVoice (Alibaba)
- Custom REST `POST /api/v1/services/audio/tts/SpeechSynthesizer`
- Auth: `Authorization: Bearer` (DashScope API key)
- Models: `cosyvoice-v3.5-plus`, `cosyvoice-v3.5-flash`
- Voice cloning + voice design from text description
- Separate voice enrollment API to create custom voices
- WebSocket streaming via `dashscope.aliyuncs.com`

### FunASR (Alibaba)
- Cloud API: async `POST /api/v1/services/audio/asr/transcription` (poll task result)
- Self-hosted: **OpenAI compatible** `POST /v1/audio/transcriptions` ✅
- Auth: `Authorization: Bearer`
- Open source, 30+ languages, speaker diarization
- Also WebSocket for real-time streaming
- Since self-hosted mode supports OpenAI format, can reuse openai executor

## Implementation Plan

### Phase 1 — Foundation
1. `executor/audio/types.go` — TTSRequest, STTRequest, MusicRequest, AudioResult, STTResult, AudioTask, Voice
2. `executor/audio/executor.go` — AudioExecutor interface
3. `executor/audio/registry.go`

### Phase 2 — TTS
4. OpenAI TTS executor (`executor/audio/openai/tts.go`)
5. ElevenLabs TTS executor
6. Tests: TTS request/response, voice list, format selection

### Phase 3 — STT
7. OpenAI Whisper executor (`executor/audio/openai/stt.go`)
8. Tests: transcription, language detection, verbose_json, srt

### Phase 4 — Music
9. Suno executor with task polling
10. Tests: submit → poll → completion

### Phase 5 — Voice Clone
11. ElevenLabs voice clone
12. ListVoices across all TTS providers
