# Omni-link

**Universal AI Protocol Translation Library** — Go library bridging AI API formats across text, image, audio, video, and embedding.

```go
import "github.com/just4zeroq/Omni-link/client"
import "github.com/just4zeroq/Omni-link/model"

// Unified client — one object for all modalities
c := client.NewClient(&model.Channel{
    ProviderType: model.ProviderOpenAI,
    ApiKey:       "sk-...",
})

// Text chat — OpenAI format body, auto-converts
resp, _ := c.Chat(ctx, []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`))

// Image, TTS, STT, Video, Embedding — all via the same client
images, _ := c.Image(ctx, &image.TextToImageRequest{Prompt: "a cat"})
stream, _ := c.Speak(ctx, &audio.TTSRequest{Input: "Hello"})
task, _ := c.Video(ctx, &video.TextToVideoRequest{Prompt: "rocket launch"})
emb, _ := c.Embed(ctx, &embedding.EmbeddingRequest{Model: "text-embedding-v3", Input: "Hello"})
```

[![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-000000?style=flat-square)](LICENSE)
[![Zero Deps](https://img.shields.io/badge/Dependencies-Zero-6366f1?style=flat-square)](go.mod)

> **Status**: Text protocol translation ✅ | Image providers ✅ | Audio providers ✅ | Video providers ✅ | Embedding ✅

---

## Modality Roadmap

| Category | Status | Providers |
|---|---|---|
| **🔤 Text** | ✅ Complete | OpenAI, Anthropic, Google, DeepSeek, Volcengine, Zhipu, Moonshot, MiniMax, Xiaomi, Kunlun, Stepfun + 35+ more via protocol translation |
| **🖼️ Image** | ✅ Complete | OpenAI, Midjourney, Alibaba (Qwen/Wan), Zhipu, Stepfun, NanoBanana, ZImage, Volcengine (Seedream), Fal (Seedream) |
| **🎵 Audio** | ✅ Complete | OpenAI, Azure, Alibaba (CosyVoice/FunASR), ElevenLabs, PlayHT, Cartesia, FishAudio, Stepfun, Suno |
| **🎬 Video** | ✅ Complete | OpenAI (Sora), Alibaba (Wan/HappyHorse), Kuaishou, Runway, Seedance, Hailuo, Pika, Luma, xAI, OmniHuman, Stepfun |
| **📊 Embedding** | ✅ Complete | OpenAI, Zhipu, Alibaba, Jina |

---

## Quick Start

```bash
go get github.com/just4zeroq/Omni-link
```

Executor registration follows `database/sql` pattern — blank-import only what you need:

```go
package main

import (
    "github.com/just4zeroq/Omni-link/client"
    "github.com/just4zeroq/Omni-link/model"
    _ "github.com/just4zeroq/Omni-link/executor/text/openai"   // only these compiled
    _ "github.com/just4zeroq/Omni-link/executor/image/openai"
    _ "github.com/just4zeroq/Omni-link/executor/embedding/openai"
)

func main() {
    ch := &model.Channel{
        ProviderType: model.ProviderOpenAI,
        Protocols: []model.ProtocolEntry{{
            Protocol: model.ProtocolOpenAI,
            BaseURL:  "https://api.openai.com",
        }},
        ApiKey: "sk-...",
    }
    c := client.NewClient(ch)

    // Text chat
    resp, _ := c.Chat(ctx, []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`))

    // Image generation
    images, _ := c.Image(ctx, &image.TextToImageRequest{Prompt: "a cat", N: 1})

    // TTS
    stream, _ := c.Speak(ctx, &audio.TTSRequest{Input: "Hello", Voice: "coral"})
    result, _ := stream.Collect()

    // STT
    text, _ := c.Transcribe(ctx, &audio.STTRequest{File: audioBytes, FileName: "speech.mp3"})

    // Video (async, poll)
    task, _ := c.Video(ctx, &video.TextToVideoRequest{Prompt: "rocket launch"})
    task, _ = c.PollVideo(ctx, task.ID)

    // Text embedding
    emb, _ := c.Embed(ctx, &embedding.EmbeddingRequest{
        Model: "text-embedding-3-small",
        Input: "Hello world",
    })
}
```

---

## Unified Client API

`client.NewClient(channel)` wraps all modalities in one object — no format juggling, no manual executor resolve.

| Method | Purpose |
|--------|---------|
| `c.Chat(ctx, body)` | Text chat (sync, OpenAI JSON body) |
| `c.ChatStream(ctx, body, callback)` | Text chat (streaming) |
| `c.Image(ctx, req)` | Text-to-image |
| `c.ImageEdit(ctx, req)` | Image-to-image |
| `c.GetImageTask(ctx, id)` | Poll async image task |
| `c.Speak(ctx, req)` | TTS → `*AudioStream` |
| `c.Transcribe(ctx, req)` | STT → `*STTResult` |
| `c.Music(ctx, req)` | Music generation (async) |
| `c.PollMusic(ctx, id)` | Poll music task |
| `c.ListVoices(ctx)` | Available TTS voices |
| `c.Video(ctx, req)` | Text-to-video (async) |
| `c.VideoFromImage(ctx, req)` | Image-to-video |
| `c.PollVideo(ctx, id)` | Poll video task |
| `c.Embed(ctx, req)` | Text embeddings |

See [client/client.go](client/client.go) for full API.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         model/                                │
│  ProviderType (46+), Channel config, Protocol resolution     │
└────────────────────────────┬─────────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────────┐
│                       translator/                              │
│  Convert(body, from, to) — format detection + conversion     │
│  Type defs: openai.go, claude.go, responses.go, gemini.go    │
│  12 directional converters in conv.go                         │
└────────────────────────────┬─────────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────────┐
│                        executor/                               │
│  ┌───────────── text/ ─────────────────────────────────────┐  │
│  │ OpenAI · Anthropic · Google · DeepSeek · Volcengine     │  │
│  │ Zhipu · Moonshot · MiniMax · Xiaomi · Kunlun · Stepfun  │  │
│  └─────────────────────────────────────────────────────────┘  │
│  ┌───────────── image/ ────────────────────────────────────┐  │
│  │ OpenAI · Alibaba · Zhipu · Stepfun                      │  │
│  │ Midjourney · NanoBanana · ZImage · Volcengine · Fal     │  │
│  └─────────────────────────────────────────────────────────┘  │
│  ┌───────────── audio/ ────────────────────────────────────┐  │
│  │ OpenAI · Azure · Alibaba · ElevenLabs · PlayHT          │  │
│  │ Cartesia · FishAudio · Stepfun · Suno                   │  │
│  └─────────────────────────────────────────────────────────┘  │
│  ┌───────────── video/ ────────────────────────────────────┐  │
│  │ OpenAI · Alibaba · Kuaishou · Runway · Seedance         │  │
│  │ Hailuo · Pika · Luma · xAI · OmniHuman · Stepfun       │  │
│  └─────────────────────────────────────────────────────────┘  │
│  ┌───────────── embedding/ ───────────────────────────────┐   │
│  │ OpenAI · Zhipu · Alibaba · Jina                        │   │
│  └────────────────────────────────────────────────────────┘   │
│  Plan() → optimal upstream format (score-based)              │
│  SSE stream converters: Claude↔OpenAI (bidirectional)        │
└──────────────────────────────────────────────────────────────┘
```

### Three-Layer Design

**model/** — Provider types, channel config
- 46+ `ProviderType` (OpenAI=1 … Jina=46)
- `Channel` struct with protocol list + API key
- `ResolveProtocol()` maps provider → default protocol

**translator/** — Format conversion (Text)
- `Convert(body, from, to)` — unified entry point
- `DetectFormat(body, path)` — path first, body heuristics
- 4 type definition files + 12 converter functions
- Extensible: add a file + `convertDirect` case

**executor/** — Modality-specific provider execution
- Text: `executor/text` — `Executor` interface: Init, NativeEndpoints, Convert, Customize, Stream, DoRequest
  - `Register("name", &Executor{})` — plugin registry via init()
  - `Plan(in, out, endpoints)` — upstream format selection (score: input+output mismatch)
  - Request-level format override via `UpstreamFormat`
- Image: `executor/image` — `ImageExecutor` interface (TextToImage, ImageToImage, GetTask)
- Audio: `executor/audio` — `AudioExecutor` interface (TextToSpeech/*AudioStream*, SpeechToText, MusicGenerate, GetTask, ListVoices)
- Video: `executor/video` — `VideoExecutor` interface (TextToVideo…+GetTask)
- Embedding: `executor/embedding` — `EmbeddingExecutor` interface (Embed)

---

## Provider Implementations — Text

| Executor | Native Formats | Thinking/Reasoning | Notes |
|---|---|---|---|
| **OpenAI** | `openai` (`/v1/chat/completions`) | `reasoning_effort` | Standard ref |
| **Anthropic** | `claude` (`/v1/messages`) | `thinking.type` | Claude native |
| **Google** | `gemini` (Google endpoint) | — | Via OpenAI hub fallback |
| **DeepSeek** | `openai` + `claude` dual | Effort mapping min→max | 27 tests |
| **Volcengine** | `openai` + `openai_responses` dual | — | 32 tests, bot routing |
| **Zhipu (GLM)** | `openai` (`/v1/chat/completions`) | `thinking.type` + `reasoning_effort` | Base: open.bigmodel.cn |
| **Moonshot (Kimi)** | `openai` (`/v1/chat/completions`) | `reasoning_effort` | Base: api.moonshot.cn |
| **MiniMax** | `openai` (`/v1/chat/completions`) | `thinking.type: adaptive` + `reasoning_split` | Base: api.minimaxi.com |
| **Xiaomi (MiMo)** | `openai` (`/v1/chat/completions`) | `enable_thinking` | Base: api.xiaomimimo.com |
| **Kunlun (SkyClaw)** | `openai` (`/v1/chat/completions`) | — | Base: api.apifree.ai |
| **Stepfun** | `openai` (`/v1/chat/completions`) | `reasoning_effort` | Base: api.stepfun.com |

---

## Provider Implementations — Image

| Executor | T2I | I2I | Pattern | Auth | Notes |
|----------|-----|-----|---------|------|-------|
| **OpenAI** | ✅ | ✅ (edits) | Sync | Bearer | DALL-E / GPT Image 2 |
| **Alibaba** | ✅ | ✅ | Async | Bearer | DashScope: Qwen + Wan models |
| **Zhipu** | ✅ | ✅ | Sync | Bearer | CogView models |
| **Stepfun** | ✅ | ✅ | Sync | Bearer | step-image-edit-2 |
| **Midjourney** | ✅ | ✅ | Async | Bearer | /v1/imagine → poll |
| **NanoBanana** | ✅ | ❌ | Sync | Bearer | OpenAI-compatible |
| **ZImage** | ✅ | ❌ | Sync | Bearer | OpenAI-compatible |
| **Volcengine** (Seedream) | ✅ | ✅ | Sync | Bearer | ark.cn-beijing.volces.com |
| **Fal** (Seedream) | ✅ | ✅ | Sync | Key | fal.ai |

---

## Provider Implementations — Audio

| Executor | TTS | STT | Music | Pattern | Notes |
|----------|-----|-----|-------|---------|-------|
| **OpenAI** | ✅ | ✅ | ❌ | Sync | /v1/audio/speech + transcriptions |
| **Azure** | ✅ | ✅ | ❌ | Sync/SSML | Region-based URL |
| **Alibaba** (CosyVoice) | ✅ | ❌ | ❌ | Sync/URL | DashScope SpeechSynthesizer |
| **Alibaba** (FunASR) | ❌ | ✅ | ❌ | Sync+Async | DashScope + self-hosted |
| **ElevenLabs** | ✅ | ❌ | ❌ | Sync | POST /v1/text-to-speech/{voice_id} |
| **PlayHT** | ✅ | ❌ | ❌ | Sync | /v2/tts/stream |
| **Cartesia** | ✅ | ❌ | ❌ | Sync | Sonic-3 ultra-low-latency |
| **FishAudio** | ✅ | ❌ | ❌ | Sync | Zero-shot voice clone |
| **Stepfun** | ✅ | ❌ | ❌ | Sync | StepAudio TTS |
| **Suno** | ❌ | ❌ | ✅ | Async | Music gen via relay |

### TTS Streaming

`TextToSpeech` returns `*AudioStream` — unified sync/streaming interface:

```go
stream, _ := tts.TextToSpeech(req)

// Streaming: iterate chunks as they arrive
for chunk := range stream.Chunk {
    audioSink.Write(chunk.Data)
}

// Or sync convenience: drain to single result
result, _ := stream.Collect() // *AudioResult with full audio bytes
```

Sync providers wrap result with `audio.NewStreamFromResult()`. Streaming providers push chunks to channel. Caller decides pattern.

---

## Provider Implementations — Video

All video providers are **async** — return pending task, poll via `GetTask`:

| Executor | T2V | I2V | V2V | Extend | Edit | Notes |
|----------|-----|-----|-----|--------|------|-------|
| **OpenAI** (Sora) | ✅ | ❌ | ❌ | ❌ | ✅ | Deprecating Sep 2026 |
| **Alibaba** (Wan) | ✅ | ✅ | ❌ | ❌ | ❌ | DashScope wan2.7 |
| **Alibaba** (HappyHorse) | ✅ | ✅ | ❌ | ❌ | ❌ | DashScope |
| **Kuaishou** (Kling) | ✅ | ✅ | ❌ | ❌ | ❌ | JWT auth |
| **xAI** (Grok) | ✅ | ❌ | ❌ | ❌ | ❌ | Cheapest provider |
| **Runway** | ✅ | ✅ | ✅ | ✅ | ✅ | Gen-4 |
| **Seedance** | ✅ | ❌ | ❌ | ❌ | ❌ | ByteDance, 2K |
| **Hailuo** | ✅ | ❌ | ❌ | ❌ | ❌ | MiniMax |
| **Pika** | ✅ | ✅ | ✅ | ✅ | ✅ | fal.ai |
| **Luma** | ✅ | ✅ | ❌ | ❌ | ❌ | Ray3.2 via fal.ai |
| **OmniHuman** | ❌ | ✅ | ❌ | ❌ | ❌ | ByteDance avatar |
| **Stepfun** | ✅ | ✅ | ❌ | ❌ | ❌ | step-video-ti2v |

---

## Provider Implementations — Embedding

All embedding providers use OpenAI-compatible `/v1/embeddings` format:

| Executor | Models | Dims | Notes |
|----------|--------|------|-------|
| **OpenAI** | text-embedding-3-small/large | 1536/3078 | Standard |
| **Zhipu** | embedding-3, embedding-2 | 256–2048 | open.bigmodel.cn |
| **Alibaba** | text-embedding-v4/v3/v2/v1 | 64–2048 | DashScope |
| **Jina** | jina-embeddings-v3, v2-base-zh | 32–2048 | Matryoshka |

---

## Executor Registration

Omni-link uses Go's `init()` + blank-import pattern (same as `database/sql`):

```go
import (
    _ "github.com/just4zeroq/Omni-link/executor/text/openai"
    _ "github.com/just4zeroq/Omni-link/executor/image/alibaba"
    _ "github.com/just4zeroq/Omni-link/executor/embedding/jina"
)
```

**Only imported executors compile into your binary** — true on-demand loading.
Forget to import → runtime error: `"executor %q not registered (forgot to import?)"`.

---

## Test Coverage

```
Package                    Tests     Notes
─────────────────────────────────────────────────
translator/                  37      No API keys needed
executor/text/deepseek/      27      Needs DEEPSEEK_API_KEY
executor/text/volcengine/    32      Needs VOLC_API_KEY
─────────────────────────────────────────────────
Total                        96      go test ./... -count=1 -timeout 300s
```

```bash
go test ./translator/                             # 37 unit tests
go test ./executor/text/deepseek/ -timeout 120s    # 27 integration
go test ./executor/text/volcengine/ -timeout 180s  # 32 integration
```

Integration tests require `.env`:
```env
DEEPSEEK_API_KEY=sk-...
VOLC_API_KEY=ark-...
```

---

## Project Structure

```
Omni-link/
├── model/
│   └── model.go              # ProviderType (46+), Channel, ResolveProtocol
├── translator/
│   ├── translator.go         # Convert(), DetectFormat(), Format constants
│   ├── conv.go               # 12 directional converters
│   ├── conv_test.go          # 37 tests
│   ├── openai.go             # OpenAI Chat type defs
│   ├── claude.go             # Claude Messages type defs
│   ├── gemini.go             # Gemini type defs
│   └── responses.go          # Responses API type defs
├── client/
│   └── client.go             # Unified client (Chat/Image/Speak/Video/Embed)
├── executor/
│   ├── text/                 # 11 text executors
│   │   ├── executor.go       # Executor interface, Plan()
│   │   ├── registry.go       # Plugin registry
│   │   ├── shared.go         # Helpers (ReplaceModelField, etc.)
│   │   ├── stream_exec.go    # Stream execution pipeline
│   │   ├── streams.go        # SSE converters (Claude↔OpenAI)
│   │   ├── openai/           # OpenAI executor
│   │   ├── anthropic/        # Anthropic Claude executor
│   │   ├── google/           # Google Gemini executor
│   │   ├── deepseek/         # DeepSeek (27 tests)
│   │   ├── volcengine/       # Volcengine/Doubao (32 tests)
│   │   ├── zhipu/            # Zhipu GLM
│   │   ├── moonshot/         # Moonshot Kimi
│   │   ├── minimax/          # MiniMax
│   │   ├── xiaomi/           # Xiaomi MiMo
│   │   ├── kunlun/           # Kunlun SkyClaw
│   │   └── stepfun/          # Stepfun
│   ├── image/                # 9 image executors
│   │   ├── openai/           # OpenAI GPT Image / DALL-E
│   │   ├── alibaba/          # Alibaba Qwen + Wan (DashScope)
│   │   ├── zhipu/            # Zhipu CogView
│   │   ├── stepfun/          # Stepfun image gen/edit
│   │   ├── midjourney/       # Midjourney
│   │   ├── nanobanana/       # NanoBanana
│   │   ├── zimage/           # Z-AIGC
│   │   ├── volcengine/       # ByteDance Seedream via Volcengine
│   │   └── fal/              # ByteDance Seedream via fal.ai
│   ├── audio/                # 10 audio executors
│   │   ├── openai/           # OpenAI TTS/STT
│   │   ├── azure/            # Azure Speech
│   │   ├── alibaba/          # Alibaba CosyVoice + FunASR
│   │   ├── elevenlabs/       # ElevenLabs TTS
│   │   ├── playht/           # PlayHT TTS
│   │   ├── cartesia/         # Cartesia TTS
│   │   ├── fishaudio/        # FishAudio TTS
│   │   ├── stepfun/          # Stepfun StepAudio TTS
│   │   └── suno/             # Suno Music
│   ├── video/                # 12 video executors
│   │   ├── openai/           # OpenAI Sora
│   │   ├── alibaba/          # Alibaba Wan + HappyHorse
│   │   ├── kuaishou/         # Kuaishou Kling
│   │   ├── xai/              # xAI Grok
│   │   ├── stepfun/          # Stepfun Step Video
│   │   ├── runway/           # Runway Gen-4
│   │   ├── seedance/         # ByteDance Seedance
│   │   ├── hailuo/           # MiniMax Hailuo
│   │   ├── pika/             # Pika Labs
│   │   ├── luma/             # Luma Ray3.2
│   │   └── omnihuman/        # ByteDance OmniHuman
│   └── embedding/            # 4 embedding executors
│       ├── openai/           # OpenAI embeddings
│       ├── zhipu/            # Zhipu embeddings
│       ├── alibaba/          # Alibaba Qwen embeddings
│       └── jina/             # Jina AI embeddings
├── docs/
│   ├── image-generation.md   # Image integration spec
│   ├── audio-speech.md       # Audio/speech integration spec
│   ├── video-generation.md   # Video integration spec
│   └── provider-reference.md # Per-provider reference
├── CLAUDE.md                 # Dev conventions
├── go.mod                    # Go 1.23, zero external deps
└── README.md
```

---

## Adding a New Provider

### Text Chat Provider
1. **Define `ProviderType`** in `model/model.go`
2. **Add format types** (if new protocol) in `translator/`
3. **Implement `text.Executor`** in `executor/text/<name>/` with `init()` registration
4. **Define `NativeEndpoints()`** — supported formats + URL paths
5. **Add vendor logic** in `RequestCustomize`/`ResponseCustomize`
6. **Write tests** — unit + integration

### Image / Audio / Video Provider
1. **Pick modality** — `executor/image/`, `executor/audio/`, `executor/video/`
2. **Implement executor interface** (e.g. `ImageExecutor`, `AudioExecutor`, `VideoExecutor`)
3. **Register** via `RegisterImage()`, `RegisterAudio()`, `RegisterVideo()` in `init()`
4. **TTS note**: sync → `audio.NewStreamFromResult()`. Streaming → push to `AudioStream.Chunk`
5. **Video note**: all video async — return pending `*VideoTask`, implement `GetTask` for polling
6. **Write tests**

### Embedding Provider
1. **Implement `embedding.EmbeddingExecutor`** in `executor/embedding/<name>/`
2. **Register** via `embedding.RegisterEmbedding("name", &Executor{})` in `init()`
3. **Standard format**: OpenAI-compatible POST `/v1/embeddings` — zero conversion needed

---

## License

MIT
