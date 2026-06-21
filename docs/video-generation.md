# Video Generation — Design

## Overview

Video modality for Omni-link. Covers text-to-video (T2V), image-to-video (I2V), video-to-video (V2V), and video editing. Same pattern as image/audio: unified interface, provider adaptation, stateless library.

## Categories

| Type | Description | Input | Async |
|---|---|---|---|
| **T2V** | Text → Video | prompt + params | ⏳ All providers |
| **I2V** | Image + Text → Video | image + prompt + params | ⏳ All providers |
| **V2V** | Video + Text → Modified Video | video + prompt + params | ⏳ All providers |
| **Extend** | Continue existing video | video_id + duration | ⏳ All providers |
| **Edit** | Instruction-based editing | video + instructions | ⏳ All providers |

## Architecture

```
executor/
├── text/                  # Text (existing, migrated)
├── image/                 # Image
├── audio/                 # Audio
├── video/                 # Video (new)
│   ├── types.go           # VideoRequest types, VideoTask, VideoResult
│   ├── executor.go        # VideoExecutor interface
│   ├── registry.go        RegisterVideo()
│   ├── sora/              OpenAI Sora ⚠️ deprecating Sep 2026
│   ├── kling/             Kuaishou Kling
│   ├── runway/            Runway Gen-4
│   ├── seedance/          ByteDance Seedance (via fal)
│   ├── hailuo/            MiniMax Hailuo (via aggregators)
│   ├── pika/              Pika Labs (via fal)
│   ├── wan/               Alibaba Wan
│   ├── luma/              Luma Ray3.2 (via fal/replicate)
│   ├── grok/              xAI Grok Imagine Video
│   ├── omnihuman/         ByteDance OmniHuman (数字人像, via fal)
│   └── happyhorse/        Alibaba HappyHorse
└── ...
```

## Principle: Unified Interface, Provider Adaptation

Video is **always async** — no sync providers. The interface is simpler as a result:

```
  Client                        Omni-link                         Provider
    │                             │                                  │
    │  VideoRequest (standard)    │                                  │
    │────────────────────────────▶│                                  │
    │  VideoTask{pending, id}     │── submit generation task ──────▶│
    │◀────────────────────────────│  task_id                         │
    │                             │                                  │
    │──── GetTask(id) ───────────▶│── check status (each call) ────▶│
    │◀─── VideoTask{status, url}──│◀── result or status ───────────│
```

## VideoExecutor Interface

```go
type VideoExecutor interface {
    Init(channel any)
    GetName() string

    // — Text-to-Video —

    // TextToVideo generates video from a text prompt.
    // Always returns pending task — all video providers are async.
    TextToVideo(req *TextToVideoRequest) (*VideoTask, error)

    // — Image-to-Video (optional; may return ErrNotSupported) —

    // ImageToVideo animates/generates video from one or more images.
    ImageToVideo(req *ImageToVideoRequest) (*VideoTask, error)

    // — Video-to-Video (optional; may return ErrNotSupported) —

    // VideoToVideo modifies an existing video with a prompt.
    VideoToVideo(req *VideoToVideoRequest) (*VideoTask, error)

    // — Video Extend (optional) —

    // ExtendVideo continues an existing video.
    ExtendVideo(req *ExtendVideoRequest) (*VideoTask, error)

    // — Video Edit (optional) —

    // EditVideo applies instruction-based edits to a video.
    EditVideo(req *EditVideoRequest) (*VideoTask, error)

    // — Character (Sora-specific, optional) —

    // CreateCharacter registers a character from a reference clip for consistent generation.
    CreateCharacter(req *CharacterRequest) (*Character, error)

    // — Async Task Polling —

    // GetTask queries task status. Each call proxies to upstream.
    GetTask(taskID string) (*VideoTask, error)
}
```

### Async Polling

Client-driven, same as image and audio. No background daemon.

```go
// Submit async task
task, _ := sora.TextToVideo(&TextToVideoRequest{
    Prompt:   "A calico cat playing piano on stage",
    Duration: 10,
    Size:     "1920x1080",
})
// task.Status == pending

// Client polls
for !task.Status.IsTerminal() {
    time.Sleep(5 * time.Second)   // caller controls interval
    task, _ = sora.GetTask(task.ID)
}

// task.Status == completed
// task.VideoURL -> download MP4
```

## Data Types

### TextToVideoRequest

```go
type TextToVideoRequest struct {
    Prompt      string            `json:"prompt"`
    Model       string            `json:"model,omitempty"`
    Size        string            `json:"size,omitempty"`        // "1920x1080"
    Duration    int               `json:"duration,omitempty"`    // seconds
    Quality     string            `json:"quality,omitempty"`     // "standard", "pro"
    Extra       map[string]any    `json:"extra,omitempty"`
}
```

### ImageToVideoRequest

```go
type ImageToVideoRequest struct {
    Prompt      string            `json:"prompt"`
    Model       string            `json:"model,omitempty"`
    Image       string            `json:"image"`                 // URL or base64
    ImageEnd    string            `json:"image_end,omitempty"`   // last frame (Wan, Kling)
    Size        string            `json:"size,omitempty"`
    Duration    int               `json:"duration,omitempty"`
    Quality     string            `json:"quality,omitempty"`
    Extra       map[string]any    `json:"extra,omitempty"`
}
```

### VideoToVideoRequest

```go
type VideoToVideoRequest struct {
    Prompt      string            `json:"prompt"`
    Model       string            `json:"model,omitempty"`
    Video       string            `json:"video"`                 // source video URL
    Strength    float64           `json:"strength,omitempty"`    // 0-1
    Size        string            `json:"size,omitempty"`
    Extra       map[string]any    `json:"extra,omitempty"`
}
```

### ExtendVideoRequest

```go
type ExtendVideoRequest struct {
    VideoID     string            `json:"video_id"`
    Duration    int               `json:"duration,omitempty"`    // additional seconds
    Prompt      string            `json:"prompt,omitempty"`
    Extra       map[string]any    `json:"extra,omitempty"`
}
```

### EditVideoRequest

```go
type EditVideoRequest struct {
    Video       string            `json:"video"`                 // source video URL
    Instructions string           `json:"instructions"`          // "change background to..."
    Model       string            `json:"model,omitempty"`
    Extra       map[string]any    `json:"extra,omitempty"`
}
```

### CharacterRequest

```go
type CharacterRequest struct {
    Name        string            `json:"name"`
    Video       string            `json:"video"`                 // reference clip URL
    Model       string            `json:"model,omitempty"`
    Extra       map[string]any    `json:"extra,omitempty"`
}

type Character struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Model       string            `json:"model"`
}
```

### VideoTask

All generation tasks share one task type:

```go
type VideoTaskStatus string

const (
    VideoTaskQueued      VideoTaskStatus = "queued"
    VideoTaskProcessing  VideoTaskStatus = "processing"
    VideoTaskCompleted   VideoTaskStatus = "completed"
    VideoTaskFailed      VideoTaskStatus = "failed"
)

func (s VideoTaskStatus) IsTerminal() bool {
    return s == VideoTaskCompleted || s == VideoTaskFailed
}

type VideoTask struct {
    ID              string           `json:"id"`
    Status          VideoTaskStatus  `json:"status"`
    VideoURL        string           `json:"video_url,omitempty"`   // expires ~24h
    ThumbnailURL    string           `json:"thumbnail_url,omitempty"`
    Duration        float64          `json:"duration,omitempty"`
    Size            string           `json:"size,omitempty"`       // "1920x1080"
    Error           string           `json:"error,omitempty"`
    CreatedAt       int64            `json:"created_at"`
}
```

## Provider Support

### T2V + I2V

| Executor | TextToVideo | ImageToVideo | Extra | Async |
|---|---|---|---|---|
| Sora | ✅ | ✅ | Extend, Edit, Character | ⏳ POST → poll |
| Kling | ✅ | ✅ | VideoExtend, LipSync | ⏳ POST → poll |
| Runway Gen-4 | ✅ | ✅ | V2V, Character | ⏳ POST → poll |
| Seedance | ✅ | ✅ | Reference-to-Video | ⏳ POST → poll |
| Hailuo | ✅ | ✅ | — | ⏳ POST → poll |
| Pika | ✅ | ✅ | V2V, Pikaffects | ⏳ POST → poll |
| Wan | ✅ | ✅ | VideoEdit, Extend | ⏳ POST → poll |
| Luma Ray3.2 | ✅ | ✅ | Extend, Keyframe | ⏳ POST → poll |
| Grok Imagine | ✅ | ✅ | Reference-to-Video, Edit | ⏳ POST → poll |
| OmniHuman | ❌ | ✅ | Avatar (image+audio→video) | ⏳ POST → poll |
| HappyHorse | ✅ | ✅ | Extend, Edit | ⏳ POST → poll |

### API Format Summary

| Executor | Base URL | Auth | Poll Endpoint |
|---|---|---|---|
| Sora | `https://api.openai.com/v1/videos` | `Authorization: Bearer` | `GET /v1/videos/{id}` |
| Kling | `https://api.klingai.com/v1` | JWT (AK/SK) | `GET /v1/videos/{type}/{id}` |
| Runway | `https://api.dev.runwayml.com/v1` | `Authorization: Bearer` | `GET /v1/tasks/{id}` |
| Seedance | `https://fal.run/bytedance/seedance-2.0` | `Authorization: Key` | `GET /fal-ai/...` (fal style) |
| Hailuo | varies (JD/UCloud/Atlas) | `Authorization: Bearer` | `GET /v1/tasks/{id}` |
| Pika | `https://fal.run/fal-ai/pika/...` | `Authorization: Key` | fal subscribe + poll |
| Wan | `https://dashscope.aliyuncs.com` | `Authorization: Bearer` | `GET /api/v1/tasks/{id}` |
| Luma | `https://fal.run/luma/ray-3.2` | `Authorization: Key` | fal subscribe + poll |
| Grok | `https://api.x.ai/v1/videos/generations` | `Authorization: Bearer` | `GET /v1/videos/generations/{id}` |
| OmniHuman | `https://fal.run/bytedance/omnihuman/v1.5` | `Authorization: Key` | fal subscribe + poll |
| HappyHorse | `https://dashscope.aliyuncs.com` | `Authorization: Bearer` | `GET /api/v1/tasks/{id}` |

## Provider Implementations

### Sora (OpenAI)
- `POST /v1/videos` — create generation
- `GET /v1/videos/{id}` — poll status
- `GET /v1/videos/{id}/content` — download MP4
- Models: `sora-2`, `sora-2-pro`
- Native audio on pro tier
- Character system for consistent characters
- ⚠️ Openai is discontinuing Sora 2 on September 24, 2026

### Kling (Kuaishou)
- `POST /v1/videos/text2video` / `image2video`
- JWT auth (AK/SK signed, 30-min expiry)
- Models: `kling-v3`, `kling-v2.6`, `kling-video-o1`
- Native audio, up to 1080p
- Negative prompt, camera motion controls

### Runway Gen-4
- `POST /v1/text_to_video` / `image_to_video`
- Auth: `Authorization: Bearer` + `X-Runway-Version`
- Models: `gen4.5`, `gen4_turbo`, `gen4_aleph`
- Act Two character performance

### Seedance (ByteDance)
- Via fal.ai: `POST /fal-run/bytedance/seedance-2.0/text-to-video`
- Also `api.seedance.tv/v1/videos`
- 2K resolution, native audio
- Sync audio + video

### Hailuo (MiniMax)
- Via aggregators (JD Cloud, UCloud ModelVerse, Atlas Cloud)
- Camera movement directive system: `[推进]`, `[拉远]`, `[左移]`, `[跟随]`
- Models: `MiniMax-Hailuo-2.3`, `MiniMax-Hailuo-02`
- 768P / 1080P

### Pika (Pika Labs)
- Via fal.ai: `fal-ai/pika/v2.2/text-to-video`
- Pikaffects, Pikascenes, character references
- Max 25s via keyframe stitching

### Wan (Alibaba)
- `POST dashscope.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis`
- Models: `wan2.7-t2v`, `wan2.7-i2v`, `wan2.7-videoedit`
- Open source (Apache 2.0, April 2026)
- First-frame + last-frame image control

### Luma AI (Ray3.2)
- Via fal.ai: `fal-run/luma/ray-3.2/text-to-video` or replicate
- Official API launched June 2026
- Models: `ray-3.2`, `ray-2`, `ray-flash-2`
- Frame-level control (keyframes), HDR export
- T2V, I2V, video extend

### Grok Imagine Video (xAI)
- `POST api.x.ai/v1/videos/generations`
- Auth: `Authorization: Bearer` (xAI API key)
- Models: `grok-imagine-video-1.5`, `grok-imagine-video-1.5-preview`
- 480p / 720p, audio + speech in same pass
- 25s fast generation (720p)
- Cheapest video provider ($0.08-0.14/sec)

### OmniHuman (ByteDance)
- Via fal.ai: `fal-run/bytedance/omnihuman/v1.5`
- **Specialized**: image + audio → talking avatar video (数字人像)
- Not general T2V/I2V — only `ImageToVideo` with audio in Extra
- Parameters: `image_url` + `audio_url` + optional `prompt`
- Max 60s at 720p, 30s at 1080p
- Also via BytePlus official API (HMAC-SHA256 auth)

### HappyHorse (Alibaba)
- Same DashScope infra as Wan: `POST dashscope.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis`
- Auth: `Authorization: Bearer` (DashScope API key)
- Models: `happyhorse-1.0-t2v`, `happyhorse-1.0-i2v`, `happyhorse-1.0-r2v`, `happyhorse-1.0-video-edit`
- 720P / 1080P, 3-15 seconds
- Same polling as Wan (shared DashScope task API)

## Implementation Plan

### Phase 1 — Foundation
1. `executor/video/types.go` — all request types, VideoTask, Character
2. `executor/video/executor.go` — VideoExecutor interface
3. `executor/video/registry.go`

### Phase 2 — First Executor
4. Sora executor (`executor/video/sora/`)
5. Tests: text-to-video, image-to-video, poll flow

### Phase 3 — More Sync Executors
6. Kling, Wan, Runway
7. Individual test suites

### Phase 4 — Aggregator-Native Providers
8. Seedance, Hailuo, Pika (each requires specific auth/adapter)
