// Package client provides a unified Go-idiomatic entry point for all Omni-link API calls.
//
//	// Text chat (sync)
//	resp, err := client.Chat(ctx, []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Hi"}]}`))
//
//	// Text chat (streaming)
//	err := client.ChatStream(ctx, body, func(chunk []byte) error { ... })
//
//	// Image generation
//	images, err := client.Image(ctx, &image.TextToImageRequest{Prompt: "a cat", N: 1})
//
//	// TTS
//	stream, err := client.Speak(ctx, &audio.TTSRequest{Input: "Hello", Voice: "coral"})
//
//	// STT
//	text, err := client.Transcribe(ctx, &audio.STTRequest{File: audioBytes, FileName: "audio.mp3"})
//
//	// Video generation (async)
//	task, err := client.Video(ctx, &video.TextToVideoRequest{Prompt: "rocket launch"})
//	// Poll:
//	task, err = client.PollVideo(ctx, task.ID)
//
// # Executor Registration
//
// Executors register themselves via init(). Import the subpackages you need:
//
//	import (
//		_ "github.com/just4zeroq/Omni-link/executor/text/openai"
//		_ "github.com/just4zeroq/Omni-link/executor/text/anthropic"
//		_ "github.com/just4zeroq/Omni-link/executor/image/openai"
//	)
//
// Only imported executors are compiled into the binary — true on-demand loading.
// For all executors, import:
//
//	import _ "github.com/just4zeroq/Omni-link/executor/all"
package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/just4zeroq/Omni-link/executor/audio"
	audioexec "github.com/just4zeroq/Omni-link/executor/audio"
	"github.com/just4zeroq/Omni-link/executor/image"
	imageexec "github.com/just4zeroq/Omni-link/executor/image"
	textexec "github.com/just4zeroq/Omni-link/executor/text"
	"github.com/just4zeroq/Omni-link/executor/video"
	videoexec "github.com/just4zeroq/Omni-link/executor/video"
	"github.com/just4zeroq/Omni-link/executor/embedding"
	embeddingexec "github.com/just4zeroq/Omni-link/executor/embedding"
	"github.com/just4zeroq/Omni-link/model"
	"github.com/just4zeroq/Omni-link/translator"
)

// ErrUnsupported is returned when the channel's provider doesn't support a capability.
var ErrUnsupported = fmt.Errorf("operation not supported by this provider")

// textExecutorName maps ProviderType → text executor registry name.
// Zero-value = "" means text not supported.
var textExecutorName = map[model.ProviderType]string{
	model.ProviderOpenAI:     "openai",
	model.ProviderAnthropic:  "anthropic",
	model.ProviderDeepSeek:   "deepseek",
	model.ProviderGoogle:     "google",
	model.ProviderVolcengine: "volcengine",
	model.ProviderZhipu:      "zhipu",
	model.ProviderMoonshot:   "moonshot",
	model.ProviderMiniMax:    "minimax",
	model.ProviderXiaomi:     "xiaomi",
	model.ProviderKunlun:     "kunlun",
	model.ProviderStepfun:    "stepfun",
}

// imageExecutorName maps ProviderType → image executor registry name.
var imageExecutorName = map[model.ProviderType]string{
	model.ProviderOpenAI:     "openai",
	model.ProviderMidjourney: "midjourney",
	model.ProviderSeedream:   "seedream",
	model.ProviderAli:        "alibaba",
	model.ProviderZhipu:      "zhipu",
	model.ProviderStepfun:    "stepfun",
	model.ProviderFal:        "fal",
}

// audioExecutorName maps ProviderType → audio executor registry name.
var audioExecutorName = map[model.ProviderType]string{
	model.ProviderOpenAI: "openai",
	model.ProviderAzure:  "azure",
	model.ProviderAli:    "alibaba",
	model.ProviderStepfun: "stepfun",
}

// videoExecutorName maps ProviderType → video executor registry name.
var videoExecutorName = map[model.ProviderType]string{
	model.ProviderOpenAI: "openai",
	model.ProviderAli:    "alibaba",
	model.ProviderKling:  "kuaishou",
	model.ProviderXAI:    "xai",
	model.ProviderStepfun: "stepfun",
}

// embeddingExecutorName maps ProviderType → embedding executor registry name.
var embeddingExecutorName = map[model.ProviderType]string{
	model.ProviderOpenAI: "openai",
	model.ProviderZhipu:  "zhipu",
	model.ProviderAli:    "alibaba",
	model.ProviderJina:   "jina",
}

// Client is the unified entry point for all Omni-link API calls.
// Create via NewClient, then call Chat/Image/Speak/Transcribe/Video/Embed.
type Client struct {
	channel *model.Channel

	// Lazy-resolved executors (thread-safe init).
	mu     sync.Mutex
	tExec  textexec.Executor
	iExec  imageexec.ImageExecutor
	aExec  audioexec.AudioExecutor
	vExec  videoexec.VideoExecutor
	eExec  embeddingexec.EmbeddingExecutor
}

// NewClient creates a client from a channel configuration.
//
// The channel determines provider, API keys, base URLs, and protocol settings.
// Executors are resolved lazily on first use — NewClient never errors.
func NewClient(channel *model.Channel) *Client {
	return &Client{channel: channel}
}

// Channel returns the underlying channel config.
func (c *Client) Channel() *model.Channel { return c.channel }

// --- Text Chat ---

// Chat sends a non-streaming text completion request.
//
// body must be a JSON-serialized request in OpenAI Chat format:
//
//	{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}
//
// Returns the upstream response as JSON bytes, converted to the input format.
func (c *Client) Chat(ctx context.Context, body []byte) ([]byte, error) {
	exec, err := c.textExecutor()
	if err != nil {
		return nil, err
	}

	// Use OpenAI as client-facing format (universal).
	// Upstream format auto-resolved by Plan().
	info := &textexec.RequestInfo{
		RequestID:     newID(),
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		Channel:       c.channel,
	}
	if p := c.channel.ResolveProtocol(model.ResolveProtocol(c.channel.ProviderType)); p != nil {
		info.Protocol = p
		info.BaseURL = p.BaseURL
		info.ApiKey = c.resolveAPIKey()
	}

	return textexec.Request(exec, info, body)
}

// ChatStream sends a streaming text completion request.
//
// body must be JSON in OpenAI Chat format. Each chunk from the upstream
// stream is converted to the input format and passed to callback.
func (c *Client) ChatStream(ctx context.Context, body []byte, callback func([]byte) error) error {
	exec, err := c.textExecutor()
	if err != nil {
		return err
	}

	info := &textexec.RequestInfo{
		RequestID:     newID(),
		InboundFormat: translator.FormatOpenAI,
		ClientFormat:  translator.FormatOpenAI,
		IsStream:      true,
		Channel:       c.channel,
	}
	if p := c.channel.ResolveProtocol(model.ResolveProtocol(c.channel.ProviderType)); p != nil {
		info.Protocol = p
		info.BaseURL = p.BaseURL
		info.ApiKey = c.resolveAPIKey()
	}

	return textexec.ExecuteStream(ctx, exec, info, body, callback)
}

// --- Image ---

// Image generates images from a text prompt.
//
// Returns completed ImageTask for sync providers, pending for async (Midjourney).
// For async, use GetImageTask to poll.
func (c *Client) Image(ctx context.Context, req *image.TextToImageRequest) (*image.ImageTask, error) {
	exec, err := c.imageExecutor()
	if err != nil {
		return nil, err
	}
	return exec.TextToImage(req)
}

// ImageEdit performs image-to-image operations (edit, inpainting, variation).
func (c *Client) ImageEdit(ctx context.Context, req *image.ImageToImageRequest) (*image.ImageTask, error) {
	exec, err := c.imageExecutor()
	if err != nil {
		return nil, err
	}
	return exec.ImageToImage(req)
}

// GetImageTask polls the status of an async image task (Midjourney).
func (c *Client) GetImageTask(ctx context.Context, taskID string) (*image.ImageTask, error) {
	exec, err := c.imageExecutor()
	if err != nil {
		return nil, err
	}
	return exec.GetTask(taskID)
}

// --- Audio ---

// Speak converts text to speech.
//
// Returns AudioStream — call Collect() for sync convenience or range over Chunk for streaming.
func (c *Client) Speak(ctx context.Context, req *audio.TTSRequest) (*audio.AudioStream, error) {
	exec, err := c.audioExecutor()
	if err != nil {
		return nil, err
	}
	return exec.TextToSpeech(req)
}

// Transcribe converts speech to text.
func (c *Client) Transcribe(ctx context.Context, req *audio.STTRequest) (*audio.STTResult, error) {
	exec, err := c.audioExecutor()
	if err != nil {
		return nil, err
	}
	return exec.SpeechToText(req)
}

// Music generates music from a text prompt.
// Async — returns pending task, use PollMusic to check status.
func (c *Client) Music(ctx context.Context, req *audio.MusicRequest) (*audio.AudioTask, error) {
	exec, err := c.audioExecutor()
	if err != nil {
		return nil, err
	}
	return exec.MusicGenerate(req)
}

// PollMusic checks the status of a music generation task.
func (c *Client) PollMusic(ctx context.Context, taskID string) (*audio.AudioTask, error) {
	exec, err := c.audioExecutor()
	if err != nil {
		return nil, err
	}
	return exec.GetTask(taskID)
}

// ListVoices returns available TTS voices.
func (c *Client) ListVoices(ctx context.Context) ([]audio.Voice, error) {
	exec, err := c.audioExecutor()
	if err != nil {
		return nil, err
	}
	return exec.ListVoices()
}

// --- Video ---

// Video generates video from a text prompt.
// Async — returns pending task, use PollVideo to check status.
func (c *Client) Video(ctx context.Context, req *video.TextToVideoRequest) (*video.VideoTask, error) {
	exec, err := c.videoExecutor()
	if err != nil {
		return nil, err
	}
	return exec.TextToVideo(req)
}

// VideoFromImage generates video from one or more images.
func (c *Client) VideoFromImage(ctx context.Context, req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	exec, err := c.videoExecutor()
	if err != nil {
		return nil, err
	}
	return exec.ImageToVideo(req)
}

// PollVideo checks the status of a video generation task.
func (c *Client) PollVideo(ctx context.Context, taskID string) (*video.VideoTask, error) {
	exec, err := c.videoExecutor()
	if err != nil {
		return nil, err
	}
	return exec.GetTask(taskID)
}

// --- Embedding ---

// Embed creates text embeddings using the provider's embedding model.
//
// req.Model — embedding model name
// req.Input — string or []string
// req.Dimensions — optional dimension truncation
//
// Returns embedding vectors with token usage.
func (c *Client) Embed(ctx context.Context, req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	exec, err := c.embeddingExecutor()
	if err != nil {
		return nil, err
	}
	return exec.Embed(req)
}

// --- Internal executor resolution ---

func (c *Client) textExecutor() (textexec.Executor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tExec != nil {
		return c.tExec, nil
	}
	name := textExecutorName[c.channel.ProviderType]
	if name == "" {
		return nil, fmt.Errorf("%w: text chat (no text executor for provider %d)", ErrUnsupported, c.channel.ProviderType)
	}
	e := textexec.GetByProvider(name)
	if e == nil {
		return nil, fmt.Errorf("text executor %q not registered (forgot to import?)", name)
	}
	e.Init(c.channel)
	c.tExec = e
	return e, nil
}

func (c *Client) imageExecutor() (imageexec.ImageExecutor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.iExec != nil {
		return c.iExec, nil
	}
	name := imageExecutorName[c.channel.ProviderType]
	if name == "" {
		// Try provider type map; default to gptimage for OpenAI
		switch c.channel.ProviderType {
		case model.ProviderOpenAI:
			name = "gptimage"
		default:
			return nil, fmt.Errorf("%w: image generation (no image executor for provider %d)", ErrUnsupported, c.channel.ProviderType)
		}
	}
	e := imageexec.GetImage(name)
	if e == nil {
		return nil, fmt.Errorf("image executor %q not registered", name)
	}
	e.Init(c.channel)
	c.iExec = e
	return e, nil
}

func (c *Client) audioExecutor() (audioexec.AudioExecutor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.aExec != nil {
		return c.aExec, nil
	}
	name := audioExecutorName[c.channel.ProviderType]
	if name == "" {
		switch c.channel.ProviderType {
		case model.ProviderOpenAI:
			name = "openai"
		default:
			return nil, fmt.Errorf("%w: audio (no audio executor for provider %d)", ErrUnsupported, c.channel.ProviderType)
		}
	}
	e := audioexec.GetAudio(name)
	if e == nil {
		return nil, fmt.Errorf("audio executor %q not registered", name)
	}
	e.Init(c.channel)
	c.aExec = e
	return e, nil
}

func (c *Client) videoExecutor() (videoexec.VideoExecutor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.vExec != nil {
		return c.vExec, nil
	}
	name := videoExecutorName[c.channel.ProviderType]
	if name == "" {
		switch c.channel.ProviderType {
		case model.ProviderSora:
			name = "sora"
		default:
			return nil, fmt.Errorf("%w: video generation (no video executor for provider %d)", ErrUnsupported, c.channel.ProviderType)
		}
	}
	e := videoexec.GetVideo(name)
	if e == nil {
		return nil, fmt.Errorf("video executor %q not registered", name)
	}
	e.Init(c.channel)
	c.vExec = e
	return e, nil
}

func (c *Client) embeddingExecutor() (embeddingexec.EmbeddingExecutor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eExec != nil {
		return c.eExec, nil
	}
	name := embeddingExecutorName[c.channel.ProviderType]
	if name == "" {
		return nil, fmt.Errorf("%w: text embedding (no embedding executor for provider %d)", ErrUnsupported, c.channel.ProviderType)
	}
	e := embeddingexec.GetEmbedding(name)
	if e == nil {
		return nil, fmt.Errorf("embedding executor %q not registered (forgot to import?)", name)
	}
	e.Init(c.channel)
	c.eExec = e
	return e, nil
}

func (c *Client) resolveAPIKey() string {
	// Try protocol-level API key, then channel-level.
	if c.channel.ApiKey != "" {
		return c.channel.ApiKey
	}
	return ""
}

var idCounter int64

func newID() string {
	idCounter++
	return fmt.Sprintf("cli-%d", idCounter)
}
