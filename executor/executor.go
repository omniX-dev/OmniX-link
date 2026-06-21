// Package executor defines the model-level executor interface for
// LLM request execution with direct format conversion (no hub).
//
// Each executor implements converters for the formats its upstream supports.
// Conversion is always direct — no intermediate format.
package executor

import (
	"io"
	"net/http"

	"github.com/just4zeroq/Omni-link/translator"
)

// Executor is the per-model execution unit.
// Each provider/model registers its own executor with converters
// for the format pairs it supports.
type Executor interface {
	// Init initializes the executor with channel configuration.
	Init(channel any) // *model.Channel

	// GetName returns the human-readable executor name.
	GetName() string

	// NativeEndpoints returns the upstream endpoints this executor natively supports.
	// Used by FormatPlan to choose the optimal upstream format.
	NativeEndpoints() []Endpoint

	// GetRequestURL builds the upstream request URL.
	GetRequestURL(info *RequestInfo) (string, error)

	// SetupRequestHeader configures request headers (auth, content-type, etc.).
	SetupRequestHeader(header http.Header, info *RequestInfo) error

	// --- Direct Format Conversion (no hub) ---

	// ConvertRequest converts request body from one format to another.
	// Returns error for unsupported format pairs — never falls through to hub.
	ConvertRequest(body []byte, from, to translator.Format) ([]byte, error)

	// ConvertResponse converts response body from one format to another.
	// Returns error for unsupported format pairs.
	ConvertResponse(body []byte, from, to translator.Format) ([]byte, error)

	// --- Vendor-specific Customization ---

	// RequestCustomize applies provider-specific request modifications
	// after format conversion but before HTTP call.
	// Examples: thinking injection, model mapping, stream_options injection.
	RequestCustomize(body []byte, info *RequestInfo) []byte

	// ResponseCustomize applies provider-specific response modifications
	// after HTTP call but before response format conversion.
	ResponseCustomize(body []byte, info *RequestInfo) []byte

	// --- Streaming Response Conversion ---

	// NewResponseStream creates a streaming response converter.
	// Returns nil, nil if the format pair doesn't support streaming conversion
	// (caller should fall back to non-streaming).
	NewResponseStream(from, to translator.Format) (ResponseStream, error)

	// --- HTTP Execution ---

	// DoRequest sends the HTTP request to the upstream provider.
	DoRequest(info *RequestInfo, body io.Reader) (*http.Response, error)
}

// RequestInfo carries request-scoped metadata through the pipeline.
type RequestInfo struct {
	RequestID       string
	UpstreamFormat  translator.Format // "" = auto-resolve via Plan
	IsStream        bool
	Model           string
	ActualModelName string
	InboundFormat   translator.Format
	ClientFormat    translator.Format
	Channel         any // *model.Channel
	Protocol        any // ProtocolEntry pointer
	ApiKey          string
	BaseURL         string

	// Provider-specific overrides
	ThinkingEnabled   bool
	ThinkingDisabled  bool
	ReasoningEffort   string
	MaxTokensOverride int
}

// Endpoint describes an upstream endpoint an executor natively supports.
type Endpoint struct {
	Format     translator.Format `json:"format"`
	PathSuffix string            `json:"path_suffix"`
}

// DefaultBaseURL returns the executor's default base URL when client doesn't provide one.
// Override in each executor to return provider-specific default.
func DefaultBaseURL(e Executor) string {
	entries := e.NativeEndpoints()
	if len(entries) > 0 && entries[0].PathSuffix != "" {
		return ""
	}
	return ""
}

// Plan selects the best upstream format given input/output/capabilities.
func Plan(input, output translator.Format, endpoints []Endpoint) translator.Format {
	if len(endpoints) == 0 {
		return input
	}

	best := input // fallback = input format
	bestScore := 999

	for _, ep := range endpoints {
		score := 0
		if input != ep.Format {
			score++
		}
		if output != ep.Format {
			score++
		}

		if score < bestScore || (score == bestScore && ep.Format == output) {
			bestScore = score
			best = ep.Format
		}
	}

	return best
}

// SupportsFormat checks whether executor supports given upstream format.
func SupportsFormat(e Executor, f translator.Format) bool {
	for _, ep := range e.NativeEndpoints() {
		if ep.Format == f {
			return true
		}
	}
	return false
}

// GetPathSuffix returns the path suffix for a given format from the executor.
func GetPathSuffix(e Executor, format translator.Format) string {
	for _, ep := range e.NativeEndpoints() {
		if ep.Format == format {
			return ep.PathSuffix
		}
	}
	return format.EndpointPath()
}

// ResponseStream converts upstream SSE chunks to client-format SSE chunks.
// Created once per request, bound to a specific from→to conversion.
type ResponseStream interface {
	// Feed processes one upstream SSE chunk.
	// Returns the converted chunk(s) for the client (may be empty).
	Feed(chunk []byte) ([]byte, error)

	// End signals the upstream stream is complete.
	// Returns any trailing data needed by the client (e.g., [DONE]).
	End() ([]byte, error)

	// Usage returns accumulated token usage from streaming data.
	Usage() *translator.Usage
}
