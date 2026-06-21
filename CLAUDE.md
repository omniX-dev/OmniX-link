# Omni-link — AI API Protocol Translation Layer

Go module bridging AI API protocol formats (OpenAI Chat, Claude Messages, OpenAI Responses). Client using format A can transparently call provider of format B. Gemini format used internally by Gemini executor only.

## Architecture

```
model/       → Provider types, channel config, protocol metadata
translator/  → Format conversion engine + protocol type definitions
executor/    → Modality-specific executors with plugin registry
executor/text/    → Chat/completion executors (OpenAI, Claude, Gemini, etc.)
executor/image/   → Image generation executors (GPT Image, Midjourney, etc.)
executor/audio/   → Audio/Speech executors (TTS, STT, Music)
executor/video/   → Video generation executors (Sora, Kling, etc.)
```

### model — Type definitions

- `ProviderType`: 40+ provider enum (OpenAI=1, Claude=2, Gemini=3, DeepSeek=8, ...)
- `ProtocolType`: upstream API protocol ("openai-compatible", "anthropic-compatible", ...)
- `Channel`: llm_channels row with protocols, API key, settings
- `ResolveProtocol(ProviderType)`: maps provider → default protocol

### translator — Format conversion

Core conversion engine. 3 client-exposed formats + 1 internal format:

| Format | Endpoint | Notes |
|--------|----------|-------|
| `openai` | `/v1/chat/completions` | Client interface |
| `claude` | `/v1/messages` | Client interface |
| `openai_responses` | `/v1/responses` | Client interface |
| `gemini` | (Google endpoint) | **Executor-internal only** — not exposed as client interface |

**Conversion matrix** (✓ = direct, · = hub/OpenAI intermediate, — = same format):

| from ↓ → to | openai | claude | responses | gemini |
|-------------|--------|--------|-----------|--------|
| **openai** | — | ✓ | ✓ | ✓ ¹ |
| **claude** | ✓ | — | ✓ | ✓ ¹ |
| **responses** | ✓ | ✓ | — | ✓ ¹ |
| **gemini** ² | ✓ ¹ | ✓ ¹ | ✓ ¹ | — |

¹ Gemini conversion used internally by Gemini executor only.  
² Gemini format not exposed as client interface. Only convertible to/from via hub fallback.

**Key functions:**
- `Convert(body, from, to)` — entry point. Falls back via OpenAI intermediate when direct path missing
- `DetectFormat(body, path)` — format detection (path first, then body inspection)
- `DetectFormatFromPath(path)` — URL path → format lookup
- `DetectFormat(body)` — heuristics: has "input"? → Responses. Has "messages" + "max_tokens"? → Claude. Has "temperature"? → OpenAI. Default → OpenAI

All protocol type definitions live in `translator/`:
- `openai.go` — `ChatRequest`, `Message`, `Tool`, `ChatResponse`, `ChatStreamChunk`
- `claude.go` — `ClaudeRequest`, `ClaudeMessage`, `ClaudeResponse`, SSE event constants
- `responses.go` — `ResponsesRequest`, `InputItem`, `ResponsesOutput`, `ResponsesResponse`
- `gemini.go` — `GeminiChatRequest`, `GeminiContent`, `GeminiPart`, `GeminiChatResponse`, `GeminiThinkingConfig`

### executor — Modality-Specific Provider Execution

Each modality has its own plugin registry and interface under `executor/<modality>/`:

| Modality | Interface | Registry | Standard Format |
|---|---|---|---|
| **Text** | `executor/text.Executor` | `Register(provider, exec)` | OpenAI Chat, Claude, Responses |
| **Image** | `executor/image.ImageExecutor` | `RegisterImage(provider, exec)` | OpenAI `/v1/images/generations` |
| **Audio** | `executor/audio.AudioExecutor` | `RegisterAudio(provider, exec)` | OpenAI `/v1/audio/speech` + `/v1/audio/transcriptions` |
| **Video** | `executor/video.VideoExecutor` | `RegisterVideo(provider, exec)` | Custom (no universal standard yet) |

**Text Executor** — same interface as before, now at `executor/text`:

```go
import "github.com/just4zeroq/Omni-link/executor/text"
func init() { text.Register("claude", &ClaudeExecutor{}) }
```

**Image/Audio/Video executors** follow same pattern with modality-specific methods:
- `ImageExecutor` — `TextToImage`, `ImageToImage`, `GetTask`
- `AudioExecutor` — `TextToSpeech`, `SpeechToText`, `MusicGenerate`, `GetTask`, `ListVoices`
- `VideoExecutor` — `TextToVideo`, `ImageToVideo`, `VideoToVideo`, `ExtendVideo`, `EditVideo`, `GetTask`

**Implemented text executors:**
- `claude` — native Claude, SSE streaming Claude↔OpenAI
- `openai` — native OpenAI Chat, includes Responses↔OpenAI
- `gemini` — native Gemini, converts via OpenAI intermediate on request/response
- `deepseek` — dual native (OpenAI + Claude), custom thinking/reasoning injection
- `volcengine` — dual native (OpenAI Chat + OpenAI Responses), SSE passthrough

**Format planning:**
`Plan(input, output, capabilities)` selects optimal upstream format minimizing conversions (score = input mismatch + output mismatch). Prefers format matching output format on tie.

**SSE streaming architecture:**
Streaming conversion uses `ResponseStream` interface with `Feed(chunk)` / `End()` / `Usage()` methods. Implemented for Claude ↔ OpenAI (both directions) via stateful stream converters:
- `claudeToOpenAIStream` — maps Claude SSE events (message_start, content_block_start/delta/stop, message_delta/stop) → OpenAI `data:` chunks
- `openAIToClaudeStream` — maps OpenAI `data:` chunks → Claude SSE events

Both buffer incomplete events, handle tool calls, track usage accumulation.

## Conventions

**Code style:**
- Pointer-heavy for optional fields (`*int`, `*string`, `*float64`) — zero-value = unset
- `json.RawMessage` for passthrough/raw fields (tools, tool_choice)
- Channel typed as `any` for abstraction (never `*model.Channel` directly)

**Conversion architecture (SINGLE SOURCE OF TRUTH):**
- All format conversion logic lives in `translator/conv.go` — canonical, unprefixed
- All executors delegate ConvertRequest/ConvertResponse to `translator.Convert(body, from, to)`
- No conversion code duplication across executor files
- Vendor-specific modifications go in `RequestCustomize`/`ResponseCustomize` hooks (e.g. `dsInjectThinking`, `injectStreamOptionsOpenAI`, `replaceModelField`)

**Error patterns:**
- All conversions return `fmt.Errorf("provider: direction: %w", err)` — wraps with direction context
- Unsupported format pairs return explicit error, never silent fallthrough
- SSE parsing skips malformed events (continue), never fails the entire stream

**Import paths:**
`github.com/just4zeroq/Omni-link/translator`
`github.com/just4zeroq/Omni-link/executor/text` — text executor
`github.com/just4zeroq/Omni-link/executor/image` — image executor
`github.com/just4zeroq/Omni-link/executor/audio` — audio executor
`github.com/just4zeroq/Omni-link/executor/video` — video executor

## Testing

**Unit tests** (`translator/conv_test.go`):
- 37 test cases covering format detection, all conversion pairs, round-trip
- Run: `go test ./translator/`

**Integration tests** (`executor/text/deepseek/` + `executor/text/volcengine/`):
Requires valid API keys in `.env`. DeepSeek tests cover:
- OpenAI-compatible endpoint (`/v1/chat/completions`)
- Anthropic-compatible endpoint (`/anthropic/v1/messages`)
- Format conversion (OpenAI↔Claude round-trip)
- Full executor pipeline, streaming, tools, thinking, error handling
- Run: `go test ./executor/text/deepseek/ -timeout 120s`

Volcengine (Doubao/火山引擎) tests cover:
- OpenAI Chat + Responses API endpoints
- Streaming (both Chat + Responses SSE passthrough)
- Format conversion (Responses↔Chat via Plan)
- System message, tools, params, error handling
- Run: `go test ./executor/text/volcengine/ -timeout 120s`

**DeepSeek API**:
- OpenAI format: `https://api.deepseek.com/v1/chat/completions` (auth: `Authorization: Bearer`)
- Claude format: `https://api.deepseek.com/anthropic/v1/messages` (auth: `x-api-key`)
- `UpstreamFormat` controls auth header and URL path selection
- Notable: `deepseek-chat` model resolves to `deepseek-v4-flash` upstream

## Common operations

- **Add new text executor**: create `executor/text/<name>.go` with `init()` Registration, implement `Executor` interface, add vendor-specific hooks
- **Add new image executor**: create `executor/image/<name>.go` with `init()` RegisterImage, implement `ImageExecutor` interface
- **Add new audio executor**: create `executor/audio/<name>.go` with `init()` RegisterAudio, implement `AudioExecutor` interface
- **Add new video executor**: create `executor/video/<name>.go` with `init()` RegisterVideo, implement `VideoExecutor` interface
- **Add new format**: define types in new `translator/<name>.go`, add `Format` constant, implement `convertDirect` cases in `conv.go`
- **Add new channel mapping**: add `ProviderType` constant in `model/model.go`, add `ResolveProtocol` case
