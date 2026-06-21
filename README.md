# Omni-link

**Universal AI Protocol Translation Library** — Go library bridging AI API formats across text, image, audio, and video.

```go
import "github.com/just4zeroq/Omni-link/translator"

// Transparent format conversion: OpenAI ↔ Claude ↔ Responses ↔ Gemini
result, _ := translator.Convert(openaiBody, translator.FormatOpenAI, translator.FormatClaude)

// Full executor pipeline with auto-format planning
resp, _ := executor.Request(claudeExecutor, info, body)

// Streaming with cross-format SSE conversion
executor.ExecuteStream(ctx, executor, info, body, callback)
```

[![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Tests](https://img.shields.io/badge/Tests-96_passing-22c55e?style=flat-square)](https://github.com/just4zeroq/Omni-link)
[![License](https://img.shields.io/badge/License-MIT-000000?style=flat-square)](LICENSE)
[![Zero Deps](https://img.shields.io/badge/Dependencies-Zero-6366f1?style=flat-square)](go.mod)

> **Status**: Text protocol translation ✅ | Speech/Image/Video framework in design

---

## Modality Roadmap

| Category | Status | Provider Types |
|---|---|---|
| **🔤 Text** | ✅ Complete | OpenAI, Claude, Gemini, DeepSeek, Volcengine + 35+ more |
| **🖼️ Image** | 🚧 Planned | Midjourney, Jimeng, DALL-E, Stable Diffusion |
| **🎵 Audio** | 🚧 Planned | Suno, ElevenLabs, OpenAI TTS/STT |
| **🎬 Video** | 🚧 Planned | Sora, Kling, Runway |

---

## Quick Start

```bash
go get github.com/just4zeroq/Omni-link
```

```go
package main

import (
    "github.com/just4zeroq/Omni-link/translator"
    "github.com/just4zeroq/Omni-link/executor"
)

func main() {
    // 1. Convert formats
    claudeReq := `{"messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}`
    openaiReq, _ := translator.Convert([]byte(claudeReq),
        translator.FormatClaude, translator.FormatOpenAI)
    // openaiReq → {"model":"...","messages":[...],"max_tokens":1024}

    // 2. Use an executor
    e := &executor.ClaudeExecutor{}
    e.Init(channel)

    info := &executor.RequestInfo{
        InboundFormat:  translator.FormatOpenAI,
        ClientFormat:   translator.FormatOpenAI,
        UpstreamFormat: translator.FormatClaude, // auto-resolve via Plan()
        IsStream:       true,
    }
    executor.ExecuteStream(ctx, e, info, body, callback)
}
```

---

## Text Protocol Translation

### Client-Exposed Formats

| Format | Endpoint | Schema |
|---|---|---|
| `OpenAI` | `/v1/chat/completions` | `messages` + tools → `choices` |
| `Claude` | `/v1/messages` | `messages` + `max_tokens` → `type: "message"` |
| `OpenAI Responses` | `/v1/responses` | `input` → `output` |

### Conversion Matrix — All 12 Pairs Covered

| from ↓ → to | openai | claude | responses | gemini |
|---|---|---|---|---|
| **openai** | — | ✓ | ✓ | ✓ |
| **claude** | ✓ | — | ✓ | ✓ |
| **responses** | ✓ | ✓ | — | ✓ |
| **gemini** ¹ | ✓ | ✓ | ✓ | — |

¹ Gemini format = internal only (Gemini executor). No direct client exposure.  
Unsupported pairs auto-fallback via OpenAI intermediate hub.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                            model/                                 │
│  ProviderType (40+), Channel config, Protocol resolution         │
└────────────────────────────┬─────────────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────────────┐
│                         translator/                               │
│  Convert(body, from, to) — format detection + conversion engine  │
│  Type definitions: openai.go, claude.go, responses.go, gemini.go │
│  12 directional converters in conv.go                             │
└────────────────────────────┬─────────────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────────────┐
│                         executor/                                 │
│  Executor interface + plugin registry + Plan()                   │
│  ┌────────┬────────┬────────┬──────────┬────────────┐            │
│  │ Claude │ OpenAI │ Gemini │ DeepSeek │ Volcengine │            │
│  │ Cl     │ OAI    │ GEM    │ OAI+Cl   │ OAI+RSP    │            │
│  └────────┴────────┴────────┴──────────┴────────────┘            │
│  Plan() → optimal upstream format (score-based)                  │
│  SSE stream converters: Claude↔OpenAI (bidirectional)            │
└──────────────────────────────────────────────────────────────────┘
```

### Three-Layer Design

**model/** — Provider types, channel config
- 40+ `ProviderType` (OpenAI=1 ... Midjourney=40)
- `Channel` struct with protocol list + API key
- `ResolveProtocol()` maps provider → default protocol

**translator/** — Format conversion (Text)
- `Convert(body, from, to)` — unified entry point
- `DetectFormat(body, path)` — path first, body heuristics
- 4 type definition files + 12 converter functions
- Extensible: add a file + `convertDirect` case

**executor/** — Provider execution
- `Executor` interface: Init, NativeEndpoints, Convert, Customize, Stream, DoRequest
- `Register("name", &Executor{})` — plugin registry via init()
- `Plan(in, out, endpoints)` — upstream format selection (score: input+output mismatch)
- `RequestInfo.UpstreamFormat` — zero-value triggers Plan; 4-level override

---

## Provider Implementations — Text

| Executor | Native Formats | Streaming | Tests |
|---|---|---|---|
| **Claude** | `claude` (`/v1/messages`) | ✅ Claude↔OpenAI | translator-level |
| **OpenAI** | `openai` (`/v1/chat/completions`) | ✅ Native | translator-level |
| **Gemini** | `gemini` (Google endpoint) | ⚠️ Via OpenAI hub | translator-level |
| **DeepSeek** | `openai` + `claude` dual | ✅ Bidirectional | **27 tests** |
| **Volcengine** | `openai` + `openai_responses` dual | ✅ Native SSE | **32 tests** |

### DeepSeek
- Dual endpoints: OpenAI `/v1/chat/completions` + Claude `/anthropic/v1/messages`
- Auth: Bearer (OpenAI) / `x-api-key` (Claude)
- Thinking/reasoning injection with effort mapping (minimal→max)
- 27 tests: Chat, streaming, conversion, Plan, tools, thinking, errors

### Volcengine / Doubao (火山引擎)
- OpenAI Chat + Responses endpoints
- Auth: `Authorization: Bearer` + model-in-body
- Multi-model: doubao-seed-2-0-lite, GLM-4-7B, DeepSeek V3
- Bot model routing (`bot-` prefix → `/api/v3/bots/chat/completions`)
- `stream_options: {"include_usage": true}` injection
- 32 tests: 3-model Chat, Responses, streaming, 10-way conversion, Plan, tools, params

---

## Test Coverage — 96 Tests, All Passing ✅

```
Package                    Tests     Notes
─────────────────────────────────────────────────
translator/                  37      No API keys needed
executor/deepseek/           27      Needs DEEPSEEK_API_KEY
executor/volcengine/         32      Needs VOLC_API_KEY
─────────────────────────────────────────────────
Total                        96      go test ./... -count=1 -timeout 300s
```

```bash
go test ./translator/                             # 37 unit tests
go test ./executor/deepseek/ -timeout 120s         # 27 integration
go test ./executor/volcengine/ -timeout 180s       # 32 integration
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
│   └── model.go              # ProviderType (40+), Channel, ResolveProtocol
├── translator/
│   ├── translator.go         # Convert(), DetectFormat(), Format constants
│   ├── conv.go               # 12 directional converters
│   ├── conv_test.go          # 37 tests
│   ├── openai.go             # OpenAI Chat type defs
│   ├── claude.go             # Claude Messages type defs
│   ├── gemini.go             # Gemini type defs
│   └── responses.go          # Responses API type defs
├── executor/
│   ├── executor.go           # Executor interface, RequestInfo, Plan()
│   ├── registry.go           # Plugin registry
│   ├── shared.go             # Helpers (ReplaceModelField, etc.)
│   ├── stream_exec.go        # Stream execution pipeline
│   ├── streams.go            # SSE converters (Claude↔OpenAI)
│   ├── claude/               # Claude executor
│   ├── openai/               # OpenAI executor
│   ├── gemini/               # Gemini executor
│   ├── deepseek/             # DeepSeek (27 tests)
│   └── volcengine/           # Volcengine/Doubao (32 tests)
├── CLAUDE.md                 # Dev conventions
├── go.mod                    # Go 1.23, zero external deps
└── README.md
```

---

## Adding a New Provider

1. **Define `ProviderType`** in `model/model.go`
2. **Add format types** (if new protocol) in `translator/`
3. **Implement `Executor`** in `executor/<name>.go` with `init()` registration
4. **Define `NativeEndpoints()`** — supported formats + URL paths
5. **Add vendor logic** in `RequestCustomize`/`ResponseCustomize`
6. **Write tests** — unit + integration in `executor/<name>/<name>_test.go`

---

## Future Modalities

Each modality follows the same three-layer pattern with modality-specific types:

```
model/          → Modality tags on ProviderType
translator/     → modality/ sub-packages (text/, speech/, image/, video/)
executor/       → Modality-aware executors per provider
```

### Speech (TTS / STT)
- Input: text + voice params → Output: audio stream
- Providers: OpenAI TTS, Azure Speech, ElevenLabs, Suno
- Translation: SSML ↔ plain text, voice profile mapping

### Image Generation
- Input: prompt + params → Output: image URL / base64
- Providers: Midjourney, Jimeng, DALL-E, Stable Diffusion
- Translation: Prompt normalization, parameter mapping

### Video Generation
- Input: prompt + params → Output: video URL / stream
- Providers: Sora, Kling, Runway, Seedance

---

## License

MIT
