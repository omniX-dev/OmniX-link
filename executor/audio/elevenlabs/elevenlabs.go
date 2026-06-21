// Package elevenlabs implements AudioExecutor for ElevenLabs TTS + Voice Clone.
//
// Endpoints:
//   - POST /v1/text-to-speech/{voice_id} — TextToSpeech
//   - GET /v1/voices — ListVoices
//   - POST /v1/voices/add — Voice Clone
//
// Models: eleven_v3, eleven_flash_v2, eleven_turbo_v2
package elevenlabs

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
	audio.RegisterAudio("elevenlabs", &ElevenLabsExecutor{})
}

// ElevenLabsExecutor handles ElevenLabs TTS and voice management.
type ElevenLabsExecutor struct {
	channel any
}

func (e *ElevenLabsExecutor) Init(channel any) {
	e.channel = channel
}

func (e *ElevenLabsExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "ElevenLabs"
}

func (e *ElevenLabsExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.elevenlabs.io"
}

func (e *ElevenLabsExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// elTTSRequest maps to ElevenLabs TTS body.
type elTTSRequest struct {
	Text     string `json:"text"`
	ModelID  string `json:"model_id,omitempty"`
	VoiceSettings *elVoiceSettings `json:"voice_settings,omitempty"`
}

type elVoiceSettings struct {
	Stability       float64 `json:"stability,omitempty"`
	SimilarityBoost float64 `json:"similarity_boost,omitempty"`
	Style           float64 `json:"style,omitempty"`
	UseSpeakerBoost bool    `json:"use_speaker_boost,omitempty"`
}

func (e *ElevenLabsExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	voiceID := req.Voice
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel (default)
	}

	elReq := elTTSRequest{
		Text:    req.Input,
		ModelID: req.Model,
	}
	if elReq.ModelID == "" {
		elReq.ModelID = "eleven_turbo_v2"
	}

	// Voice settings from Extra
	if req.Extra != nil {
		settings := &elVoiceSettings{}
		if v, ok := req.Extra["stability"].(float64); ok {
			settings.Stability = v
		}
		if v, ok := req.Extra["similarity_boost"].(float64); ok {
			settings.SimilarityBoost = v
		}
		elReq.VoiceSettings = settings
	}

	// Map standard params
	if req.Speed > 0 {
		// ElevenLabs doesn't have direct speed param; set via Extra
		if elReq.VoiceSettings == nil {
			elReq.VoiceSettings = &elVoiceSettings{}
		}
	}

	payload, err := json.Marshal(elReq)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: marshal: %w", err)
	}

	path := "/v1/text-to-speech/" + voiceID
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: read audio: %w", err)
	}

	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: "audio/" + format,
		Format:      format,
	}), nil
}

func (e *ElevenLabsExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *ElevenLabsExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *ElevenLabsExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *ElevenLabsExecutor) ListVoices() ([]audio.Voice, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/v1/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: create voice req: %w", err)
	}
	req.Header.Set("xi-api-key", e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: list voices: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: read voices: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs: voices HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var voicesResp struct {
		Voices []struct {
			VoiceID   string            `json:"voice_id"`
			Name      string            `json:"name"`
			PreviewURL string           `json:"preview_url,omitempty"`
			Category  string            `json:"category,omitempty"`
			Labels    map[string]string `json:"labels,omitempty"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(raw, &voicesResp); err != nil {
		return nil, fmt.Errorf("elevenlabs: unmarshal voices: %w", err)
	}

	var voices []audio.Voice
	for _, v := range voicesResp.Voices {
		voices = append(voices, audio.Voice{
			ID:         v.VoiceID,
			Name:       v.Name,
			PreviewURL: v.PreviewURL,
			Category:   v.Category,
			Labels:     v.Labels,
		})
	}
	return voices, nil
}

func (e *ElevenLabsExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", e.getAPIKey())
	return (&http.Client{}).Do(req)
}
