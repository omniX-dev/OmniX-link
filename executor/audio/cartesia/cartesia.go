// Package cartesia implements AudioExecutor for Cartesia TTS.
//
// Endpoints:
//   - POST /tts/bytes — sync TTS
//   - POST /tts/sse — SSE streaming TTS
//   - WebSocket — real-time streaming
//
// Model: sonic-3 (~90ms first-byte latency)
// 42 languages, 80+ voices.
package cartesia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("cartesia", &CartesiaExecutor{})
}

// CartesiaExecutor handles Cartesia ultra-low-latency TTS.
type CartesiaExecutor struct {
	channel any
}

func (e *CartesiaExecutor) Init(channel any) {
	e.channel = channel
}

func (e *CartesiaExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Cartesia"
}

func (e *CartesiaExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.cartesia.ai"
}

func (e *CartesiaExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// cartesiaReq maps to Cartesia /tts/bytes request.
type cartesiaReq struct {
	Model       string `json:"model,omitempty"`
	Text        string `json:"text"`
	Voice       string `json:"voice,omitempty"`
	Format      string `json:"output_format,omitempty"`
	Speed       string `json:"speed,omitempty"`
	Emotion     string `json:"emotion,omitempty"`
	Language    string `json:"language,omitempty"`
}

func (e *CartesiaExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	voice := req.Voice
	if voice == "" {
		voice = "a0e41e7a-6b41-4b50-9b09-64b0e0d717f5" // Sonic voice
	}

	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}

	cReq := cartesiaReq{
		Model:  req.Model,
		Text:   req.Input,
		Voice:  voice,
		Format: format,
	}
	if cReq.Model == "" {
		cReq.Model = "sonic-3"
	}

	// Map Extra params
	if req.Extra != nil {
		if s, ok := req.Extra["speed"].(string); ok {
			cReq.Speed = s
		}
		if e, ok := req.Extra["emotion"].(string); ok {
			cReq.Emotion = e
		}
		if l, ok := req.Extra["language"].(string); ok {
			cReq.Language = l
		}
	}

	payload, err := json.Marshal(cReq)
	if err != nil {
		return nil, fmt.Errorf("cartesia: marshal: %w", err)
	}

	resp, err := e.doRequest("/tts/bytes", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cartesia: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cartesia: read audio: %w", err)
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: "audio/" + format,
		Format:      format,
	}), nil
}

func (e *CartesiaExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *CartesiaExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *CartesiaExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *CartesiaExecutor) ListVoices() ([]audio.Voice, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("cartesia: create voice req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("cartesia: list voices: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cartesia: read voices: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cartesia: voices HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var voicesResp []struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Description string           `json:"description,omitempty"`
		Language   string            `json:"language,omitempty"`
	}
	if err := json.Unmarshal(raw, &voicesResp); err != nil {
		return nil, fmt.Errorf("cartesia: unmarshal voices: %w", err)
	}

	var voices []audio.Voice
	for _, v := range voicesResp {
		voices = append(voices, audio.Voice{
			ID:   v.ID,
			Name: v.Name,
			Labels: map[string]string{
				"language": v.Language,
				"description": v.Description,
			},
		})
	}
	return voices, nil
}

func (e *CartesiaExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("cartesia: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}
