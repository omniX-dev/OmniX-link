# Image Generation — Design

## Overview

Multi-modality extension for Omni-link. Each modality (image, audio, video) gets independent executor interface, standard format, and provider implementations.

## Architecture

```
executor/
├── text/                  # Text (existing Executor interface, migrated later)
├── image/                 # Image (new)
│   ├── executor.go        ImageExecutor interface
│   ├── types.go           ImageRequest / ImageResult / ImageTask
│   ├── registry.go        RegisterImage()
│   ├── gptimage/          GPT Image 2 (OpenAI)
│   ├── midjourney/        Midjourney
│   ├── seedream/          Seedream 5.0 / 4.5 / 4.0
│   ├── qwen/              Qwen Image
│   ├── wan/               Wan2.5 T2I + I2I
│   ├── nanobanana/        Nano Banana 2 / Pro
│   └── zimage/            Z Image Turbo
├── audio/                 # Audio (TBD)
└── video/                 # Video (TBD)
```

Omni-link is a stateless library — no background goroutines, daemons, or task managers. All polling is client-driven.

## Principle: Unified Interface, Provider Adaptation

Omni-link exposes **one set of standard types** (`ImageRequest` / `ImageTask` / `ImageResult`) to all clients. Each executor adapts these to its provider's native format internally. The client never touches provider-specific schemas.

```
  Client                        Omni-link                         Provider
    │                             │                                  │
    │  ImageRequest (standard)    │                                  │
    │────────────────────────────▶│                                  │
    │                             │── executor converts to native ──▶│
    │                             │◀─ executor converts from native ─│
    │◀────────────────────────────│                                  │
    │  ImageTask (standard)       │                                  │
```

This mirrors the text protocol translation pattern (`translator.Convert`): standard in → provider native → standard out.

## ImageExecutor Interface

```go
type ImageExecutor interface {
    Init(channel any) // *model.Channel

    GetName() string

    // TextToImage — text-to-image generation.
    // Sync providers return completed task; async providers return pending task.
    TextToImage(req *TextToImageRequest) (*ImageTask, error)

    // ImageToImage — image-to-image (inpainting, variation, edit).
    // May return ErrNotSupported if provider lacks this capability.
    ImageToImage(req *ImageToImageRequest) (*ImageTask, error)

    // GetTask — query async task status (Midjourney etc.).
    // Each call proxies directly to the upstream provider.
    GetTask(taskID string) (*ImageTask, error)
}
```

### Sync vs Async

| Provider | TextToImage | ImageToImage | GetTask |
|---|---|---|---|
| GPT Image 2 | ✅ Sync completed | ✅ Sync completed | Not needed |
| Seedream / Qwen / Wan | ✅ Sync completed | ✅ Sync completed | Not needed |
| Nano Banana / Z Image Turbo | ✅ Sync completed | ❌ ErrNotSupported | Not needed |
| Midjourney | ⏳ Sync pending + ID | ⏳ Sync pending + ID | ✅ Poll until terminal |

No `Wait()` or `Subscribe()` on the executor. The caller decides polling strategy:

```go
// Sync provider — one call
task, _ := gptimage.TextToImage(req)
// task.Status == completed, use task.Images

// Async provider — client polls
task, _ := mj.TextToImage(req)
// task.Status == pending

for !task.Status.IsTerminal() {
    time.Sleep(2 * time.Second)   // caller controls interval
    task, _ = mj.GetTask(task.ID) // proxies to upstream
}
```

## Data Types

### TextToImageRequest

Standard format = OpenAI `/v1/images/generations` compatible.

```go
type TextToImageRequest struct {
    Prompt         string            `json:"prompt"`
    N              int               `json:"n,omitempty"`           // images count, default 1
    Size           string            `json:"size,omitempty"`        // "1024x1024"
    Quality        string            `json:"quality,omitempty"`     // low / medium / high
    ResponseFormat string            `json:"response_format,omitempty"` // "url" or "b64_json"
    Extra          map[string]any    `json:"extra,omitempty"`
}
```

### ImageToImageRequest

```go
type ImageToImageRequest struct {
    Prompt         string            `json:"prompt"`
    Image          string            `json:"image"`                 // source image (URL or base64)
    Mask           string            `json:"mask,omitempty"`        // inpaint region
    Strength       float64           `json:"strength,omitempty"`    // 0-1, transformation degree
    N              int               `json:"n,omitempty"`
    Size           string            `json:"size,omitempty"`
    ResponseFormat string            `json:"response_format,omitempty"`
    Extra          map[string]any    `json:"extra,omitempty"`
}
```

Executor converts request → provider native format on send, and provider response → `ImageResult` on receive.

**Extra passthrough** for provider-specific options:
```go
req.Extra = map[string]any{
    "seed": 12345,
    "guidance_scale": 7.0,
    "style": "cinematic",
}
```

### ImageResult

```go
type ImageResult struct {
    Index         int    `json:"index"`
    URL           string `json:"url,omitempty"`
    B64JSON       string `json:"b64_json,omitempty"`
    RevisedPrompt string `json:"revised_prompt,omitempty"`
    Seed          int64  `json:"seed,omitempty"`
    ContentType   string `json:"content_type,omitempty"` // "image/png", "image/webp"
}
```

### ImageTask

```go
type TaskStatus string

const (
    TaskStatusPending    TaskStatus = "pending"
    TaskStatusProcessing TaskStatus = "processing"
    TaskStatusCompleted  TaskStatus = "completed"
    TaskStatusFailed     TaskStatus = "failed"
)

func (s TaskStatus) IsTerminal() bool {
    return s == TaskStatusCompleted || s == TaskStatusFailed
}

type ImageTask struct {
    ID        string       `json:"id"`
    Status    TaskStatus   `json:"status"`
    Images    []ImageResult `json:"images,omitempty"`
    Error     string       `json:"error,omitempty"`
    CreatedAt int64        `json:"created_at"`
}
```

## Registry

```go
// executor/image/registry.go

var imageRegistry = map[string]ImageExecutor{}

func RegisterImage(name string, exec ImageExecutor) {
    imageRegistry[name] = exec
}

func GetImage(name string) (ImageExecutor, bool) {
    e, ok := imageRegistry[name]
    return e, ok
}
```

Self-registration via `init()`:
```go
func init() {
    RegisterImage("midjourney", &MidjourneyExecutor{})
}
```

## Async Polling (Client-Driven)

Omni-link does NOT run background tasks. For async providers (Midjourney):

```
 Client                    Omni-link                    Upstream
   │                         │                            │
   │──── Generate(req) ─────▶│──── submit prompt ────────▶│
   │◀─── Task{pending, ID} ──│◀─── task_id ──────────────│
   │                         │                            │
   │──── GetTask(ID) ───────▶│──── check status ─────────▶│
   │◀─── Task{processing} ───│◀─── status ───────────────│
   │                         │                            │
   │──── GetTask(ID) ───────▶│──── check status ─────────▶│
   │◀─── Task{completed} ────│◀─── image URLs ───────────│
   │                         │                            │
   ▼                         ▼                            ▼
```

Each `GetTask()` call is a fresh proxy to upstream — stateless, no cache, no daemon.

The caller (API service using Omni-link) can implement whatever polling strategy it needs: simple loop, adaptive backoff, webhook, or event-driven.

## Supported Providers

| Executor | T2I | I2I | Sync/Async | API Format |
|---|---|---|---|---|
| GPT Image 2 | ✅ | ✅ | Sync | OpenAI `/v1/images/generations` |
| Midjourney | ✅ | ✅ | Async | Custom REST (imagine → poll) |
| Seedream 5.0 | ✅ | ✅ | Sync | Custom REST |
| Seedream 4.5 | ✅ | ✅ | Sync | Custom REST |
| Seedream 4.0 | ✅ | ✅ | Sync | Custom REST |
| Nano Banana 2 | ✅ | ❌ | Sync | Custom REST |
| Nanobanana Pro | ✅ | ❌ | Sync | Custom REST |
| Z Image Turbo | ✅ | ❌ | Sync | Custom REST |
| Qwen Image | ✅ | ✅ | Sync | Alibaba Cloud |
| Wan2.5 T2I | ✅ | ❌ | Sync | Custom REST |
| Wan2.5 I2I | ❌ | ✅ | Sync | Custom REST |

## Text Executor Migration

Existing `executor/` moves to `executor/text/`:

| Before | After |
|---|---|
| `executor/executor.go` | `executor/text/executor.go` |
| `executor/registry.go` | `executor/text/registry.go` |
| `executor/claude/` | `executor/text/claude/` |
| `executor/openai/` | `executor/text/openai/` |
| `executor/gemini/` | `executor/text/gemini/` |
| `executor/deepseek/` | `executor/text/deepseek/` |
| `executor/volcengine/` | `executor/text/volcengine/` |
| `executor/shared.go` | `executor/text/shared.go` |
| `executor/streams.go` | `executor/text/streams.go` |
| `executor/stream_exec.go` | `executor/text/stream_exec.go` |

Common types (RequestInfo, Plan, Endpoint) stay at `executor/` level for cross-modality sharing.

## Implementation Plan

### Phase 1 — Foundation
1. `executor/image/types.go` — ImageRequest, ImageResult, ImageTask
2. `executor/image/executor.go` — ImageExecutor interface
3. `executor/image/registry.go` — Register/GetImage

### Phase 2 — GPT Image 2 (OpenAI)
4. Implement `executor/image/gptimage/` — `/v1/images/generations`
5. Tests: generate, params, url vs b64_json

### Phase 3 — More Sync Providers
6. Seedream, Qwen, Wan, Nano Banana, Z Image Turbo
7. Tests per provider

### Phase 4 — Async Provider
8. Midjourney executor with `GetTask()` polling
9. Tests: submit → poll → complete

### Phase 5 — Text Migration
10. Move `executor/` → `executor/text/`, update imports, verify tests

## Future Modalities

- **Audio**: AudioExecutor (TTS/STT) — standard openai_audio format, same pattern
- **Video**: VideoExecutor — async task model (Sora, Kling), client-driven polling
