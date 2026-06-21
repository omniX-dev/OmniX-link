// Package cosyvoice implements AudioExecutor for Alibaba CosyVoice TTS.
//
// API: DashScope — POST /api/v1/services/audio/tts/SpeechSynthesizer
// Models: cosyvoice-v3.5-plus, cosyvoice-v3.5-flash
// Features: Voice cloning, voice design from text description
package cosyvoice

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
	audio.RegisterAudio("cosyvoice", &CosyVoiceExecutor{})
}

// CosyVoiceExecutor handles Alibaba CosyVoice TTS via DashScope API.
type CosyVoiceExecutor struct {
	channel any
}

func (e *CosyVoiceExecutor) Init(channel any) {
	e.channel = channel
}

func (e *CosyVoiceExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "CosyVoice"
}

func (e *CosyVoiceExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *CosyVoiceExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// cosyTTSRequest maps to DashScope CosyVoice SpeechSynthesizer.
type cosyTTSRequest struct {
	Model string `json:"model"`
	Input struct {
		Text string `json:"text"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

func (e *CosyVoiceExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	model := req.Model
	if model == "" {
		model = "cosyvoice-v3.5-flash"
	}

	cReq := cosyTTSRequest{Model: model}
	cReq.Input.Text = req.Input

	params := make(map[string]any)
	if req.Voice != "" {
		params["voice"] = req.Voice
	}
	if req.ResponseFormat != "" {
		params["format"] = req.ResponseFormat
	}
	if req.Speed > 0 {
		params["speed"] = req.Speed
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	cReq.Parameters = params

	payload, err := json.Marshal(cReq)
	if err != nil {
		return nil, fmt.Errorf("cosyvoice: marshal: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/audio/tts/SpeechSynthesizer", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cosyvoice: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	// DashScope Audio API may return audio in response body directly or structured JSON
	contentType := resp.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "audio/") {
		// Direct audio response
		audioData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("cosyvoice: read audio: %w", err)
		}
		return audio.NewStreamFromResult(&audio.AudioResult{
			Audio:       audioData,
			ContentType: contentType,
			Format:      req.ResponseFormat,
		}), nil
	}

	// Structured JSON response with audio
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cosyvoice: read response: %w", err)
	}

	var cResp struct {
		Output struct {
			TaskID     string `json:"task_id,omitempty"`
			TaskStatus string `json:"task_status,omitempty"`
			AudioURL   string `json:"audio_url,omitempty"`
			Code       string `json:"code,omitempty"`
			Message    string `json:"message,omitempty"`
			Results    []struct {
				AudioURL string `json:"audio_url,omitempty"`
			} `json:"results,omitempty"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &cResp); err != nil {
		return nil, fmt.Errorf("cosyvoice: unmarshal: %w", err)
	}
	if cResp.Output.Code != "" {
		return nil, fmt.Errorf("cosyvoice: %s: %s", cResp.Output.Code, cResp.Output.Message)
	}

	audioURL := cResp.Output.AudioURL
	if audioURL == "" && len(cResp.Output.Results) > 0 {
		audioURL = cResp.Output.Results[0].AudioURL
	}

	if audioURL != "" {
		// Fetch audio from URL
		audioResp, err := http.Get(audioURL)
		if err != nil {
			return nil, fmt.Errorf("cosyvoice: fetch audio: %w", err)
		}
		defer audioResp.Body.Close()
		audioData, err := io.ReadAll(audioResp.Body)
		if err != nil {
			return nil, fmt.Errorf("cosyvoice: read fetched audio: %w", err)
		}
		return audio.NewStreamFromResult(&audio.AudioResult{
			Audio:       audioData,
			ContentType: "audio/mpeg",
			Format:      req.ResponseFormat,
		}), nil
	}

	return nil, fmt.Errorf("cosyvoice: no audio in response")
}

func (e *CosyVoiceExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *CosyVoiceExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *CosyVoiceExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *CosyVoiceExecutor) ListVoices() ([]audio.Voice, error) {
	return nil, audio.ErrNotSupported
}

func (e *CosyVoiceExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("cosyvoice: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}
